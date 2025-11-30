package services

import (
	"fmt"
	"regexp"
	"sort"

	"DF-PLCH/internal"
	"DF-PLCH/internal/models"

	"github.com/google/uuid"
)

type EntityRuleService struct{}

func NewEntityRuleService() *EntityRuleService {
	return &EntityRuleService{}
}

// GetAllRules returns all active entity rules sorted by priority
func (s *EntityRuleService) GetAllRules() ([]models.EntityRule, error) {
	var rules []models.EntityRule
	if err := internal.DB.Where("is_active = ?", true).Order("priority DESC").Find(&rules).Error; err != nil {
		return nil, fmt.Errorf("failed to get entity rules: %w", err)
	}
	return rules, nil
}

// GetAllRulesIncludingInactive returns all entity rules including inactive ones
func (s *EntityRuleService) GetAllRulesIncludingInactive() ([]models.EntityRule, error) {
	var rules []models.EntityRule
	if err := internal.DB.Order("priority DESC").Find(&rules).Error; err != nil {
		return nil, fmt.Errorf("failed to get entity rules: %w", err)
	}
	return rules, nil
}

// GetRule returns a single rule by ID
func (s *EntityRuleService) GetRule(id string) (*models.EntityRule, error) {
	var rule models.EntityRule
	if err := internal.DB.First(&rule, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("rule not found: %w", err)
	}
	return &rule, nil
}

// GetRuleByCode returns a single rule by code
func (s *EntityRuleService) GetRuleByCode(code string) (*models.EntityRule, error) {
	var rule models.EntityRule
	if err := internal.DB.First(&rule, "code = ?", code).Error; err != nil {
		return nil, fmt.Errorf("rule not found: %w", err)
	}
	return &rule, nil
}

// CreateRule creates a new entity rule
func (s *EntityRuleService) CreateRule(rule *models.EntityRule) (*models.EntityRule, error) {
	if rule.ID == "" {
		rule.ID = "entity_" + uuid.New().String()[:8]
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

// UpdateRule updates an existing entity rule
func (s *EntityRuleService) UpdateRule(id string, updates *models.EntityRule) (*models.EntityRule, error) {
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
	if updates.Code != "" {
		rule.Code = updates.Code
	}
	if updates.Description != "" {
		rule.Description = updates.Description
	}
	if updates.Color != "" {
		rule.Color = updates.Color
	}
	if updates.Icon != "" {
		rule.Icon = updates.Icon
	}

	rule.Priority = updates.Priority
	rule.IsActive = updates.IsActive

	if err := internal.DB.Save(rule).Error; err != nil {
		return nil, fmt.Errorf("failed to update rule: %w", err)
	}

	return rule, nil
}

// DeleteRule deletes an entity rule
func (s *EntityRuleService) DeleteRule(id string) error {
	if err := internal.DB.Delete(&models.EntityRule{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("failed to delete rule: %w", err)
	}
	return nil
}

// InitializeDefaultRules creates or updates default rules
func (s *EntityRuleService) InitializeDefaultRules() error {
	defaultRules := models.DefaultEntityRules()
	created := 0
	updated := 0

	for _, rule := range defaultRules {
		// Check if rule with this ID already exists
		var existing models.EntityRule
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
				"code":        rule.Code,
				"description": rule.Description,
				"pattern":     rule.Pattern,
				"priority":    rule.Priority,
				"is_active":   rule.IsActive,
				"color":       rule.Color,
				"icon":        rule.Icon,
			}
			if err := internal.DB.Model(&models.EntityRule{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
				return fmt.Errorf("failed to update default rule %s: %w", rule.ID, err)
			}
			updated++
		}
	}

	fmt.Printf("Initialized default entity rules: %d created, %d updated\n", created, updated)
	return nil
}

// DetectEntity detects the entity for a placeholder key using configured rules
func (s *EntityRuleService) DetectEntity(key string) (string, error) {
	rules, err := s.GetAllRules()
	if err != nil {
		return "general", err
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

		if re.MatchString(key) {
			return rule.Code, nil
		}
	}

	return "general", nil
}

// GetEntityLabels returns a map of entity codes to their display names
func (s *EntityRuleService) GetEntityLabels() (map[string]string, error) {
	rules, err := s.GetAllRulesIncludingInactive()
	if err != nil {
		return nil, err
	}

	labels := make(map[string]string)
	for _, rule := range rules {
		labels[rule.Code] = rule.Name
	}

	return labels, nil
}

// GetEntityColors returns a map of entity codes to their colors
func (s *EntityRuleService) GetEntityColors() (map[string]string, error) {
	rules, err := s.GetAllRulesIncludingInactive()
	if err != nil {
		return nil, err
	}

	colors := make(map[string]string)
	for _, rule := range rules {
		colors[rule.Code] = rule.Color
	}

	return colors, nil
}
