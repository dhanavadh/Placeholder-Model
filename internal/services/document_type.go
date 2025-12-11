package services

import (
	"fmt"

	"DF-PLCH/internal"
	"DF-PLCH/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DocumentTypeService struct{}

func NewDocumentTypeService() *DocumentTypeService {
	return &DocumentTypeService{}
}

// CreateDocumentTypeRequest contains fields for creating a document type
type CreateDocumentTypeRequest struct {
	Code           string `json:"code"`
	Name           string `json:"name"`
	NameEN         string `json:"name_en"`
	Description    string `json:"description"`
	OriginalSource string `json:"original_source"`
	Category       string `json:"category"`
	Icon           string `json:"icon"`
	Color          string `json:"color"`
	SortOrder      int    `json:"sort_order"`
	Metadata       string `json:"metadata"`
}

// UpdateDocumentTypeRequest contains fields for updating a document type
type UpdateDocumentTypeRequest struct {
	Code           string `json:"code"`
	Name           string `json:"name"`
	NameEN         string `json:"name_en"`
	Description    string `json:"description"`
	OriginalSource string `json:"original_source"`
	Category       string `json:"category"`
	Icon           string `json:"icon"`
	Color          string `json:"color"`
	SortOrder      *int   `json:"sort_order"`
	IsActive       *bool  `json:"is_active"`
	Metadata       string `json:"metadata"`
}

// TemplateAssignment represents a template assignment to a document type
type TemplateAssignment struct {
	TemplateID   string `json:"template_id"`
	VariantName  string `json:"variant_name"`
	VariantOrder int    `json:"variant_order"`
}

// Create creates a new document type
func (s *DocumentTypeService) Create(req *CreateDocumentTypeRequest) (*models.DocumentType, error) {
	// Check if code already exists
	var existing models.DocumentType
	if err := internal.DB.Where("code = ?", req.Code).First(&existing).Error; err == nil {
		return nil, fmt.Errorf("document type with code '%s' already exists", req.Code)
	}

	// Default metadata to empty JSON object if not provided
	metadata := req.Metadata
	if metadata == "" {
		metadata = "{}"
	}

	docType := &models.DocumentType{
		ID:             uuid.New().String(),
		Code:           req.Code,
		Name:           req.Name,
		NameEN:         req.NameEN,
		Description:    req.Description,
		OriginalSource: req.OriginalSource,
		Category:       req.Category,
		Icon:           req.Icon,
		Color:          req.Color,
		SortOrder:      req.SortOrder,
		IsActive:       true,
		Metadata:       metadata,
	}

	if err := internal.DB.Create(docType).Error; err != nil {
		return nil, fmt.Errorf("failed to create document type: %w", err)
	}

	return docType, nil
}

// GetByID retrieves a document type by ID
func (s *DocumentTypeService) GetByID(id string) (*models.DocumentType, error) {
	var docType models.DocumentType
	if err := internal.DB.First(&docType, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("document type not found: %w", err)
	}
	return &docType, nil
}

// GetByCode retrieves a document type by code
func (s *DocumentTypeService) GetByCode(code string) (*models.DocumentType, error) {
	var docType models.DocumentType
	if err := internal.DB.First(&docType, "code = ?", code).Error; err != nil {
		return nil, fmt.Errorf("document type not found: %w", err)
	}
	return &docType, nil
}

// GetAll retrieves all document types with optional filtering
func (s *DocumentTypeService) GetAll(category string, activeOnly bool) ([]models.DocumentType, error) {
	var docTypes []models.DocumentType
	query := internal.DB.Order("sort_order ASC, name ASC")

	if activeOnly {
		query = query.Where("is_active = ?", true)
	}

	if category != "" {
		query = query.Where("category = ?", category)
	}

	if err := query.Find(&docTypes).Error; err != nil {
		return nil, fmt.Errorf("failed to get document types: %w", err)
	}

	return docTypes, nil
}

// GetWithTemplates retrieves a document type with its templates
func (s *DocumentTypeService) GetWithTemplates(id string) (*models.DocumentType, error) {
	var docType models.DocumentType
	if err := internal.DB.Preload("Templates", func(db *gorm.DB) *gorm.DB {
		return db.Order("variant_order ASC")
	}).First(&docType, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("document type not found: %w", err)
	}
	return &docType, nil
}

// GetAllWithTemplates retrieves all document types with their templates
func (s *DocumentTypeService) GetAllWithTemplates(category string, activeOnly bool) ([]models.DocumentType, error) {
	var docTypes []models.DocumentType
	query := internal.DB.Preload("Templates", func(db *gorm.DB) *gorm.DB {
		return db.Order("variant_order ASC")
	}).Order("sort_order ASC, name ASC")

	if activeOnly {
		query = query.Where("is_active = ?", true)
	}

	if category != "" {
		query = query.Where("category = ?", category)
	}

	if err := query.Find(&docTypes).Error; err != nil {
		return nil, fmt.Errorf("failed to get document types with templates: %w", err)
	}

	return docTypes, nil
}

// Update updates an existing document type
func (s *DocumentTypeService) Update(id string, req *UpdateDocumentTypeRequest) (*models.DocumentType, error) {
	docType, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	// Check if new code conflicts with existing one (if code is being changed)
	if req.Code != "" && req.Code != docType.Code {
		var existing models.DocumentType
		if err := internal.DB.Where("code = ? AND id != ?", req.Code, id).First(&existing).Error; err == nil {
			return nil, fmt.Errorf("document type with code '%s' already exists", req.Code)
		}
		docType.Code = req.Code
	}

	// Update fields
	if req.Name != "" {
		docType.Name = req.Name
	}
	if req.NameEN != "" {
		docType.NameEN = req.NameEN
	}
	if req.Description != "" {
		docType.Description = req.Description
	}
	if req.OriginalSource != "" {
		docType.OriginalSource = req.OriginalSource
	}
	if req.Category != "" {
		docType.Category = req.Category
	}
	if req.Icon != "" {
		docType.Icon = req.Icon
	}
	if req.Color != "" {
		docType.Color = req.Color
	}
	if req.SortOrder != nil {
		docType.SortOrder = *req.SortOrder
	}
	if req.IsActive != nil {
		docType.IsActive = *req.IsActive
	}
	if req.Metadata != "" {
		docType.Metadata = req.Metadata
	}

	if err := internal.DB.Save(docType).Error; err != nil {
		return nil, fmt.Errorf("failed to update document type: %w", err)
	}

	return docType, nil
}

// Delete soft-deletes a document type
func (s *DocumentTypeService) Delete(id string) error {
	docType, err := s.GetByID(id)
	if err != nil {
		return err
	}

	// Check if there are templates linked to this document type
	var count int64
	if err := internal.DB.Model(&models.Template{}).Where("document_type_id = ?", id).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to check linked templates: %w", err)
	}

	if count > 0 {
		return fmt.Errorf("cannot delete document type with %d linked templates. Unlink templates first", count)
	}

	if err := internal.DB.Delete(docType).Error; err != nil {
		return fmt.Errorf("failed to delete document type: %w", err)
	}

	return nil
}

// GetCategories returns all available document type categories
func (s *DocumentTypeService) GetCategories() []string {
	return []string{
		string(models.CategoryIdentification),
		string(models.CategoryCertificate),
		string(models.CategoryContract),
		string(models.CategoryApplication),
		string(models.CategoryFinancial),
		string(models.CategoryGovernment),
		string(models.CategoryEducation),
		string(models.CategoryMedical),
		string(models.CategoryOther),
	}
}

// AssignTemplateToDocumentType assigns a template to a document type
func (s *DocumentTypeService) AssignTemplateToDocumentType(templateID, documentTypeID, variantName string, variantOrder int) error {
	// Verify document type exists
	if _, err := s.GetByID(documentTypeID); err != nil {
		return err
	}

	// Update template
	result := internal.DB.Model(&models.Template{}).Where("id = ?", templateID).Updates(map[string]interface{}{
		"document_type_id": documentTypeID,
		"variant_name":     variantName,
		"variant_order":    variantOrder,
	})

	if result.Error != nil {
		return fmt.Errorf("failed to assign template to document type: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("template not found")
	}

	return nil
}

// UnassignTemplateFromDocumentType removes a template from its document type
func (s *DocumentTypeService) UnassignTemplateFromDocumentType(templateID string) error {
	result := internal.DB.Model(&models.Template{}).Where("id = ?", templateID).Updates(map[string]interface{}{
		"document_type_id": nil,
		"variant_name":     "",
		"variant_order":    0,
	})

	if result.Error != nil {
		return fmt.Errorf("failed to unassign template from document type: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("template not found")
	}

	return nil
}

// GetTemplatesByDocumentType retrieves all templates belonging to a document type
func (s *DocumentTypeService) GetTemplatesByDocumentType(documentTypeID string) ([]models.Template, error) {
	var templates []models.Template
	if err := internal.DB.Where("document_type_id = ?", documentTypeID).Order("variant_order ASC").Find(&templates).Error; err != nil {
		return nil, fmt.Errorf("failed to get templates: %w", err)
	}
	return templates, nil
}

// BulkAssignTemplates assigns multiple templates to a document type
func (s *DocumentTypeService) BulkAssignTemplates(documentTypeID string, assignments []TemplateAssignment) error {
	// Verify document type exists
	if _, err := s.GetByID(documentTypeID); err != nil {
		return err
	}

	// Update each template
	for _, assignment := range assignments {
		result := internal.DB.Model(&models.Template{}).Where("id = ?", assignment.TemplateID).Updates(map[string]interface{}{
			"document_type_id": documentTypeID,
			"variant_name":     assignment.VariantName,
			"variant_order":    assignment.VariantOrder,
		})

		if result.Error != nil {
			return fmt.Errorf("failed to assign template %s: %w", assignment.TemplateID, result.Error)
		}
	}

	return nil
}
