package handlers

import (
	"net/http"
	"regexp"

	"DF-PLCH/internal/models"
	"DF-PLCH/internal/services"

	"github.com/gin-gonic/gin"
)

type FieldRuleHandler struct {
	service *services.FieldRuleService
}

func NewFieldRuleHandler(service *services.FieldRuleService) *FieldRuleHandler {
	return &FieldRuleHandler{service: service}
}

// GetAllRules returns all field rules
func (h *FieldRuleHandler) GetAllRules(c *gin.Context) {
	includeInactive := c.Query("include_inactive") == "true"

	var rules []models.FieldRule
	var err error

	if includeInactive {
		rules, err = h.service.GetAllRulesIncludingInactive()
	} else {
		rules, err = h.service.GetAllRules()
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"rules": rules,
		"total": len(rules),
	})
}

// GetRule returns a single rule by ID
func (h *FieldRuleHandler) GetRule(c *gin.Context) {
	id := c.Param("ruleId")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Rule ID is required"})
		return
	}

	rule, err := h.service.GetRule(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}

	c.JSON(http.StatusOK, rule)
}

// CreateRuleRequest represents the request body for creating a rule
type CreateRuleRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Pattern     string `json:"pattern" binding:"required"`
	Priority    int    `json:"priority"`
	IsActive    bool   `json:"is_active"`
	DataType    string `json:"data_type"`
	InputType   string `json:"input_type"`
	GroupName   string `json:"group_name"`
	Validation  string `json:"validation"`
	Options     string `json:"options"`
}

// CreateRule creates a new field rule
func (h *FieldRuleHandler) CreateRule(c *gin.Context) {
	var req CreateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Ensure validation and options are valid JSON or empty
	validation := req.Validation
	if validation == "" {
		validation = "{}"
	}
	options := req.Options
	if options == "" {
		options = "[]"
	}

	rule := &models.FieldRule{
		Name:        req.Name,
		Description: req.Description,
		Pattern:     req.Pattern,
		Priority:    req.Priority,
		IsActive:    req.IsActive,
		DataType:    req.DataType,
		InputType:   req.InputType,
		GroupName:   req.GroupName,
		Validation:  validation,
		Options:     options,
	}

	createdRule, err := h.service.CreateRule(rule)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Rule created successfully",
		"rule":    createdRule,
	})
}

// UpdateRuleRequest represents the request body for updating a rule
type UpdateRuleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Pattern     string `json:"pattern"`
	Priority    int    `json:"priority"`
	IsActive    bool   `json:"is_active"`
	DataType    string `json:"data_type"`
	InputType   string `json:"input_type"`
	GroupName   string `json:"group_name"`
	Validation  string `json:"validation"`
	Options     string `json:"options"`
}

// UpdateRule updates an existing field rule
func (h *FieldRuleHandler) UpdateRule(c *gin.Context) {
	id := c.Param("ruleId")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Rule ID is required"})
		return
	}

	var req UpdateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Ensure validation and options are valid JSON or empty
	validation := req.Validation
	if validation == "" {
		validation = "{}"
	}
	options := req.Options
	if options == "" {
		options = "[]"
	}

	updates := &models.FieldRule{
		Name:        req.Name,
		Description: req.Description,
		Pattern:     req.Pattern,
		Priority:    req.Priority,
		IsActive:    req.IsActive,
		DataType:    req.DataType,
		InputType:   req.InputType,
		GroupName:   req.GroupName,
		Validation:  validation,
		Options:     options,
	}

	updatedRule, err := h.service.UpdateRule(id, updates)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Rule updated successfully",
		"rule":    updatedRule,
	})
}

// DeleteRule deletes a field rule
func (h *FieldRuleHandler) DeleteRule(c *gin.Context) {
	id := c.Param("ruleId")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Rule ID is required"})
		return
	}

	if err := h.service.DeleteRule(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Rule deleted successfully",
	})
}

// TestRule tests a pattern against sample placeholders
type TestRuleRequest struct {
	Pattern      string   `json:"pattern" binding:"required"`
	Placeholders []string `json:"placeholders" binding:"required"`
}

func (h *FieldRuleHandler) TestRule(c *gin.Context) {
	var req TestRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	results := make(map[string]bool)
	for _, placeholder := range req.Placeholders {
		// Remove {{ and }} for testing
		key := placeholder
		if len(key) > 4 && key[:2] == "{{" && key[len(key)-2:] == "}}" {
			key = key[2 : len(key)-2]
		}

		matched, _ := regexp.MatchString(req.Pattern, key)
		results[placeholder] = matched
	}

	c.JSON(http.StatusOK, gin.H{
		"pattern": req.Pattern,
		"results": results,
	})
}

// InitializeDefaultRules initializes default rules if none exist
func (h *FieldRuleHandler) InitializeDefaultRules(c *gin.Context) {
	if err := h.service.InitializeDefaultRules(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Default rules initialized successfully",
	})
}

// GetDataTypes returns available data types
func (h *FieldRuleHandler) GetDataTypes(c *gin.Context) {
	dataTypes := []map[string]string{
		{"value": "text", "label": "ข้อความ"},
		{"value": "id_number", "label": "เลขบัตรประชาชน"},
		{"value": "date", "label": "วันที่"},
		{"value": "time", "label": "เวลา"},
		{"value": "number", "label": "ตัวเลข"},
		{"value": "address", "label": "ที่อยู่"},
		{"value": "province", "label": "จังหวัด"},
		{"value": "country", "label": "ประเทศ"},
		{"value": "name_prefix", "label": "คำนำหน้าชื่อ"},
		{"value": "name", "label": "ชื่อ"},
		{"value": "weekday", "label": "วันในสัปดาห์"},
		{"value": "phone", "label": "เบอร์โทรศัพท์"},
		{"value": "email", "label": "อีเมล"},
		{"value": "house_code", "label": "รหัสบ้าน"},
		{"value": "zodiac", "label": "ปีนักษัตร"},
		{"value": "lunar_month", "label": "เดือนจันทรคติ"},
	}

	c.JSON(http.StatusOK, gin.H{"data_types": dataTypes})
}

// GetInputTypes returns available input types
func (h *FieldRuleHandler) GetInputTypes(c *gin.Context) {
	inputTypes := []map[string]string{
		{"value": "text", "label": "Text Input"},
		{"value": "select", "label": "Dropdown Select"},
		{"value": "date", "label": "Date Picker"},
		{"value": "time", "label": "Time Picker"},
		{"value": "number", "label": "Number Input"},
		{"value": "textarea", "label": "Text Area"},
	}

	c.JSON(http.StatusOK, gin.H{"input_types": inputTypes})
}
