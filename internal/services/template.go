package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"DF-PLCH/internal"
	"DF-PLCH/internal/models"
	"DF-PLCH/internal/processor"
	"DF-PLCH/internal/storage"
	"DF-PLCH/internal/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TemplateService struct {
	storageClient     storage.StorageClient
	conversionService *ConversionService
}

func NewTemplateService(storageClient storage.StorageClient) *TemplateService {
	return &TemplateService{
		storageClient: storageClient,
	}
}

// SetConversionService sets the conversion service for automatic HTML/PDF generation
func (s *TemplateService) SetConversionService(convService *ConversionService) {
	s.conversionService = convService
}

// generateFieldDefinitionsFromDatabase generates field definitions using Data Types from database
// Each DataType has a pattern field for matching placeholder names
// Placeholders are ordered by their position in the document (first to last)
// Entity/Group is set to "general" by default - users choose groups themselves
func generateFieldDefinitionsFromDatabase(placeholders []string) map[string]utils.FieldDefinition {
	definitions := make(map[string]utils.FieldDefinition)

	// Load data types from database (sorted by priority DESC - higher priority first)
	var dataTypes []models.DataType
	internal.DB.Where("is_active = ?", true).Order("priority DESC").Find(&dataTypes)

	for i, placeholder := range placeholders {
		key := strings.ReplaceAll(placeholder, "{{", "")
		key = strings.ReplaceAll(key, "}}", "")

		// Apply data type rules without entity auto-detection
		// Users will choose groups for each placeholder themselves
		definition := applyDataTypeRulesWithOrder(key, placeholder, dataTypes, i)
		definitions[key] = definition
	}

	return definitions
}

// applyDataTypeRulesWithOrder applies data type patterns to a placeholder with order
// Entity is set to "general" by default - users choose groups themselves
func applyDataTypeRulesWithOrder(key, placeholder string, dataTypes []models.DataType, order int) utils.FieldDefinition {
	// Default definition with order (first to last based on document position)
	// Entity defaults to "general" - users will choose groups themselves
	definition := utils.FieldDefinition{
		Placeholder: placeholder,
		DataType:    utils.DataTypeText,
		Entity:      utils.EntityGeneral,
		InputType:   utils.InputTypeText,
		Order:       order, // Set order based on position in document
	}

	// Try each data type pattern (already sorted by priority DESC)
	for _, dt := range dataTypes {
		if !dt.IsActive || dt.Pattern == "" {
			continue
		}

		re, err := regexp.Compile(dt.Pattern)
		if err != nil {
			continue
		}

		if !re.MatchString(key) {
			continue
		}

		// Pattern matched - apply this data type
		definition.DataType = utils.DataType(dt.Code)

		// Set input type from data type
		if dt.InputType != "" {
			definition.InputType = utils.InputType(dt.InputType)
		}

		// Parse validation from data type
		if dt.Validation != "" && dt.Validation != "{}" {
			var validation utils.FieldValidation
			if err := json.Unmarshal([]byte(dt.Validation), &validation); err == nil {
				definition.Validation = &validation
			}
		}

		// Get options from data type if input type is select
		if definition.InputType == utils.InputTypeSelect {
			if dt.Options != "" && dt.Options != "{}" {
				var options []string
				if err := json.Unmarshal([]byte(dt.Options), &options); err == nil && len(options) > 0 {
					if definition.Validation == nil {
						definition.Validation = &utils.FieldValidation{}
					}
					definition.Validation.Options = options
				}
			}
		}

		// Data type matched, stop processing
		break
	}

	return definition
}

// applyDataTypeRules applies data type patterns and entity rules to a placeholder
// Deprecated: Use applyDataTypeRulesWithOrder instead
func applyDataTypeRules(key, placeholder string, dataTypes []models.DataType, entityRules []models.EntityRule) utils.FieldDefinition {
	// Default definition
	definition := utils.FieldDefinition{
		Placeholder: placeholder,
		DataType:    utils.DataTypeText,
		Entity:      utils.EntityGeneral,
		InputType:   utils.InputTypeText,
	}

	// Apply entity rules first
	if len(entityRules) > 0 {
		definition.Entity = applyEntityRules(key, entityRules)
	}

	// Try each data type pattern (already sorted by priority DESC)
	for _, dt := range dataTypes {
		if !dt.IsActive || dt.Pattern == "" {
			continue
		}

		re, err := regexp.Compile(dt.Pattern)
		if err != nil {
			continue
		}

		if !re.MatchString(key) {
			continue
		}

		// Pattern matched - apply this data type
		definition.DataType = utils.DataType(dt.Code)

		// Set input type from data type
		if dt.InputType != "" {
			definition.InputType = utils.InputType(dt.InputType)
		}

		// Parse validation from data type
		if dt.Validation != "" && dt.Validation != "{}" {
			var validation utils.FieldValidation
			if err := json.Unmarshal([]byte(dt.Validation), &validation); err == nil {
				definition.Validation = &validation
			}
		}

		// Get options from data type if input type is select
		if definition.InputType == utils.InputTypeSelect {
			if dt.Options != "" && dt.Options != "{}" {
				var options []string
				if err := json.Unmarshal([]byte(dt.Options), &options); err == nil && len(options) > 0 {
					if definition.Validation == nil {
						definition.Validation = &utils.FieldValidation{}
					}
					definition.Validation.Options = options
				}
			}
		}

		// Data type matched, stop processing
		break
	}

	return definition
}

// applyEntityRules applies entity rules to determine the entity type
func applyEntityRules(key string, entityRules []models.EntityRule) utils.Entity {
	for _, rule := range entityRules {
		if !rule.IsActive {
			continue
		}

		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			continue
		}

		if re.MatchString(key) {
			return utils.Entity(rule.Code)
		}
	}

	return utils.EntityGeneral
}

func (s *TemplateService) UploadTemplate(ctx context.Context, file multipart.File, header *multipart.FileHeader) (*models.Template, error) {
	return s.UploadTemplateWithMetadata(ctx, file, header, header.Filename, "", "", nil)
}

func (s *TemplateService) UploadTemplateWithMetadata(ctx context.Context, file multipart.File, header *multipart.FileHeader, fileName, description, author string, aliases map[string]string) (*models.Template, error) {
	return s.UploadTemplateWithHTMLPreview(ctx, file, header, nil, nil, fileName, description, author, aliases)
}

func (s *TemplateService) UploadTemplateWithHTMLPreview(ctx context.Context, file multipart.File, header *multipart.FileHeader, htmlFile multipart.File, htmlHeader *multipart.FileHeader, fileName, description, author string, aliases map[string]string) (*models.Template, error) {
	templateID := uuid.New().String()
	objectName := storage.GenerateObjectName(templateID, header.Filename)

	// Upload DOCX to GCS
	result, err := s.storageClient.UploadFile(ctx, file, objectName, header.Header.Get("Content-Type"))
	if err != nil {
		return nil, fmt.Errorf("failed to upload to GCS: %w", err)
	}

	// Create temp file for processing
	file.Seek(0, 0) // Reset file pointer
	tempFile, err := s.createTempFile(file)
	if err != nil {
		// Cleanup GCS file on failure
		s.storageClient.DeleteFile(ctx, objectName)
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer s.cleanupTempFile(tempFile)

	// Upload HTML preview file if provided, otherwise auto-generate
	var htmlObjectName string
	if htmlFile != nil && htmlHeader != nil {
		// User provided HTML file
		htmlObjectName = storage.GenerateObjectName(templateID, htmlHeader.Filename)
		_, err := s.storageClient.UploadFile(ctx, htmlFile, htmlObjectName, "text/html")
		if err != nil {
			s.storageClient.DeleteFile(ctx, objectName)
			return nil, fmt.Errorf("failed to upload HTML preview to GCS: %w", err)
		}
		fmt.Printf("[INFO] Uploaded user-provided HTML preview for template %s\n", templateID)
	} else {
		// Auto-generate HTML preview from DOCX
		var htmlContent []byte
		var htmlGenErr error
		htmlGenerated := false

		// Try remote service (Gotenberg) first if available
		if s.conversionService != nil && s.conversionService.IsHTMLConversionAvailable() {
			fmt.Printf("[INFO] Auto-generating HTML preview for template %s using remote service\n", templateID)
			htmlContent, htmlGenErr = s.conversionService.ConvertDocxToHTML(ctx, tempFile)
			if htmlGenErr != nil {
				fmt.Printf("[WARNING] Failed to auto-generate HTML preview via remote service: %v\n", htmlGenErr)
			} else {
				htmlGenerated = true
			}
		}

		// Fallback to local LibreOffice if remote failed or not available
		if !htmlGenerated && processor.IsLibreOfficeAvailable() {
			fmt.Printf("[INFO] Auto-generating HTML preview for template %s using local LibreOffice\n", templateID)
			htmlContent, htmlGenErr = processor.ConvertToHTMLBytes(tempFile)
			if htmlGenErr != nil {
				fmt.Printf("[WARNING] Failed to auto-generate HTML preview via local LibreOffice: %v\n", htmlGenErr)
			} else {
				htmlGenerated = true
			}
		}

		// Upload generated HTML if successful
		if htmlGenerated && len(htmlContent) > 0 {
			htmlFileName := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename)) + ".html"
			htmlObjectName = storage.GenerateObjectName(templateID, htmlFileName)
			htmlReader := bytes.NewReader(htmlContent)
			_, err := s.storageClient.UploadFile(ctx, io.NopCloser(htmlReader), htmlObjectName, "text/html")
			if err != nil {
				fmt.Printf("[WARNING] Failed to upload auto-generated HTML preview: %v\n", err)
				htmlObjectName = "" // Reset on failure
			} else {
				fmt.Printf("[INFO] Successfully auto-generated HTML preview for template %s\n", templateID)
			}
		}
	}

	// Auto-generate PDF preview and thumbnail
	var pdfObjectName string
	var thumbnailObjectName string
	var pdfContent []byte
	pdfGenerated := false

	// Try remote service (Gotenberg) first if available
	if s.conversionService != nil && s.conversionService.IsPDFConversionAvailable() {
		fmt.Printf("[INFO] Auto-generating PDF preview for template %s using remote service\n", templateID)
		var pdfErr error
		pdfContent, pdfErr = s.conversionService.ConvertDocxToPDF(ctx, tempFile)
		if pdfErr != nil {
			fmt.Printf("[WARNING] Failed to auto-generate PDF preview via remote service: %v\n", pdfErr)
		} else {
			pdfGenerated = true
		}
	}

	// Fallback to local LibreOffice if remote failed or not available
	if !pdfGenerated && processor.IsLibreOfficeAvailable() {
		fmt.Printf("[INFO] Auto-generating PDF preview for template %s using local LibreOffice\n", templateID)

		// Create temp output file for PDF
		pdfTempFile, err := os.CreateTemp("", "*.pdf")
		if err != nil {
			fmt.Printf("[WARNING] Failed to create temp PDF file: %v\n", err)
		} else {
			pdfTempPath := pdfTempFile.Name()
			pdfTempFile.Close()
			defer os.Remove(pdfTempPath)

			err = processor.ConvertToPDF(tempFile, pdfTempPath)
			if err != nil {
				fmt.Printf("[WARNING] Failed to auto-generate PDF preview via local LibreOffice: %v\n", err)
			} else {
				// Read the generated PDF
				pdfContent, err = os.ReadFile(pdfTempPath)
				if err != nil {
					fmt.Printf("[WARNING] Failed to read generated PDF: %v\n", err)
				} else {
					pdfGenerated = true
				}
			}
		}
	}

	// Upload generated PDF if successful
	if pdfGenerated && len(pdfContent) > 0 {
		pdfFileName := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename)) + ".pdf"
		pdfObjectName = storage.GenerateObjectName(templateID, pdfFileName)
		pdfReader := bytes.NewReader(pdfContent)
		_, err := s.storageClient.UploadFile(ctx, io.NopCloser(pdfReader), pdfObjectName, "application/pdf")
		if err != nil {
			fmt.Printf("[WARNING] Failed to upload auto-generated PDF preview: %v\n", err)
			pdfObjectName = "" // Reset on failure
		} else {
			fmt.Printf("[INFO] Successfully auto-generated PDF preview for template %s\n", templateID)

			// Generate thumbnail from PDF (try remote first, then local)
			var thumbnailContent []byte
			thumbnailGenerated := false

			if s.conversionService != nil && s.conversionService.IsThumbnailGenerationAvailable() {
				fmt.Printf("[INFO] Generating thumbnail for template %s using remote service\n", templateID)
				pdfTempFile, err := os.CreateTemp("", "*.pdf")
				if err == nil {
					pdfTempFile.Write(pdfContent)
					pdfTempFile.Close()
					defer os.Remove(pdfTempFile.Name())

					thumbnailContent, err = s.conversionService.GenerateThumbnailFromPDFWithQuality(ctx, pdfTempFile.Name(), 600, ThumbnailQualityHD)
					if err != nil {
						fmt.Printf("[WARNING] Failed to generate thumbnail via remote service: %v\n", err)
					} else {
						thumbnailGenerated = true
					}
				}
			}

			// Upload thumbnail if generated
			if thumbnailGenerated && len(thumbnailContent) > 0 {
				thumbnailFileName := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename)) + "_thumb.png"
				thumbnailObjectName = storage.GenerateObjectName(templateID, thumbnailFileName)
				thumbnailReader := bytes.NewReader(thumbnailContent)
				_, err := s.storageClient.UploadFile(ctx, io.NopCloser(thumbnailReader), thumbnailObjectName, "image/png")
				if err != nil {
					fmt.Printf("[WARNING] Failed to upload thumbnail: %v\n", err)
					thumbnailObjectName = ""
				} else {
					fmt.Printf("[INFO] Successfully generated thumbnail for template %s\n", templateID)
				}
			}
		}
	}

	// Process DOCX to extract placeholders
	proc := processor.NewDocxProcessor(tempFile, "")
	if err := proc.UnzipDocx(); err != nil {
		s.storageClient.DeleteFile(ctx, objectName)
		return nil, fmt.Errorf("failed to process document: %w", err)
	}
	defer proc.Cleanup()

	placeholders, err := proc.ExtractPlaceholders()
	if err != nil {
		s.storageClient.DeleteFile(ctx, objectName)
		return nil, fmt.Errorf("failed to extract placeholders: %w", err)
	}

	// Detect page orientation
	pageOrientation := models.OrientationPortrait
	isLandscape, err := proc.DetectOrientation()
	if err != nil {
		fmt.Printf("[WARNING] Failed to detect page orientation: %v\n", err)
	} else if isLandscape {
		pageOrientation = models.OrientationLandscape
		fmt.Printf("[INFO] Detected landscape orientation for template %s\n", templateID)
	}

	// Convert placeholders to JSON
	placeholdersJSON, err := json.Marshal(placeholders)
	if err != nil {
		s.storageClient.DeleteFile(ctx, objectName)
		return nil, fmt.Errorf("failed to marshal placeholders: %w", err)
	}

	// Auto-detect field definitions from placeholders using database rules
	fieldDefinitions := generateFieldDefinitionsFromDatabase(placeholders)
	fieldDefinitionsJSON, err := json.Marshal(fieldDefinitions)
	if err != nil {
		s.storageClient.DeleteFile(ctx, objectName)
		return nil, fmt.Errorf("failed to marshal field definitions: %w", err)
	}

	// Save to database
	template := &models.Template{
		ID:               templateID,
		Filename:         header.Filename,
		OriginalName:     header.Filename,
		DisplayName:      fileName,
		Description:      description,
		Author:           author,
		GCSPath:          objectName,
		GCSPathHTML:      htmlObjectName,
		GCSPathPDF:       pdfObjectName,
		GCSPathThumbnail: thumbnailObjectName,
		FileSize:         result.Size,
		MimeType:         header.Header.Get("Content-Type"),
		Placeholders:     string(placeholdersJSON),
		FieldDefinitions: string(fieldDefinitionsJSON),
		PageOrientation:  pageOrientation,
	}

	// Convert aliases to JSON (if provided)
	if aliases != nil && len(aliases) > 0 {
		aliasesBytes, err := json.Marshal(aliases)
		if err != nil {
			s.storageClient.DeleteFile(ctx, objectName)
			return nil, fmt.Errorf("failed to marshal aliases: %w", err)
		}
		template.Aliases = string(aliasesBytes)
	} else {
		// Store empty JSON object for no aliases (valid JSON)
		template.Aliases = "{}"
	}

	if err := internal.DB.Create(template).Error; err != nil {
		s.storageClient.DeleteFile(ctx, objectName)
		return nil, fmt.Errorf("failed to save template metadata: %w", err)
	}

	return template, nil
}

func (s *TemplateService) GetTemplate(templateID string) (*models.Template, error) {
	var template models.Template
	if err := internal.DB.First(&template, "id = ?", templateID).Error; err != nil {
		return nil, fmt.Errorf("template not found: %w", err)
	}
	return &template, nil
}

func (s *TemplateService) GetAllTemplates() ([]models.Template, error) {
	var templates []models.Template
	if err := internal.DB.Find(&templates).Error; err != nil {
		return nil, fmt.Errorf("failed to get templates: %w", err)
	}
	return templates, nil
}

// TemplateFilter represents filter options for querying templates
type TemplateFilter struct {
	DocumentTypeID      string // Filter by document type ID
	Type                string // Filter by template type (official, private, community)
	Tier                string // Filter by tier (free, basic, premium, enterprise)
	Category            string // Filter by category
	IsVerified          *bool  // Filter by verification status
	Search              string // Search in name, description, author
	IncludeDocumentType bool   // Whether to preload document type
	Sort                string // Sort order: "popular" (by usage), "recent" (by created_at), "name" (alphabetical)
	Limit               int    // Limit number of results (0 = no limit)
}

// GetTemplatesWithFilter retrieves templates with filtering options
func (s *TemplateService) GetTemplatesWithFilter(filter *TemplateFilter) ([]models.Template, error) {
	var templates []models.Template
	query := internal.DB.Model(&models.Template{})

	// Apply filters
	if filter.DocumentTypeID != "" {
		query = query.Where("document_type_id = ?", filter.DocumentTypeID)
	}
	if filter.Type != "" {
		query = query.Where("type = ?", filter.Type)
	}
	if filter.Tier != "" {
		query = query.Where("tier = ?", filter.Tier)
	}
	if filter.Category != "" {
		query = query.Where("category = ?", filter.Category)
	}
	if filter.IsVerified != nil {
		query = query.Where("is_verified = ?", *filter.IsVerified)
	}
	if filter.Search != "" {
		searchPattern := "%" + filter.Search + "%"
		query = query.Where("display_name ILIKE ? OR description ILIKE ? OR author ILIKE ?", searchPattern, searchPattern, searchPattern)
	}

	// Preload document type if requested
	if filter.IncludeDocumentType {
		query = query.Preload("DocumentType")
	}

	// Apply sorting
	switch filter.Sort {
	case "popular":
		// Sort by actual document count - most used first
		// Uses documents table which has real records of generated documents
		query = query.
			Joins("LEFT JOIN (SELECT template_id, COUNT(*) as doc_count FROM documents WHERE deleted_at IS NULL GROUP BY template_id) doc_counts ON doc_counts.template_id = id").
			Order("COALESCE(doc_counts.doc_count, 0) DESC, created_at DESC")
	case "name":
		query = query.Order("COALESCE(NULLIF(display_name, ''), filename) ASC")
	case "recent":
		query = query.Order("created_at DESC")
	default:
		// Default: order by variant_order within document type, then by created_at
		query = query.Order("document_type_id ASC, variant_order ASC, created_at DESC")
	}

	// Apply limit if specified
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}

	if err := query.Find(&templates).Error; err != nil {
		return nil, fmt.Errorf("failed to get templates: %w", err)
	}

	return templates, nil
}

// GetTemplatesGroupedByDocumentType retrieves all templates grouped by their document type
func (s *TemplateService) GetTemplatesGroupedByDocumentType() ([]models.DocumentType, []models.Template, error) {
	// Get all document types with their templates
	var documentTypes []models.DocumentType
	if err := internal.DB.Preload("Templates", func(db *gorm.DB) *gorm.DB {
		return db.Order("variant_order ASC")
	}).Order("sort_order ASC, name ASC").Find(&documentTypes).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to get document types: %w", err)
	}

	// Get orphan templates (templates without a document type)
	var orphanTemplates []models.Template
	if err := internal.DB.Where("document_type_id IS NULL OR document_type_id = ''").Order("created_at DESC").Find(&orphanTemplates).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to get orphan templates: %w", err)
	}

	return documentTypes, orphanTemplates, nil
}

func (s *TemplateService) GetPlaceholders(templateID string) ([]string, error) {
	template, err := s.GetTemplate(templateID)
	if err != nil {
		return nil, err
	}

	var placeholders []string
	if err := json.Unmarshal([]byte(template.Placeholders), &placeholders); err != nil {
		return nil, fmt.Errorf("failed to unmarshal placeholders: %w", err)
	}

	return placeholders, nil
}

func (s *TemplateService) DeleteTemplate(ctx context.Context, templateID string) error {
	template, err := s.GetTemplate(templateID)
	if err != nil {
		return err
	}

	// Delete from GCS
	if err := s.storageClient.DeleteFile(ctx, template.GCSPath); err != nil {
		// Log error but continue with database deletion
		fmt.Printf("Warning: failed to delete GCS file %s: %v\n", template.GCSPath, err)
	}

	// Soft delete from database
	return internal.DB.Delete(template).Error
}

// TemplateUpdateRequest contains all fields that can be updated on a template
type TemplateUpdateRequest struct {
	DisplayName    string
	Name           string
	Description    string
	Author         string
	Category       string
	OriginalSource string
	Remarks        string
	IsVerified     *bool
	IsAIAvailable  *bool
	Type           string
	Tier           string
	Group          string
	Aliases        map[string]string
}

func (s *TemplateService) UpdateTemplate(ctx context.Context, templateID string, req *TemplateUpdateRequest, htmlFile multipart.File, htmlHeader *multipart.FileHeader) (*models.Template, error) {
	template, err := s.GetTemplate(templateID)
	if err != nil {
		return nil, err
	}

	// Update basic fields
	template.DisplayName = req.DisplayName
	template.Description = req.Description
	template.Author = req.Author

	// Update new fields
	if req.Name != "" {
		template.Name = req.Name
	}
	if req.Category != "" {
		template.Category = models.Category(req.Category)
	}
	if req.OriginalSource != "" {
		template.OriginalSource = req.OriginalSource
	}
	if req.Remarks != "" {
		template.Remarks = req.Remarks
	}
	if req.IsVerified != nil {
		template.IsVerified = *req.IsVerified
	}
	if req.IsAIAvailable != nil {
		template.IsAIAvailable = *req.IsAIAvailable
	}
	if req.Type != "" {
		template.Type = models.TemplateType(req.Type)
	}
	if req.Tier != "" {
		template.Tier = models.Tier(req.Tier)
	}
	if req.Group != "" {
		template.Group = req.Group
	}

	// Convert aliases to JSON (if provided)
	if req.Aliases != nil {
		aliasesJSON, err := json.Marshal(req.Aliases)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal aliases: %w", err)
		}
		template.Aliases = string(aliasesJSON)
	}

	// Upload HTML preview file if provided
	if htmlFile != nil && htmlHeader != nil {
		htmlObjectName := storage.GenerateObjectName(templateID, htmlHeader.Filename)
		_, err := s.storageClient.UploadFile(ctx, htmlFile, htmlObjectName, "text/html")
		if err != nil {
			return nil, fmt.Errorf("failed to upload HTML preview to GCS: %w", err)
		}

		// Delete old HTML file if exists
		if template.GCSPathHTML != "" {
			if err := s.storageClient.DeleteFile(ctx, template.GCSPathHTML); err != nil {
				// Log but don't fail on deletion error
				fmt.Printf("Warning: failed to delete old HTML file %s: %v\n", template.GCSPathHTML, err)
			}
		}

		template.GCSPathHTML = htmlObjectName
	}

	// Save to database
	if err := internal.DB.Save(template).Error; err != nil {
		return nil, fmt.Errorf("failed to update template: %w", err)
	}

	return template, nil
}

// ReplaceTemplateFiles replaces the DOCX and/or HTML files for an existing template
// If regenerateFields is true, field definitions will be regenerated from the new placeholders
// HTML and PDF previews are auto-generated from the new DOCX file
func (s *TemplateService) ReplaceTemplateFiles(ctx context.Context, templateID string, docxFile multipart.File, docxHeader *multipart.FileHeader, htmlFile multipart.File, htmlHeader *multipart.FileHeader, thumbnailFile multipart.File, thumbnailHeader *multipart.FileHeader, regenerateFields bool) (*models.Template, error) {
	template, err := s.GetTemplate(templateID)
	if err != nil {
		return nil, err
	}

	// Handle DOCX file replacement
	if docxFile != nil && docxHeader != nil {
		// Validate file extension
		if ext := strings.ToLower(filepath.Ext(docxHeader.Filename)); ext != ".docx" {
			return nil, fmt.Errorf("invalid file type: expected .docx, got %s", ext)
		}

		// Upload new DOCX file
		objectName := storage.GenerateObjectName(templateID, docxHeader.Filename)
		result, err := s.storageClient.UploadFile(ctx, docxFile, objectName, "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
		if err != nil {
			return nil, fmt.Errorf("failed to upload DOCX file: %w", err)
		}

		// Create temp file for processing
		docxFile.Seek(0, 0) // Reset file pointer
		tempFile, err := s.createTempFile(docxFile)
		if err != nil {
			s.storageClient.DeleteFile(ctx, objectName)
			return nil, fmt.Errorf("failed to create temp file: %w", err)
		}
		defer s.cleanupTempFile(tempFile)

		// Auto-generate HTML preview from DOCX (unless user provides HTML file)
		if htmlFile == nil {
			var htmlContent []byte
			var htmlGenErr error
			htmlGenerated := false

			// Try remote service (Gotenberg) first if available
			if s.conversionService != nil && s.conversionService.IsHTMLConversionAvailable() {
				fmt.Printf("[INFO] Auto-generating HTML preview for template %s using remote service\n", templateID)
				htmlContent, htmlGenErr = s.conversionService.ConvertDocxToHTML(ctx, tempFile)
				if htmlGenErr != nil {
					fmt.Printf("[WARNING] Failed to auto-generate HTML preview via remote service: %v\n", htmlGenErr)
				} else {
					htmlGenerated = true
				}
			}

			// Fallback to local LibreOffice if remote failed or not available
			if !htmlGenerated && processor.IsLibreOfficeAvailable() {
				fmt.Printf("[INFO] Auto-generating HTML preview for template %s using local LibreOffice\n", templateID)
				htmlContent, htmlGenErr = processor.ConvertToHTMLBytes(tempFile)
				if htmlGenErr != nil {
					fmt.Printf("[WARNING] Failed to auto-generate HTML preview via local LibreOffice: %v\n", htmlGenErr)
				} else {
					htmlGenerated = true
				}
			}

			// Upload generated HTML if successful
			if htmlGenerated && len(htmlContent) > 0 {
				// Delete old HTML file
				if template.GCSPathHTML != "" {
					if err := s.storageClient.DeleteFile(ctx, template.GCSPathHTML); err != nil {
						fmt.Printf("Warning: failed to delete old HTML file %s: %v\n", template.GCSPathHTML, err)
					}
				}

				// Upload generated HTML
				htmlFileName := strings.TrimSuffix(docxHeader.Filename, filepath.Ext(docxHeader.Filename)) + ".html"
				htmlObjectName := storage.GenerateObjectName(templateID, htmlFileName)
				htmlReader := bytes.NewReader(htmlContent)
				_, err := s.storageClient.UploadFile(ctx, io.NopCloser(htmlReader), htmlObjectName, "text/html")
				if err != nil {
					fmt.Printf("[WARNING] Failed to upload auto-generated HTML preview: %v\n", err)
				} else {
					template.GCSPathHTML = htmlObjectName
					fmt.Printf("[INFO] Successfully auto-generated HTML preview for template %s\n", templateID)
				}
			}
		}

		// Auto-generate PDF preview and thumbnail from DOCX
		var pdfContent []byte
		pdfGenerated := false

		// Try remote service (Gotenberg) first if available
		if s.conversionService != nil && s.conversionService.IsPDFConversionAvailable() {
			fmt.Printf("[INFO] Auto-generating PDF preview for template %s using remote service\n", templateID)
			var pdfErr error
			pdfContent, pdfErr = s.conversionService.ConvertDocxToPDF(ctx, tempFile)
			if pdfErr != nil {
				fmt.Printf("[WARNING] Failed to auto-generate PDF preview via remote service: %v\n", pdfErr)
			} else {
				pdfGenerated = true
			}
		}

		// Fallback to local LibreOffice if remote failed or not available
		if !pdfGenerated && processor.IsLibreOfficeAvailable() {
			fmt.Printf("[INFO] Auto-generating PDF preview for template %s using local LibreOffice\n", templateID)

			// Create temp output file for PDF
			pdfTempFile, err := os.CreateTemp("", "*.pdf")
			if err != nil {
				fmt.Printf("[WARNING] Failed to create temp PDF file: %v\n", err)
			} else {
				pdfTempPath := pdfTempFile.Name()
				pdfTempFile.Close()
				defer os.Remove(pdfTempPath)

				err = processor.ConvertToPDF(tempFile, pdfTempPath)
				if err != nil {
					fmt.Printf("[WARNING] Failed to auto-generate PDF preview via local LibreOffice: %v\n", err)
				} else {
					pdfContent, err = os.ReadFile(pdfTempPath)
					if err != nil {
						fmt.Printf("[WARNING] Failed to read generated PDF: %v\n", err)
					} else {
						pdfGenerated = true
					}
				}
			}
		}

		// Upload generated PDF if successful
		if pdfGenerated && len(pdfContent) > 0 {
			// Delete old PDF file
			if template.GCSPathPDF != "" {
				if err := s.storageClient.DeleteFile(ctx, template.GCSPathPDF); err != nil {
					fmt.Printf("Warning: failed to delete old PDF file %s: %v\n", template.GCSPathPDF, err)
				}
			}

			// Upload generated PDF
			pdfFileName := strings.TrimSuffix(docxHeader.Filename, filepath.Ext(docxHeader.Filename)) + ".pdf"
			pdfObjectName := storage.GenerateObjectName(templateID, pdfFileName)
			pdfReader := bytes.NewReader(pdfContent)
			_, err := s.storageClient.UploadFile(ctx, io.NopCloser(pdfReader), pdfObjectName, "application/pdf")
			if err != nil {
				fmt.Printf("[WARNING] Failed to upload auto-generated PDF preview: %v\n", err)
			} else {
				template.GCSPathPDF = pdfObjectName
				fmt.Printf("[INFO] Successfully auto-generated PDF preview for template %s\n", templateID)

				// Generate thumbnail from PDF (try remote first)
				var thumbnailContent []byte
				thumbnailGenerated := false

				if s.conversionService != nil && s.conversionService.IsThumbnailGenerationAvailable() {
					fmt.Printf("[INFO] Generating thumbnail for template %s using remote service\n", templateID)
					pdfTempFile, err := os.CreateTemp("", "*.pdf")
					if err == nil {
						pdfTempFile.Write(pdfContent)
						pdfTempFile.Close()
						defer os.Remove(pdfTempFile.Name())

						thumbnailContent, err = s.conversionService.GenerateThumbnailFromPDFWithQuality(ctx, pdfTempFile.Name(), 600, ThumbnailQualityHD)
						if err != nil {
							fmt.Printf("[WARNING] Failed to generate thumbnail via remote service: %v\n", err)
						} else {
							thumbnailGenerated = true
						}
					}
				}

				// Upload thumbnail if generated
				if thumbnailGenerated && len(thumbnailContent) > 0 {
					// Delete old thumbnail
					if template.GCSPathThumbnail != "" {
						if err := s.storageClient.DeleteFile(ctx, template.GCSPathThumbnail); err != nil {
							fmt.Printf("Warning: failed to delete old thumbnail %s: %v\n", template.GCSPathThumbnail, err)
						}
					}

					// Upload thumbnail
					thumbnailFileName := strings.TrimSuffix(docxHeader.Filename, filepath.Ext(docxHeader.Filename)) + "_thumb.png"
					thumbnailObjectName := storage.GenerateObjectName(templateID, thumbnailFileName)
					thumbnailReader := bytes.NewReader(thumbnailContent)
					_, err := s.storageClient.UploadFile(ctx, io.NopCloser(thumbnailReader), thumbnailObjectName, "image/png")
					if err != nil {
						fmt.Printf("[WARNING] Failed to upload thumbnail: %v\n", err)
					} else {
						template.GCSPathThumbnail = thumbnailObjectName
						fmt.Printf("[INFO] Successfully generated thumbnail for template %s\n", templateID)
					}
				}
			}
		}

		// Process DOCX to extract placeholders
		proc := processor.NewDocxProcessor(tempFile, "")
		if err := proc.UnzipDocx(); err != nil {
			s.storageClient.DeleteFile(ctx, objectName)
			return nil, fmt.Errorf("failed to process document: %w", err)
		}
		defer proc.Cleanup()

		placeholders, err := proc.ExtractPlaceholders()
		if err != nil {
			s.storageClient.DeleteFile(ctx, objectName)
			return nil, fmt.Errorf("failed to extract placeholders: %w", err)
		}

		// Convert placeholders to JSON
		placeholdersJSON, err := json.Marshal(placeholders)
		if err != nil {
			s.storageClient.DeleteFile(ctx, objectName)
			return nil, fmt.Errorf("failed to marshal placeholders: %w", err)
		}

		// Delete old DOCX file
		if template.GCSPath != "" {
			if err := s.storageClient.DeleteFile(ctx, template.GCSPath); err != nil {
				fmt.Printf("Warning: failed to delete old DOCX file %s: %v\n", template.GCSPath, err)
			}
		}

		// Update template with new DOCX info
		template.GCSPath = objectName
		template.Filename = docxHeader.Filename
		template.FileSize = result.Size
		template.Placeholders = string(placeholdersJSON)

		// Regenerate field definitions if requested
		if regenerateFields {
			fieldDefinitions := generateFieldDefinitionsFromDatabase(placeholders)
			fieldDefinitionsJSON, err := json.Marshal(fieldDefinitions)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal field definitions: %w", err)
			}
			template.FieldDefinitions = string(fieldDefinitionsJSON)
		}
	}

	// Handle HTML file replacement (manual override)
	if htmlFile != nil && htmlHeader != nil {
		// Validate file extension
		if ext := strings.ToLower(filepath.Ext(htmlHeader.Filename)); ext != ".html" && ext != ".htm" {
			return nil, fmt.Errorf("invalid file type: expected .html, got %s", ext)
		}

		// Upload new HTML file
		htmlObjectName := storage.GenerateObjectName(templateID, htmlHeader.Filename)
		_, err := s.storageClient.UploadFile(ctx, htmlFile, htmlObjectName, "text/html")
		if err != nil {
			return nil, fmt.Errorf("failed to upload HTML file: %w", err)
		}

		// Delete old HTML file
		if template.GCSPathHTML != "" {
			if err := s.storageClient.DeleteFile(ctx, template.GCSPathHTML); err != nil {
				fmt.Printf("Warning: failed to delete old HTML file %s: %v\n", template.GCSPathHTML, err)
			}
		}

		template.GCSPathHTML = htmlObjectName
	}

	// Handle thumbnail file replacement (manual override)
	if thumbnailFile != nil && thumbnailHeader != nil {
		// Validate file extension
		ext := strings.ToLower(filepath.Ext(thumbnailHeader.Filename))
		if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".webp" {
			return nil, fmt.Errorf("invalid file type: expected .png, .jpg, .jpeg, or .webp, got %s", ext)
		}

		// Determine content type
		contentType := "image/png"
		switch ext {
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		case ".webp":
			contentType = "image/webp"
		}

		// Upload new thumbnail file
		thumbnailObjectName := storage.GenerateObjectName(templateID, thumbnailHeader.Filename)
		_, err := s.storageClient.UploadFile(ctx, thumbnailFile, thumbnailObjectName, contentType)
		if err != nil {
			return nil, fmt.Errorf("failed to upload thumbnail file: %w", err)
		}

		// Delete old thumbnail file
		if template.GCSPathThumbnail != "" {
			if err := s.storageClient.DeleteFile(ctx, template.GCSPathThumbnail); err != nil {
				fmt.Printf("Warning: failed to delete old thumbnail file %s: %v\n", template.GCSPathThumbnail, err)
			}
		}

		template.GCSPathThumbnail = thumbnailObjectName
		fmt.Printf("[INFO] Custom thumbnail uploaded for template %s\n", templateID)
	}

	// Save to database
	if err := internal.DB.Save(template).Error; err != nil {
		return nil, fmt.Errorf("failed to update template: %w", err)
	}

	return template, nil
}

func (s *TemplateService) createTempFile(reader io.Reader) (string, error) {
	tempFile, err := os.CreateTemp("", "*.docx")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	_, err = io.Copy(tempFile, reader)
	if err != nil {
		os.Remove(tempFile.Name())
		return "", err
	}

	return tempFile.Name(), nil
}

func (s *TemplateService) cleanupTempFile(filePath string) {
	os.Remove(filePath)
}

func (s *TemplateService) GetHTMLPreview(ctx context.Context, gcsPath string) (string, error) {
	reader, err := s.storageClient.ReadFile(ctx, gcsPath)
	if err != nil {
		return "", fmt.Errorf("failed to read HTML preview from GCS: %w", err)
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read HTML preview content: %w", err)
	}

	return string(content), nil
}

// GetPDFPreview returns a reader for the PDF preview of a template
func (s *TemplateService) GetPDFPreview(ctx context.Context, gcsPath string) (io.ReadCloser, error) {
	reader, err := s.storageClient.ReadFile(ctx, gcsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF preview from storage: %w", err)
	}
	return reader, nil
}

// GetThumbnail returns a reader for the thumbnail image of a template
func (s *TemplateService) GetThumbnail(ctx context.Context, gcsPath string) (io.ReadCloser, error) {
	reader, err := s.storageClient.ReadFile(ctx, gcsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read thumbnail from storage: %w", err)
	}
	return reader, nil
}

// GenerateAndStorePDFPreview generates a PDF preview from the DOCX and stores it
// Returns the PDF content and the new GCS path
func (s *TemplateService) GenerateAndStorePDFPreview(ctx context.Context, template *models.Template) ([]byte, string, error) {
	if s.conversionService == nil || !s.conversionService.IsPDFConversionAvailable() {
		return nil, "", fmt.Errorf("PDF conversion is not available")
	}

	if template.GCSPath == "" {
		return nil, "", fmt.Errorf("template has no DOCX file")
	}

	// Download DOCX from storage
	reader, err := s.storageClient.ReadFile(ctx, template.GCSPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read DOCX from storage: %w", err)
	}
	defer reader.Close()

	// Create temp file for the DOCX
	tempFile, err := os.CreateTemp("", "*.docx")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	// Copy DOCX content to temp file
	if _, err := io.Copy(tempFile, reader); err != nil {
		tempFile.Close()
		return nil, "", fmt.Errorf("failed to write temp file: %w", err)
	}
	tempFile.Close()

	// Convert to PDF
	pdfContent, err := s.conversionService.ConvertDocxToPDF(ctx, tempPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to convert to PDF: %w", err)
	}

	// Upload PDF to storage
	pdfFileName := strings.TrimSuffix(template.Filename, filepath.Ext(template.Filename)) + ".pdf"
	pdfObjectName := storage.GenerateObjectName(template.ID, pdfFileName)
	pdfReader := bytes.NewReader(pdfContent)
	_, err = s.storageClient.UploadFile(ctx, io.NopCloser(pdfReader), pdfObjectName, "application/pdf")
	if err != nil {
		return nil, "", fmt.Errorf("failed to upload PDF: %w", err)
	}

	// Update template with new PDF path
	template.GCSPathPDF = pdfObjectName
	if err := internal.DB.Save(template).Error; err != nil {
		// Log warning but don't fail - PDF was already generated
		fmt.Printf("[WARNING] Failed to update template with PDF path: %v\n", err)
	}

	fmt.Printf("[INFO] Generated and stored PDF preview for template %s at %s\n", template.ID, pdfObjectName)
	return pdfContent, pdfObjectName, nil
}

// GenerateThumbnailForTemplate generates a thumbnail for a single template from its PDF
func (s *TemplateService) GenerateThumbnailForTemplate(ctx context.Context, template *models.Template) (string, error) {
	if s.conversionService == nil || !s.conversionService.IsThumbnailGenerationAvailable() {
		return "", fmt.Errorf("thumbnail generation is not available")
	}

	// If no PDF exists, generate it first
	var pdfContent []byte
	var err error

	if template.GCSPathPDF == "" {
		fmt.Printf("[INFO] Template %s has no PDF, generating PDF first...\n", template.ID)
		pdfContent, _, err = s.GenerateAndStorePDFPreview(ctx, template)
		if err != nil {
			return "", fmt.Errorf("failed to generate PDF: %w", err)
		}
	} else {
		// Read existing PDF
		reader, err := s.storageClient.ReadFile(ctx, template.GCSPathPDF)
		if err != nil {
			return "", fmt.Errorf("failed to read PDF from storage: %w", err)
		}
		defer reader.Close()
		pdfContent, err = io.ReadAll(reader)
		if err != nil {
			return "", fmt.Errorf("failed to read PDF content: %w", err)
		}
	}

	// Create temp file for PDF
	pdfTempFile, err := os.CreateTemp("", "*.pdf")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(pdfTempFile.Name())

	if _, err := pdfTempFile.Write(pdfContent); err != nil {
		pdfTempFile.Close()
		return "", fmt.Errorf("failed to write temp PDF: %w", err)
	}
	pdfTempFile.Close()

	// Generate HD thumbnail for better quality on detail pages
	thumbnailContent, err := s.conversionService.GenerateThumbnailFromPDFWithQuality(ctx, pdfTempFile.Name(), 600, ThumbnailQualityHD)
	if err != nil {
		return "", fmt.Errorf("failed to generate thumbnail: %w", err)
	}

	// Upload thumbnail
	thumbnailFileName := strings.TrimSuffix(template.Filename, filepath.Ext(template.Filename)) + "_thumb.png"
	thumbnailObjectName := storage.GenerateObjectName(template.ID, thumbnailFileName)
	thumbnailReader := bytes.NewReader(thumbnailContent)
	_, err = s.storageClient.UploadFile(ctx, io.NopCloser(thumbnailReader), thumbnailObjectName, "image/png")
	if err != nil {
		return "", fmt.Errorf("failed to upload thumbnail: %w", err)
	}

	// Update template with thumbnail path
	template.GCSPathThumbnail = thumbnailObjectName
	if err := internal.DB.Save(template).Error; err != nil {
		fmt.Printf("[WARNING] Failed to update template with thumbnail path: %v\n", err)
	}

	fmt.Printf("[INFO] Generated thumbnail for template %s at %s\n", template.ID, thumbnailObjectName)
	return thumbnailObjectName, nil
}

// GenerateHDThumbnail generates a high-definition thumbnail on-demand for a template
// This is used for the detail page where pixel-perfect rendering is needed
func (s *TemplateService) GenerateHDThumbnail(ctx context.Context, template *models.Template, width int) ([]byte, error) {
	if s.conversionService == nil || !s.conversionService.IsThumbnailGenerationAvailable() {
		return nil, fmt.Errorf("thumbnail generation is not available")
	}

	// Read PDF from storage
	if template.GCSPathPDF == "" {
		return nil, fmt.Errorf("PDF not found for template %s", template.ID)
	}

	reader, err := s.storageClient.ReadFile(ctx, template.GCSPathPDF)
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF from storage: %w", err)
	}
	defer reader.Close()

	pdfContent, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF content: %w", err)
	}

	// Create temp file for PDF
	pdfTempFile, err := os.CreateTemp("", "*.pdf")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(pdfTempFile.Name())

	if _, err := pdfTempFile.Write(pdfContent); err != nil {
		pdfTempFile.Close()
		return nil, fmt.Errorf("failed to write temp PDF: %w", err)
	}
	pdfTempFile.Close()

	// Generate HD thumbnail using the HD quality setting
	thumbnailContent, err := s.conversionService.GenerateThumbnailFromPDFWithQuality(ctx, pdfTempFile.Name(), width, ThumbnailQualityHD)
	if err != nil {
		return nil, fmt.Errorf("failed to generate HD thumbnail: %w", err)
	}

	return thumbnailContent, nil
}

// RegenerateThumbnailsForAllTemplates generates thumbnails for all templates missing them
// Returns the number of successfully generated thumbnails and any errors
func (s *TemplateService) RegenerateThumbnailsForAllTemplates(ctx context.Context, forceRegenerate bool) (int, int, []string) {
	var templates []models.Template
	query := internal.DB.Model(&models.Template{})

	if !forceRegenerate {
		// Only get templates without thumbnails
		query = query.Where("gcs_path_thumbnail IS NULL OR gcs_path_thumbnail = ''")
	}

	if err := query.Find(&templates).Error; err != nil {
		return 0, 0, []string{fmt.Sprintf("failed to get templates: %v", err)}
	}

	successCount := 0
	failCount := 0
	var errors []string

	for _, template := range templates {
		fmt.Printf("[INFO] Processing template %s (%s)...\n", template.ID, template.DisplayName)

		_, err := s.GenerateThumbnailForTemplate(ctx, &template)
		if err != nil {
			failCount++
			errMsg := fmt.Sprintf("Template %s (%s): %v", template.ID, template.DisplayName, err)
			errors = append(errors, errMsg)
			fmt.Printf("[ERROR] %s\n", errMsg)
		} else {
			successCount++
		}
	}

	return successCount, failCount, errors
}

// UpdateFieldDefinitions updates the field definitions for a template
func (s *TemplateService) UpdateFieldDefinitions(templateID string, fieldDefinitions map[string]utils.FieldDefinition) (*models.Template, error) {
	template, err := s.GetTemplate(templateID)
	if err != nil {
		return nil, err
	}

	// Convert field definitions to JSON
	fieldDefinitionsJSON, err := json.Marshal(fieldDefinitions)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal field definitions: %w", err)
	}

	template.FieldDefinitions = string(fieldDefinitionsJSON)

	if err := internal.DB.Save(template).Error; err != nil {
		return nil, fmt.Errorf("failed to update field definitions: %w", err)
	}

	return template, nil
}

// GetFieldDefinitions retrieves the field definitions for a template
func (s *TemplateService) GetFieldDefinitions(templateID string) (map[string]utils.FieldDefinition, error) {
	template, err := s.GetTemplate(templateID)
	if err != nil {
		return nil, err
	}

	if template.FieldDefinitions == "" {
		return make(map[string]utils.FieldDefinition), nil
	}

	var definitions map[string]utils.FieldDefinition
	if err := json.Unmarshal([]byte(template.FieldDefinitions), &definitions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal field definitions: %w", err)
	}

	return definitions, nil
}

// RegenerateFieldDefinitions regenerates field definitions from placeholders
func (s *TemplateService) RegenerateFieldDefinitions(templateID string) (*models.Template, error) {
	template, err := s.GetTemplate(templateID)
	if err != nil {
		return nil, err
	}

	// Parse placeholders
	var placeholders []string
	if err := json.Unmarshal([]byte(template.Placeholders), &placeholders); err != nil {
		return nil, fmt.Errorf("failed to unmarshal placeholders: %w", err)
	}

	// Generate field definitions using database rules
	fieldDefinitions := generateFieldDefinitionsFromDatabase(placeholders)
	fieldDefinitionsJSON, err := json.Marshal(fieldDefinitions)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal field definitions: %w", err)
	}

	template.FieldDefinitions = string(fieldDefinitionsJSON)

	if err := internal.DB.Save(template).Error; err != nil {
		return nil, fmt.Errorf("failed to save field definitions: %w", err)
	}

	return template, nil
}