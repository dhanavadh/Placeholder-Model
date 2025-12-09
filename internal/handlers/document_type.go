package handlers

import (
	"fmt"
	"net/http"

	"DF-PLCH/internal/services"

	"github.com/gin-gonic/gin"
)

type DocumentTypeHandler struct {
	service           *services.DocumentTypeService
	autoSuggestService *services.AutoSuggestService
}

func NewDocumentTypeHandler(service *services.DocumentTypeService) *DocumentTypeHandler {
	return &DocumentTypeHandler{
		service:           service,
		autoSuggestService: services.NewAutoSuggestService(),
	}
}

// CreateDocumentType creates a new document type
// POST /api/v1/document-types
func (h *DocumentTypeHandler) CreateDocumentType(c *gin.Context) {
	var req services.CreateDocumentTypeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	if req.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	docType, err := h.service.Create(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create document type: %v", err)})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":       "Document type created successfully",
		"document_type": docType,
	})
}

// GetDocumentType retrieves a document type by ID
// GET /api/v1/document-types/:id
func (h *DocumentTypeHandler) GetDocumentType(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Document type ID is required"})
		return
	}

	// Check if we should include templates
	includeTemplates := c.Query("include_templates") == "true"

	var docType interface{}
	var err error

	if includeTemplates {
		docType, err = h.service.GetWithTemplates(id)
	} else {
		docType, err = h.service.GetByID(id)
	}

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Document type not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"document_type": docType,
	})
}

// GetDocumentTypeByCode retrieves a document type by code
// GET /api/v1/document-types/code/:code
func (h *DocumentTypeHandler) GetDocumentTypeByCode(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Document type code is required"})
		return
	}

	docType, err := h.service.GetByCode(code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Document type not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"document_type": docType,
	})
}

// GetAllDocumentTypes retrieves all document types
// GET /api/v1/document-types
func (h *DocumentTypeHandler) GetAllDocumentTypes(c *gin.Context) {
	category := c.Query("category")
	activeOnly := c.Query("active_only") != "false" // Default to true
	includeTemplates := c.Query("include_templates") == "true"

	var docTypes interface{}
	var err error

	if includeTemplates {
		docTypes, err = h.service.GetAllWithTemplates(category, activeOnly)
	} else {
		docTypes, err = h.service.GetAll(category, activeOnly)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get document types: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"document_types": docTypes,
	})
}

// UpdateDocumentType updates an existing document type
// PUT /api/v1/document-types/:id
func (h *DocumentTypeHandler) UpdateDocumentType(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Document type ID is required"})
		return
	}

	var req services.UpdateDocumentTypeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	docType, err := h.service.Update(id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to update document type: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Document type updated successfully",
		"document_type": docType,
	})
}

// DeleteDocumentType deletes a document type
// DELETE /api/v1/document-types/:id
func (h *DocumentTypeHandler) DeleteDocumentType(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Document type ID is required"})
		return
	}

	if err := h.service.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to delete document type: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Document type deleted successfully",
	})
}

// GetCategories returns all available document type categories
// GET /api/v1/document-types/categories
func (h *DocumentTypeHandler) GetCategories(c *gin.Context) {
	categories := h.service.GetCategories()
	c.JSON(http.StatusOK, gin.H{
		"categories": categories,
	})
}

// AssignTemplateRequest represents the request body for assigning a template
type AssignTemplateRequest struct {
	TemplateID   string `json:"template_id"`
	VariantName  string `json:"variant_name"`
	VariantOrder int    `json:"variant_order"`
}

// AssignTemplate assigns a template to a document type
// POST /api/v1/document-types/:id/templates
func (h *DocumentTypeHandler) AssignTemplate(c *gin.Context) {
	documentTypeID := c.Param("id")
	if documentTypeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Document type ID is required"})
		return
	}

	var req AssignTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	if req.TemplateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "template_id is required"})
		return
	}

	if err := h.service.AssignTemplateToDocumentType(req.TemplateID, documentTypeID, req.VariantName, req.VariantOrder); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to assign template: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Template assigned successfully",
	})
}

// UnassignTemplate removes a template from its document type
// DELETE /api/v1/document-types/:id/templates/:templateId
func (h *DocumentTypeHandler) UnassignTemplate(c *gin.Context) {
	templateID := c.Param("templateId")
	if templateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Template ID is required"})
		return
	}

	if err := h.service.UnassignTemplateFromDocumentType(templateID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to unassign template: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Template unassigned successfully",
	})
}

// GetTemplates retrieves all templates belonging to a document type
// GET /api/v1/document-types/:id/templates
func (h *DocumentTypeHandler) GetTemplates(c *gin.Context) {
	documentTypeID := c.Param("id")
	if documentTypeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Document type ID is required"})
		return
	}

	templates, err := h.service.GetTemplatesByDocumentType(documentTypeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get templates: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"templates": templates,
	})
}

// BulkAssignTemplatesRequest represents the request body for bulk assigning templates
type BulkAssignTemplatesRequest struct {
	Assignments []services.TemplateAssignment `json:"assignments"`
}

// BulkAssignTemplates assigns multiple templates to a document type
// POST /api/v1/document-types/:id/templates/bulk
func (h *DocumentTypeHandler) BulkAssignTemplates(c *gin.Context) {
	documentTypeID := c.Param("id")
	if documentTypeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Document type ID is required"})
		return
	}

	var req BulkAssignTemplatesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	if len(req.Assignments) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least one assignment is required"})
		return
	}

	if err := h.service.BulkAssignTemplates(documentTypeID, req.Assignments); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to assign templates: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("%d templates assigned successfully", len(req.Assignments)),
	})
}

// GetAutoSuggestions returns auto-suggested groupings for unassigned templates
// GET /api/v1/document-types/suggestions
func (h *DocumentTypeHandler) GetAutoSuggestions(c *gin.Context) {
	suggestions, err := h.autoSuggestService.GetAutoSuggestions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get suggestions: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"suggestions": suggestions,
	})
}

// GetSuggestionForTemplate returns a suggestion for a specific template
// GET /api/v1/document-types/suggestions/:templateId
func (h *DocumentTypeHandler) GetSuggestionForTemplate(c *gin.Context) {
	templateID := c.Param("templateId")
	if templateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Template ID is required"})
		return
	}

	suggestion, err := h.autoSuggestService.SuggestForTemplate(templateID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get suggestion: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"suggestion": suggestion,
	})
}

// ApplySuggestion applies a suggested grouping by creating document type and assigning templates
// POST /api/v1/document-types/suggestions/apply
func (h *DocumentTypeHandler) ApplySuggestion(c *gin.Context) {
	var req struct {
		SuggestedName     string `json:"suggested_name"`
		SuggestedCode     string `json:"suggested_code"`
		SuggestedCategory string `json:"suggested_category"`
		ExistingTypeID    string `json:"existing_type_id"`
		Templates         []struct {
			ID              string `json:"id"`
			SuggestedVariant string `json:"suggested_variant"`
			VariantOrder    int    `json:"variant_order"`
		} `json:"templates"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	var documentTypeID string

	// If existing type ID provided, use it; otherwise create new
	if req.ExistingTypeID != "" {
		documentTypeID = req.ExistingTypeID
	} else {
		// Create new document type
		newDocType, err := h.service.Create(&services.CreateDocumentTypeRequest{
			Code:        req.SuggestedCode,
			Name:        req.SuggestedName,
			Category:    req.SuggestedCategory,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create document type: %v", err)})
			return
		}
		documentTypeID = newDocType.ID
	}

	// Assign templates
	assignments := make([]services.TemplateAssignment, len(req.Templates))
	for i, t := range req.Templates {
		assignments[i] = services.TemplateAssignment{
			TemplateID:   t.ID,
			VariantName:  t.SuggestedVariant,
			VariantOrder: t.VariantOrder,
		}
	}

	if err := h.service.BulkAssignTemplates(documentTypeID, assignments); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to assign templates: %v", err)})
		return
	}

	// Get the updated document type
	docType, _ := h.service.GetWithTemplates(documentTypeID)

	c.JSON(http.StatusOK, gin.H{
		"message":       "Suggestion applied successfully",
		"document_type": docType,
	})
}

// AutoGroupAll automatically groups all unassigned templates into document types
// POST /api/v1/document-types/auto-group
func (h *DocumentTypeHandler) AutoGroupAll(c *gin.Context) {
	createdTypes, err := h.autoSuggestService.AutoGroupAllTemplates()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to auto-group templates: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":              fmt.Sprintf("Auto-grouped templates into %d document types", len(createdTypes)),
		"created_document_types": createdTypes,
	})
}
