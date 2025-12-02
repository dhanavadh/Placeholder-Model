package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"DF-PLCH/internal"
	"DF-PLCH/internal/models"
	"DF-PLCH/internal/processor"
	"DF-PLCH/internal/storage"

	"github.com/google/uuid"
)

type DocumentService struct {
	storageClient   storage.StorageClient
	templateService *TemplateService
	pdfService      *PDFService
}

func NewDocumentService(storageClient storage.StorageClient, templateService *TemplateService, pdfService *PDFService) *DocumentService {
	return &DocumentService{
		storageClient:   storageClient,
		templateService: templateService,
		pdfService:      pdfService,
	}
}

func (s *DocumentService) ProcessDocument(ctx context.Context, templateID string, data map[string]string, userID string) (*models.Document, error) {
	fmt.Printf("[DEBUG] Starting ProcessDocument for template %s\n", templateID)

	// Get template
	fmt.Printf("[DEBUG] Fetching template metadata from database...\n")
	template, err := s.templateService.GetTemplate(templateID)
	if err != nil {
		return nil, fmt.Errorf("template not found: %w", err)
	}
	fmt.Printf("[DEBUG] Template found: %s, GCS path: %s\n", template.Filename, template.GCSPath)

	// Download template from GCS
	fmt.Printf("[DEBUG] Downloading template from GCS...\n")
	reader, err := s.storageClient.ReadFile(ctx, template.GCSPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template from GCS: %w", err)
	}
	defer reader.Close()
	fmt.Printf("[DEBUG] Template downloaded successfully from GCS\n")

	// Create temp input file
	fmt.Printf("[DEBUG] Creating temp input file...\n")
	tempInputFile, err := s.createTempFile(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp input file: %w", err)
	}
	defer s.cleanupTempFile(tempInputFile)
	fmt.Printf("[DEBUG] Temp input file created: %s\n", tempInputFile)

	// Create temp output file
	documentID := uuid.New().String()
	tempOutputFile := filepath.Join(os.TempDir(), documentID+".docx")
	fmt.Printf("[DEBUG] Created document ID: %s, temp output file: %s\n", documentID, tempOutputFile)

	// Process document
	fmt.Printf("[DEBUG] Creating DOCX processor with input: %s, output: %s\n", tempInputFile, tempOutputFile)
	proc := processor.NewDocxProcessor(tempInputFile, tempOutputFile)
	fmt.Printf("[DEBUG] DOCX processor created successfully, starting unzip...\n")
	if err := proc.UnzipDocx(); err != nil {
		return nil, fmt.Errorf("failed to unzip document: %w", err)
	}
	fmt.Printf("[DEBUG] DOCX unzip completed successfully\n")
	defer proc.Cleanup()

	// Get placeholders and prepare complete data
	fmt.Printf("[DEBUG] Starting placeholder extraction...\n")
	placeholders, err := proc.ExtractPlaceholders()
	if err != nil {
		return nil, fmt.Errorf("failed to extract placeholders: %w", err)
	}
	fmt.Printf("[DEBUG] Placeholder extraction completed, found %d placeholders\n", len(placeholders))

	fmt.Printf("[DEBUG] Preparing data for %d placeholders...\n", len(placeholders))
	completeData := make(map[string]string)
	for i, placeholder := range placeholders {
		if value, exists := data[placeholder]; exists {
			completeData[placeholder] = value
		} else {
			completeData[placeholder] = ""
		}
		fmt.Printf("[DEBUG] Placeholder %d/%d: %s -> '%s'\n", i+1, len(placeholders), placeholder, completeData[placeholder])
	}

	// Replace placeholders
	fmt.Printf("[DEBUG] Starting placeholder replacement for %d placeholders...\n", len(completeData))
	if err := proc.FindAndReplaceInDocument(completeData); err != nil {
		return nil, fmt.Errorf("failed to replace placeholders: %w", err)
	}
	fmt.Printf("[DEBUG] Placeholder replacement completed successfully\n")

	// Re-zip document
	if err := proc.ReZipDocx(); err != nil {
		return nil, fmt.Errorf("failed to create output document: %w", err)
	}

	// Upload processed DOCX document to GCS
	outputFile, err := os.Open(tempOutputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open output file: %w", err)
	}
	defer outputFile.Close()
	defer os.Remove(tempOutputFile)

	objectName := storage.GenerateDocumentObjectName(documentID, template.Filename)
	result, err := s.storageClient.UploadFile(ctx, outputFile, objectName, "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	if err != nil {
		return nil, fmt.Errorf("failed to upload processed document to GCS: %w", err)
	}

	// Generate PDF using optimized Gotenberg and upload to GCS quickly
	var pdfObjectName string
	var pdfGCSPath string
	fmt.Printf("[DEBUG] Starting optimized PDF generation for document %s\n", documentID)
	if s.pdfService != nil {
		fmt.Printf("[DEBUG] PDF service is available, proceeding with fast conversion\n")
		// Re-open the DOCX file for PDF conversion
		docxFile, err := os.Open(tempOutputFile)
		if err == nil {
			defer docxFile.Close()
			fmt.Printf("[DEBUG] Successfully opened DOCX file for PDF conversion: %s\n", tempOutputFile)

			// Detect orientation from the processed DOCX
			landscape := false
			if orientation, err := proc.DetectOrientation(); err == nil {
				landscape = orientation
				fmt.Printf("[DEBUG] Detected orientation: landscape=%v\n", landscape)
			} else {
				fmt.Printf("Warning: failed to detect orientation: %v\n", err)
			}

			// Convert DOCX to PDF with super-fast optimized Gotenberg (~200ms)
			fmt.Printf("[DEBUG] Starting lightning-fast PDF conversion and upload...\n")
			tempPDFPath := filepath.Join(os.TempDir(), documentID+"_output.pdf")

			// Use Gotenberg's Store method to save directly to temp file (ultra-fast)
			err = s.pdfService.ConvertDocxToPDFToFileWithOrientation(ctx, docxFile, template.Filename, tempPDFPath, landscape)
			if err != nil {
				fmt.Printf("[ERROR] Failed to convert DOCX to PDF: %v\n", err)
			} else {
				fmt.Printf("[DEBUG] PDF conversion successful in ~200ms, uploading from temp file...\n")
				defer os.Remove(tempPDFPath) // Clean up temp file

				// Open the temp PDF file and upload to GCS
				pdfFile, err := os.Open(tempPDFPath)
				if err != nil {
					fmt.Printf("[ERROR] Failed to open temp PDF file: %v\n", err)
				} else {
					defer pdfFile.Close()

					// Upload PDF to GCS from file - much more reliable than streaming
					pdfObjectName = storage.GenerateDocumentPDFObjectName(documentID, template.Filename)
					fmt.Printf("[DEBUG] Generated PDF object name: %s\n", pdfObjectName)

					_, err = s.storageClient.UploadFile(ctx, pdfFile, pdfObjectName, "application/pdf")
					if err != nil {
						fmt.Printf("[ERROR] Failed to upload PDF to GCS: %v\n", err)
						// Don't set pdfObjectName if upload failed
						pdfObjectName = ""
					} else {
						pdfGCSPath = pdfObjectName
						fmt.Printf("[DEBUG] PDF successfully uploaded to GCS: %s\n", pdfGCSPath)
					}
				}
			}
		} else {
			fmt.Printf("[ERROR] Failed to reopen DOCX file for PDF conversion: %v\n", err)
		}
	} else {
		fmt.Printf("[DEBUG] PDF service is nil, skipping PDF generation\n")
	}

	// Convert data to JSON
	dataJSON, err := json.Marshal(completeData)
	if err != nil {
		s.storageClient.DeleteFile(ctx, objectName)
		if pdfObjectName != "" {
			s.storageClient.DeleteFile(ctx, pdfObjectName)
		}
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	// Save document metadata
	document := &models.Document{
		ID:          documentID,
		TemplateID:  templateID,
		UserID:      userID,
		Filename:    template.Filename,
		GCSPathDocx: objectName,
		GCSPathPdf:  pdfGCSPath,
		FileSize:    result.Size,
		MimeType:    "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		Data:        string(dataJSON),
		Status:      "completed",
	}

	if err := internal.DB.Create(document).Error; err != nil {
		s.storageClient.DeleteFile(ctx, objectName)
		if pdfObjectName != "" {
			s.storageClient.DeleteFile(ctx, pdfObjectName)
		}
		return nil, fmt.Errorf("failed to save document metadata: %w", err)
	}

	return document, nil
}

func (s *DocumentService) GetDocument(documentID string) (*models.Document, error) {
	var document models.Document
	if err := internal.DB.First(&document, "id = ?", documentID).Error; err != nil {
		return nil, fmt.Errorf("document not found: %w", err)
	}
	return &document, nil
}

// GetUserDocuments retrieves all documents created by a user with pagination
func (s *DocumentService) GetUserDocuments(userID string, page, limit int) ([]models.Document, int64, error) {
	var documents []models.Document
	var total int64

	offset := (page - 1) * limit

	// Count total documents for user
	if err := internal.DB.Model(&models.Document{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count documents: %w", err)
	}

	// Get documents with template info, ordered by newest first
	if err := internal.DB.Preload("Template").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&documents).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get documents: %w", err)
	}

	return documents, total, nil
}

func (s *DocumentService) GetDocumentReader(ctx context.Context, documentID string, format string) (io.ReadCloser, string, string, error) {
	document, err := s.GetDocument(documentID)
	if err != nil {
		return nil, "", "", err
	}

	var gcsPath, filename, mimeType string

	switch format {
	case "pdf":
		if document.GCSPathPdf == "" {
			return nil, "", "", fmt.Errorf("PDF version not available")
		}
		gcsPath = document.GCSPathPdf
		filename = document.Filename[:len(document.Filename)-5] + ".pdf" // Remove .docx and add .pdf
		mimeType = "application/pdf"
	case "docx":
		fallthrough
	default:
		gcsPath = document.GCSPathDocx
		filename = document.Filename
		mimeType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	}

	reader, err := s.storageClient.ReadFile(ctx, gcsPath)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to read document from GCS: %w", err)
	}

	return reader, filename, mimeType, nil
}

func (s *DocumentService) DeleteDocument(ctx context.Context, documentID string) error {
	document, err := s.GetDocument(documentID)
	if err != nil {
		return err
	}

	// Delete from GCS
	if err := s.storageClient.DeleteFile(ctx, document.GCSPathDocx); err != nil {
		// Log error but continue with database deletion
		fmt.Printf("Warning: failed to delete GCS DOCX file %s: %v\n", document.GCSPathDocx, err)
	}
	if document.GCSPathPdf != "" {
		if err := s.storageClient.DeleteFile(ctx, document.GCSPathPdf); err != nil {
			// Log error but continue with database deletion
			fmt.Printf("Warning: failed to delete GCS PDF file %s: %v\n", document.GCSPathPdf, err)
		}
	}

	// Soft delete from database
	return internal.DB.Delete(document).Error
}

// DeleteProcessedFile deletes only the specified format file from GCS
// but keeps the document record with user data in the database
func (s *DocumentService) DeleteProcessedFile(ctx context.Context, documentID string, format string) error {
	document, err := s.GetDocument(documentID)
	if err != nil {
		return err
	}

	var gcsPath string
	switch format {
	case "pdf":
		gcsPath = document.GCSPathPdf
	default:
		gcsPath = document.GCSPathDocx
	}

	if gcsPath == "" {
		return fmt.Errorf("file path not found for format %s", format)
	}

	// Delete only the specified GCS file, keep database record
	if err := s.storageClient.DeleteFile(ctx, gcsPath); err != nil {
		return fmt.Errorf("failed to delete processed file from GCS: %w", err)
	}

	// Update document status to indicate file has been downloaded and deleted
	if err := internal.DB.Model(document).Update("status", "downloaded").Error; err != nil {
		fmt.Printf("Warning: failed to update document status: %v\n", err)
	}

	return nil
}

// DeleteProcessedFiles deletes both DOCX and PDF files from GCS
// but keeps the document record with user data in the database for regeneration
func (s *DocumentService) DeleteProcessedFiles(ctx context.Context, documentID string) error {
	document, err := s.GetDocument(documentID)
	if err != nil {
		return err
	}

	// Skip if already cleaned up
	if document.Status == "expired" {
		return nil
	}

	// Delete DOCX file from GCS
	if document.GCSPathDocx != "" {
		if err := s.storageClient.DeleteFile(ctx, document.GCSPathDocx); err != nil {
			fmt.Printf("Warning: failed to delete DOCX file %s: %v\n", document.GCSPathDocx, err)
		}
	}

	// Delete PDF file from GCS
	if document.GCSPathPdf != "" {
		if err := s.storageClient.DeleteFile(ctx, document.GCSPathPdf); err != nil {
			fmt.Printf("Warning: failed to delete PDF file %s: %v\n", document.GCSPathPdf, err)
		}
	}

	// Update document status and clear GCS paths
	updates := map[string]interface{}{
		"status":        "expired",
		"gcs_path_docx": "",
		"gcs_path_pdf":  "",
	}
	if err := internal.DB.Model(document).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update document status: %w", err)
	}

	return nil
}

// RegenerateDocument regenerates a document from stored data in the database
func (s *DocumentService) RegenerateDocument(ctx context.Context, documentID string, userID string) (*models.Document, error) {
	// Get the original document with stored data
	document, err := s.GetDocument(documentID)
	if err != nil {
		return nil, fmt.Errorf("document not found: %w", err)
	}

	// Verify user owns this document
	if document.UserID != userID {
		return nil, fmt.Errorf("unauthorized: you don't have access to this document")
	}

	// Parse the stored data
	var data map[string]string
	if err := json.Unmarshal([]byte(document.Data), &data); err != nil {
		return nil, fmt.Errorf("failed to parse stored data: %w", err)
	}

	// Get template
	template, err := s.templateService.GetTemplate(document.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("template not found: %w", err)
	}

	// Download template from GCS
	reader, err := s.storageClient.ReadFile(ctx, template.GCSPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template from GCS: %w", err)
	}
	defer reader.Close()

	// Create temp input file
	tempInputFile, err := s.createTempFile(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp input file: %w", err)
	}
	defer s.cleanupTempFile(tempInputFile)

	// Create temp output file (reuse same document ID)
	tempOutputFile := filepath.Join(os.TempDir(), documentID+"_regen.docx")

	// Process document
	proc := processor.NewDocxProcessor(tempInputFile, tempOutputFile)
	if err := proc.UnzipDocx(); err != nil {
		return nil, fmt.Errorf("failed to unzip document: %w", err)
	}
	defer proc.Cleanup()

	// Replace placeholders with stored data
	if err := proc.FindAndReplaceInDocument(data); err != nil {
		return nil, fmt.Errorf("failed to replace placeholders: %w", err)
	}

	// Re-zip document
	if err := proc.ReZipDocx(); err != nil {
		return nil, fmt.Errorf("failed to create output document: %w", err)
	}

	// Upload processed DOCX document to GCS
	outputFile, err := os.Open(tempOutputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open output file: %w", err)
	}
	defer outputFile.Close()
	defer os.Remove(tempOutputFile)

	objectName := storage.GenerateDocumentObjectName(documentID, template.Filename)
	result, err := s.storageClient.UploadFile(ctx, outputFile, objectName, "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	if err != nil {
		return nil, fmt.Errorf("failed to upload regenerated document to GCS: %w", err)
	}

	// Generate PDF
	var pdfObjectName string
	var pdfGCSPath string
	if s.pdfService != nil {
		docxFile, err := os.Open(tempOutputFile)
		if err == nil {
			defer docxFile.Close()

			landscape := false
			if orientation, err := proc.DetectOrientation(); err == nil {
				landscape = orientation
			}

			tempPDFPath := filepath.Join(os.TempDir(), documentID+"_regen.pdf")
			err = s.pdfService.ConvertDocxToPDFToFileWithOrientation(ctx, docxFile, template.Filename, tempPDFPath, landscape)
			if err == nil {
				defer os.Remove(tempPDFPath)

				pdfFile, err := os.Open(tempPDFPath)
				if err == nil {
					defer pdfFile.Close()

					pdfObjectName = storage.GenerateDocumentPDFObjectName(documentID, template.Filename)
					_, err = s.storageClient.UploadFile(ctx, pdfFile, pdfObjectName, "application/pdf")
					if err == nil {
						pdfGCSPath = pdfObjectName
					}
				}
			}
		}
	}

	// Update document record with new GCS paths and status
	updates := map[string]interface{}{
		"status":        "completed",
		"gcs_path_docx": objectName,
		"gcs_path_pdf":  pdfGCSPath,
		"file_size":     result.Size,
	}
	if err := internal.DB.Model(document).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update document: %w", err)
	}

	// Refresh document from DB
	document, _ = s.GetDocument(documentID)
	return document, nil
}

func (s *DocumentService) createTempFile(reader io.Reader) (string, error) {
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

func (s *DocumentService) cleanupTempFile(filePath string) {
	os.Remove(filePath)
}

