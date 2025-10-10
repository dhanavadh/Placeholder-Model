package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"os"

	"DF-PLCH/internal"
	"DF-PLCH/internal/models"
	"DF-PLCH/internal/processor"
	"DF-PLCH/internal/storage"

	"github.com/google/uuid"
)

type TemplateService struct {
	gcsClient *storage.GCSClient
}

func NewTemplateService(gcsClient *storage.GCSClient) *TemplateService {
	return &TemplateService{
		gcsClient: gcsClient,
	}
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
	result, err := s.gcsClient.UploadFile(ctx, file, objectName, header.Header.Get("Content-Type"))
	if err != nil {
		return nil, fmt.Errorf("failed to upload to GCS: %w", err)
	}

	// Upload HTML preview file if provided
	var htmlObjectName string
	if htmlFile != nil && htmlHeader != nil {
		htmlObjectName = storage.GenerateObjectName(templateID, htmlHeader.Filename)
		_, err := s.gcsClient.UploadFile(ctx, htmlFile, htmlObjectName, "text/html")
		if err != nil {
			// Clean up DOCX file on error
			s.gcsClient.DeleteFile(ctx, objectName)
			return nil, fmt.Errorf("failed to upload HTML preview to GCS: %w", err)
		}
	}

	// Create temp file for processing
	file.Seek(0, 0) // Reset file pointer
	tempFile, err := s.createTempFile(file)
	if err != nil {
		// Cleanup GCS file on failure
		s.gcsClient.DeleteFile(ctx, objectName)
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer s.cleanupTempFile(tempFile)

	// Process DOCX to extract placeholders
	proc := processor.NewDocxProcessor(tempFile, "")
	if err := proc.UnzipDocx(); err != nil {
		s.gcsClient.DeleteFile(ctx, objectName)
		return nil, fmt.Errorf("failed to process document: %w", err)
	}
	defer proc.Cleanup()

	placeholders, err := proc.ExtractPlaceholders()
	if err != nil {
		s.gcsClient.DeleteFile(ctx, objectName)
		return nil, fmt.Errorf("failed to extract placeholders: %w", err)
	}

	// Extract positions
	positions, err := proc.ExtractPlaceholdersWithPositions()
	if err != nil {
		s.gcsClient.DeleteFile(ctx, objectName)
		return nil, fmt.Errorf("failed to extract placeholder positions: %w", err)
	}

	// Convert placeholders to JSON
	placeholdersJSON, err := json.Marshal(placeholders)
	if err != nil {
		s.gcsClient.DeleteFile(ctx, objectName)
		return nil, fmt.Errorf("failed to marshal placeholders: %w", err)
	}

	// Convert positions to JSON
	positionsJSON, err := json.Marshal(positions)
	if err != nil {
		s.gcsClient.DeleteFile(ctx, objectName)
		return nil, fmt.Errorf("failed to marshal positions: %w", err)
	}

	// Save to database
	template := &models.Template{
		ID:           templateID,
		Filename:     header.Filename,
		OriginalName: header.Filename,
		DisplayName:  fileName,
		Description:  description,
		Author:       author,
		GCSPath:      objectName,
		GCSPathHTML:  htmlObjectName,
		FileSize:     result.Size,
		MimeType:     header.Header.Get("Content-Type"),
		Placeholders: string(placeholdersJSON),
		Positions:    string(positionsJSON),
	}

	// Convert aliases to JSON (if provided)
	if aliases != nil && len(aliases) > 0 {
		aliasesBytes, err := json.Marshal(aliases)
		if err != nil {
			s.gcsClient.DeleteFile(ctx, objectName)
			return nil, fmt.Errorf("failed to marshal aliases: %w", err)
		}
		template.Aliases = string(aliasesBytes)
	} else {
		// Store empty JSON object for no aliases (valid JSON)
		template.Aliases = "{}"
	}

	if err := internal.DB.Create(template).Error; err != nil {
		s.gcsClient.DeleteFile(ctx, objectName)
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

func (s *TemplateService) GetPlaceholderPositions(templateID string) ([]processor.PlaceholderPosition, error) {
	template, err := s.GetTemplate(templateID)
	if err != nil {
		return nil, err
	}

	var positions []processor.PlaceholderPosition
	if template.Positions != "" {
		if err := json.Unmarshal([]byte(template.Positions), &positions); err != nil {
			return nil, fmt.Errorf("failed to unmarshal positions: %w", err)
		}
	}

	return positions, nil
}

func (s *TemplateService) DeleteTemplate(ctx context.Context, templateID string) error {
	template, err := s.GetTemplate(templateID)
	if err != nil {
		return err
	}

	// Delete from GCS
	if err := s.gcsClient.DeleteFile(ctx, template.GCSPath); err != nil {
		// Log error but continue with database deletion
		fmt.Printf("Warning: failed to delete GCS file %s: %v\n", template.GCSPath, err)
	}

	// Soft delete from database
	return internal.DB.Delete(template).Error
}

func (s *TemplateService) UpdateTemplate(ctx context.Context, templateID, displayName, description, author string, aliases map[string]string, htmlFile multipart.File, htmlHeader *multipart.FileHeader) (*models.Template, error) {
	template, err := s.GetTemplate(templateID)
	if err != nil {
		return nil, err
	}

	// Update fields
	template.DisplayName = displayName
	template.Description = description
	template.Author = author

	// Convert aliases to JSON (if provided)
	if aliases != nil {
		aliasesJSON, err := json.Marshal(aliases)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal aliases: %w", err)
		}
		template.Aliases = string(aliasesJSON)
	}

	// Upload HTML preview file if provided
	if htmlFile != nil && htmlHeader != nil {
		htmlObjectName := storage.GenerateObjectName(templateID, htmlHeader.Filename)
		_, err := s.gcsClient.UploadFile(ctx, htmlFile, htmlObjectName, "text/html")
		if err != nil {
			return nil, fmt.Errorf("failed to upload HTML preview to GCS: %w", err)
		}

		// Delete old HTML file if exists
		if template.GCSPathHTML != "" {
			if err := s.gcsClient.DeleteFile(ctx, template.GCSPathHTML); err != nil {
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
	reader, err := s.gcsClient.ReadFile(ctx, gcsPath)
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