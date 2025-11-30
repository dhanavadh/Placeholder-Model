package handlers

import (
	"net/http"

	"DF-PLCH/internal/models"
	"DF-PLCH/internal/services"

	"github.com/gin-gonic/gin"
)

type EntityRuleHandler struct {
	service *services.EntityRuleService
}

func NewEntityRuleHandler(service *services.EntityRuleService) *EntityRuleHandler {
	return &EntityRuleHandler{service: service}
}

// GetAllRules returns all entity rules
func (h *EntityRuleHandler) GetAllRules(c *gin.Context) {
	includeInactive := c.Query("include_inactive") == "true"

	var rules []models.EntityRule
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
func (h *EntityRuleHandler) GetRule(c *gin.Context) {
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

// CreateRuleRequest represents the request body for creating an entity rule
type CreateEntityRuleRequest struct {
	Name        string `json:"name" binding:"required"`
	Code        string `json:"code" binding:"required"`
	Description string `json:"description"`
	Pattern     string `json:"pattern" binding:"required"`
	Priority    int    `json:"priority"`
	IsActive    bool   `json:"is_active"`
	Color       string `json:"color"`
	Icon        string `json:"icon"`
}

// CreateRule creates a new entity rule
func (h *EntityRuleHandler) CreateRule(c *gin.Context) {
	var req CreateEntityRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	rule := &models.EntityRule{
		Name:        req.Name,
		Code:        req.Code,
		Description: req.Description,
		Pattern:     req.Pattern,
		Priority:    req.Priority,
		IsActive:    req.IsActive,
		Color:       req.Color,
		Icon:        req.Icon,
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

// UpdateRuleRequest represents the request body for updating an entity rule
type UpdateEntityRuleRequest struct {
	Name        string `json:"name"`
	Code        string `json:"code"`
	Description string `json:"description"`
	Pattern     string `json:"pattern"`
	Priority    int    `json:"priority"`
	IsActive    bool   `json:"is_active"`
	Color       string `json:"color"`
	Icon        string `json:"icon"`
}

// UpdateRule updates an existing entity rule
func (h *EntityRuleHandler) UpdateRule(c *gin.Context) {
	id := c.Param("ruleId")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Rule ID is required"})
		return
	}

	var req UpdateEntityRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	updates := &models.EntityRule{
		Name:        req.Name,
		Code:        req.Code,
		Description: req.Description,
		Pattern:     req.Pattern,
		Priority:    req.Priority,
		IsActive:    req.IsActive,
		Color:       req.Color,
		Icon:        req.Icon,
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

// DeleteRule deletes an entity rule
func (h *EntityRuleHandler) DeleteRule(c *gin.Context) {
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

// InitializeDefaultRules initializes default entity rules if none exist
func (h *EntityRuleHandler) InitializeDefaultRules(c *gin.Context) {
	if err := h.service.InitializeDefaultRules(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Default entity rules initialized successfully",
	})
}

// GetEntityLabels returns all entity codes with their display names
func (h *EntityRuleHandler) GetEntityLabels(c *gin.Context) {
	labels, err := h.service.GetEntityLabels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"labels": labels})
}

// GetEntityColors returns all entity codes with their colors
func (h *EntityRuleHandler) GetEntityColors(c *gin.Context) {
	colors, err := h.service.GetEntityColors()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"colors": colors})
}
