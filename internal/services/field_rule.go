package services

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"DF-PLCH/internal"
	"DF-PLCH/internal/models"
	"DF-PLCH/internal/utils"

	"github.com/google/uuid"
)

type FieldRuleService struct{}

func NewFieldRuleService() *FieldRuleService {
	return &FieldRuleService{}
}

// GetAllRules returns all active field rules sorted by priority
func (s *FieldRuleService) GetAllRules() ([]models.FieldRule, error) {
	var rules []models.FieldRule
	if err := internal.DB.Where("is_active = ?", true).Order("priority DESC").Find(&rules).Error; err != nil {
		return nil, fmt.Errorf("failed to get field rules: %w", err)
	}
	return rules, nil
}

// GetAllRulesIncludingInactive returns all field rules including inactive ones
func (s *FieldRuleService) GetAllRulesIncludingInactive() ([]models.FieldRule, error) {
	var rules []models.FieldRule
	if err := internal.DB.Order("priority DESC").Find(&rules).Error; err != nil {
		return nil, fmt.Errorf("failed to get field rules: %w", err)
	}
	return rules, nil
}

// GetRule returns a single rule by ID
func (s *FieldRuleService) GetRule(id string) (*models.FieldRule, error) {
	var rule models.FieldRule
	if err := internal.DB.First(&rule, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("rule not found: %w", err)
	}
	return &rule, nil
}

// CreateRule creates a new field rule
func (s *FieldRuleService) CreateRule(rule *models.FieldRule) (*models.FieldRule, error) {
	if rule.ID == "" {
		rule.ID = "rule_" + uuid.New().String()[:8]
	}

	// Validate the regex pattern
	if _, err := regexp.Compile(rule.Pattern); err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	if err := internal.DB.Create(rule).Error; err != nil {
		return nil, fmt.Errorf("failed to create rule: %w", err)
	}

	return rule, nil
}

// UpdateRule updates an existing field rule
func (s *FieldRuleService) UpdateRule(id string, updates *models.FieldRule) (*models.FieldRule, error) {
	rule, err := s.GetRule(id)
	if err != nil {
		return nil, err
	}

	// Validate the regex pattern if it's being updated
	if updates.Pattern != "" {
		if _, err := regexp.Compile(updates.Pattern); err != nil {
			return nil, fmt.Errorf("invalid regex pattern: %w", err)
		}
		rule.Pattern = updates.Pattern
	}

	if updates.Name != "" {
		rule.Name = updates.Name
	}
	if updates.Description != "" {
		rule.Description = updates.Description
	}
	if updates.DataType != "" {
		rule.DataType = updates.DataType
	}
	if updates.InputType != "" {
		rule.InputType = updates.InputType
	}
	if updates.GroupName != "" {
		rule.GroupName = updates.GroupName
	}
	// Always update validation and options (they should always be valid JSON)
	rule.Validation = updates.Validation
	rule.Options = updates.Options

	rule.Priority = updates.Priority
	rule.IsActive = updates.IsActive

	if err := internal.DB.Save(rule).Error; err != nil {
		return nil, fmt.Errorf("failed to update rule: %w", err)
	}

	return rule, nil
}

// DeleteRule soft deletes a field rule
func (s *FieldRuleService) DeleteRule(id string) error {
	if err := internal.DB.Delete(&models.FieldRule{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("failed to delete rule: %w", err)
	}
	return nil
}

// InitializeDefaultRules creates or updates default rules
func (s *FieldRuleService) InitializeDefaultRules() error {
	defaultRules := models.DefaultFieldRules()
	created := 0
	updated := 0

	for _, rule := range defaultRules {
		// Check if rule with this ID already exists
		var existing models.FieldRule
		err := internal.DB.Where("id = ?", rule.ID).First(&existing).Error

		if err != nil {
			// Doesn't exist, create new
			if err := internal.DB.Create(&rule).Error; err != nil {
				return fmt.Errorf("failed to create default rule %s: %w", rule.ID, err)
			}
			created++
		} else {
			// Exists, update it
			updates := map[string]interface{}{
				"name":        rule.Name,
				"description": rule.Description,
				"pattern":     rule.Pattern,
				"priority":    rule.Priority,
				"is_active":   rule.IsActive,
				"data_type":   rule.DataType,
				"input_type":  rule.InputType,
				"group_name":  rule.GroupName,
				"validation":  rule.Validation,
				"options":     rule.Options,
			}
			if err := internal.DB.Model(&models.FieldRule{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
				return fmt.Errorf("failed to update default rule %s: %w", rule.ID, err)
			}
			updated++
		}
	}

	fmt.Printf("Initialized default field rules: %d created, %d updated\n", created, updated)
	return nil
}

// ApplyRulesToPlaceholder applies matching rules to a placeholder and returns a FieldDefinition
func (s *FieldRuleService) ApplyRulesToPlaceholder(placeholder string, rules []models.FieldRule) utils.FieldDefinition {
	// Remove {{ and }} from placeholder
	key := strings.ReplaceAll(placeholder, "{{", "")
	key = strings.ReplaceAll(key, "}}", "")

	// Default definition
	definition := utils.FieldDefinition{
		Placeholder: placeholder,
		DataType:    utils.DataTypeText,
		Entity:      utils.EntityGeneral,
		InputType:   utils.InputTypeText,
	}

	// Sort rules by priority (highest first)
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Priority > rules[j].Priority
	})

	// Try each rule
	for _, rule := range rules {
		if !rule.IsActive {
			continue
		}

		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			continue
		}

		matches := re.FindStringSubmatch(key)
		if matches == nil {
			continue
		}

		// Apply the rule
		if rule.DataType != "" {
			definition.DataType = utils.DataType(rule.DataType)
		}
		if rule.InputType != "" {
			definition.InputType = utils.InputType(rule.InputType)
		}

		// Handle group name with placeholders
		if rule.GroupName != "" {
			groupName := rule.GroupName

			// Replace {prefix} placeholder
			if strings.Contains(groupName, "{prefix}") && len(matches) > 1 {
				groupName = strings.ReplaceAll(groupName, "{prefix}", strings.ToLower(matches[1]))
			}

			// Replace {suffix} placeholder
			if strings.Contains(groupName, "{suffix}") && len(matches) > 2 {
				groupName = strings.ReplaceAll(groupName, "{suffix}", strings.ToUpper(matches[2]))
			}

			// Extract number for group order
			for i, match := range matches {
				if i == 0 {
					continue
				}
				if num, err := strconv.Atoi(match); err == nil {
					definition.GroupOrder = num
					break
				}
			}

			definition.Group = groupName
		}

		// Parse validation
		if rule.Validation != "" {
			var validation utils.FieldValidation
			if err := json.Unmarshal([]byte(rule.Validation), &validation); err == nil {
				definition.Validation = &validation
			}
		}

		// Parse options for select inputs
		if rule.Options != "" && definition.InputType == utils.InputTypeSelect {
			var options []string
			if err := json.Unmarshal([]byte(rule.Options), &options); err == nil {
				if definition.Validation == nil {
					definition.Validation = &utils.FieldValidation{}
				}
				definition.Validation.Options = options
			}
		}

		// Rule matched, stop processing
		break
	}

	return definition
}

// GenerateFieldDefinitionsWithRules generates field definitions using the configured rules
// Order is set based on document position (first come first serve)
func (s *FieldRuleService) GenerateFieldDefinitionsWithRules(placeholders []string) (map[string]utils.FieldDefinition, error) {
	// First try to use Data Types with patterns (primary detection method)
	var dataTypes []models.DataType
	if err := internal.DB.Where("is_active = ?", true).Order("priority DESC").Find(&dataTypes).Error; err == nil && len(dataTypes) > 0 {
		definitions := make(map[string]utils.FieldDefinition)

		for idx, placeholder := range placeholders {
			key := strings.ReplaceAll(placeholder, "{{", "")
			key = strings.ReplaceAll(key, "}}", "")
			def := s.ApplyDataTypesToPlaceholder(placeholder, dataTypes)
			def.Order = idx // Set order based on document position
			definitions[key] = def
		}

		return definitions, nil
	}

	// Fallback to Field Rules if no Data Types configured
	rules, err := s.GetAllRules()
	if err != nil {
		return nil, err
	}

	definitions := make(map[string]utils.FieldDefinition)

	for idx, placeholder := range placeholders {
		key := strings.ReplaceAll(placeholder, "{{", "")
		key = strings.ReplaceAll(key, "}}", "")

		var def utils.FieldDefinition
		// If no rules, fall back to the built-in detection
		if len(rules) == 0 {
			def = utils.DetectFieldType(placeholder)
		} else {
			def = s.ApplyRulesToPlaceholder(placeholder, rules)
		}
		def.Order = idx // Set order based on document position
		definitions[key] = def
	}

	return definitions, nil
}

// ApplyDataTypesToPlaceholder applies matching data types to a placeholder and returns a FieldDefinition
func (s *FieldRuleService) ApplyDataTypesToPlaceholder(placeholder string, dataTypes []models.DataType) utils.FieldDefinition {
	// Remove {{ and }} from placeholder
	key := strings.ReplaceAll(placeholder, "{{", "")
	key = strings.ReplaceAll(key, "}}", "")

	// Default definition
	definition := utils.FieldDefinition{
		Placeholder: placeholder,
		DataType:    utils.DataTypeText,
		Entity:      utils.EntityGeneral,
		InputType:   utils.InputTypeText,
	}

	// Sort data types by priority (highest first)
	sort.Slice(dataTypes, func(i, j int) bool {
		return dataTypes[i].Priority > dataTypes[j].Priority
	})

	// Try each data type
	for _, dt := range dataTypes {
		if !dt.IsActive || dt.Pattern == "" {
			continue
		}

		// Skip the fallback "text" pattern (.*) - process it last
		if dt.Code == "text" && dt.Pattern == ".*" {
			continue
		}

		re, err := regexp.Compile(dt.Pattern)
		if err != nil {
			continue
		}

		if !re.MatchString(key) {
			continue
		}

		// Apply the data type
		definition.DataType = utils.DataType(dt.Code)
		if dt.InputType != "" {
			definition.InputType = utils.InputType(dt.InputType)
		}
		definition.Description = dt.Description
		definition.DefaultValue = dt.DefaultValue

		// Parse validation
		if dt.Validation != "" && dt.Validation != "{}" {
			var validation utils.FieldValidation
			if err := json.Unmarshal([]byte(dt.Validation), &validation); err == nil {
				definition.Validation = &validation
			}
		}

		// Parse options for select inputs
		if dt.Options != "" && dt.Options != "{}" && definition.InputType == utils.InputTypeSelect {
			var options []string
			if err := json.Unmarshal([]byte(dt.Options), &options); err == nil {
				if definition.Validation == nil {
					definition.Validation = &utils.FieldValidation{}
				}
				definition.Validation.Options = options
			}
		}

		// Data type matched, stop processing
		return definition
	}

	// No match found, return default text type
	return definition
}
