package services

import (
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
)

type TemplateService struct {
	storageClient storage.StorageClient
}

func NewTemplateService(storageClient storage.StorageClient) *TemplateService {
	return &TemplateService{
		storageClient: storageClient,
	}
}

// generateFieldDefinitionsFromDatabase generates field definitions using Data Types from database
// Each DataType has a pattern field for matching placeholder names
func generateFieldDefinitionsFromDatabase(placeholders []string) map[string]utils.FieldDefinition {
	definitions := make(map[string]utils.FieldDefinition)

	// Load data types from database (sorted by priority DESC - higher priority first)
	var dataTypes []models.DataType
	internal.DB.Where("is_active = ?", true).Order("priority DESC").Find(&dataTypes)

	// Load entity rules from database
	var entityRules []models.EntityRule
	internal.DB.Where("is_active = ?", true).Order("priority DESC").Find(&entityRules)

	for _, placeholder := range placeholders {
		key := strings.ReplaceAll(placeholder, "{{", "")
		key = strings.ReplaceAll(key, "}}", "")

		definition := applyDataTypeRules(key, placeholder, dataTypes, entityRules)
		definitions[key] = definition
	}

	return definitions
}

// applyDataTypeRules applies data type patterns and entity rules to a placeholder
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

	// Upload HTML preview file if provided
	var htmlObjectName string
	if htmlFile != nil && htmlHeader != nil {
		htmlObjectName = storage.GenerateObjectName(templateID, htmlHeader.Filename)
		_, err := s.storageClient.UploadFile(ctx, htmlFile, htmlObjectName, "text/html")
		if err != nil {
			// Clean up DOCX file on error
			s.storageClient.DeleteFile(ctx, objectName)
			return nil, fmt.Errorf("failed to upload HTML preview to GCS: %w", err)
		}
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
		FileSize:         result.Size,
		MimeType:         header.Header.Get("Content-Type"),
		Placeholders:     string(placeholdersJSON),
		FieldDefinitions: string(fieldDefinitionsJSON),
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
func (s *TemplateService) ReplaceTemplateFiles(ctx context.Context, templateID string, docxFile multipart.File, docxHeader *multipart.FileHeader, htmlFile multipart.File, htmlHeader *multipart.FileHeader, regenerateFields bool) (*models.Template, error) {
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

	// Handle HTML file replacement
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