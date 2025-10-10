package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"DF-PLCH/internal/models"
	"DF-PLCH/internal/processor"
	"DF-PLCH/internal/services"

	"github.com/gin-gonic/gin"
)

type DocxHandler struct {
	templateService *services.TemplateService
	documentService *services.DocumentService
}

func NewDocxHandler(templateService *services.TemplateService, documentService *services.DocumentService) *DocxHandler {
	return &DocxHandler{
		templateService: templateService,
		documentService: documentService,
	}
}

type PlaceholderResponse struct {
	Placeholders []string `json:"placeholders"`
}

type PlaceholderPositionResponse struct {
	Placeholders []processor.PlaceholderPosition `json:"placeholders"`
}

type TemplatesResponse struct {
	Templates []models.Template `json:"templates"`
}

type ProcessRequest struct {
	Data map[string]string `json:"data"`
}

type UploadResponse struct {
	TemplateID   string            `json:"template_id"`
	FileName     string            `json:"file_name"`
	Description  string            `json:"description"`
	Author       string            `json:"author"`
	Placeholders []string          `json:"placeholders"`
	Aliases      map[string]string `json:"aliases,omitempty"`
	Message      string            `json:"message"`
}

type ProcessResponse struct {
	DocumentID     string `json:"document_id"`
	DownloadURL    string `json:"download_url"`
	DownloadPDFURL string `json:"download_pdf_url,omitempty"`
	ExpiresAt      string `json:"expires_at"`
	Message        string `json:"message"`
}

func (h *DocxHandler) UploadTemplate(c *gin.Context) {
	file, header, err := c.Request.FormFile("template")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}
	defer file.Close()

	if filepath.Ext(header.Filename) != ".docx" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only .docx files are supported"})
		return
	}

	// Get optional HTML file for preview
	var htmlFile multipart.File
	var htmlHeader *multipart.FileHeader
	htmlFile, htmlHeader, err = c.Request.FormFile("htmlPreview")
	if err == nil {
		// HTML file was provided
		defer htmlFile.Close()
		if filepath.Ext(htmlHeader.Filename) != ".html" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Preview file must be .html"})
			return
		}
	} else {
		// HTML file is optional, so err == nil means it wasn't provided
		htmlFile = nil
		htmlHeader = nil
	}

	// Get required fields from form
	fileName := c.PostForm("fileName")
	description := c.PostForm("description")
	author := c.PostForm("author")
	aliasesJSON := c.PostForm("aliases") // Optional: JSON object mapping placeholders to aliases

	if fileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "fileName is required"})
		return
	}
	if description == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "description is required"})
		return
	}
	if author == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "author is required"})
		return
	}

	// Parse aliases if provided
	var aliases map[string]string
	if aliasesJSON != "" {
		if err := json.Unmarshal([]byte(aliasesJSON), &aliases); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid aliases JSON format"})
			return
		}
	}

	template, err := h.templateService.UploadTemplateWithHTMLPreview(c.Request.Context(), file, header, htmlFile, htmlHeader, fileName, description, author, aliases)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to upload template: %v", err)})
		return
	}

	// Parse placeholders from JSON
	var placeholders []string
	if err := json.Unmarshal([]byte(template.Placeholders), &placeholders); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse placeholders"})
		return
	}

	// Parse aliases from JSON if available
	var responseAliases map[string]string
	if template.Aliases != "" {
		if err := json.Unmarshal([]byte(template.Aliases), &responseAliases); err != nil {
			// If parsing fails, just don't include aliases in response
			responseAliases = nil
		}
	}

	response := UploadResponse{
		TemplateID:   template.ID,
		FileName:     template.DisplayName,
		Description:  template.Description,
		Author:       template.Author,
		Placeholders: placeholders,
		Aliases:      responseAliases,
		Message:      "Template uploaded successfully",
	}

	c.JSON(http.StatusOK, response)
}

func (h *DocxHandler) GetAllTemplates(c *gin.Context) {
	templates, err := h.templateService.GetAllTemplates()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get templates: %v", err)})
		return
	}

	response := TemplatesResponse{
		Templates: templates,
	}

	c.JSON(http.StatusOK, response)
}

func (h *DocxHandler) GetPlaceholders(c *gin.Context) {
	templateID := c.Param("templateId")
	if templateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Template ID is required"})
		return
	}

	placeholders, err := h.templateService.GetPlaceholders(templateID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}

	response := PlaceholderResponse{
		Placeholders: placeholders,
	}

	c.JSON(http.StatusOK, response)
}

func (h *DocxHandler) GetPlaceholderPositions(c *gin.Context) {
	templateID := c.Param("templateId")
	if templateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Template ID is required"})
		return
	}

	positions, err := h.templateService.GetPlaceholderPositions(templateID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}

	response := PlaceholderPositionResponse{
		Placeholders: positions,
	}

	c.JSON(http.StatusOK, response)
}

func (h *DocxHandler) GetHTMLPreview(c *gin.Context) {
	templateID := c.Param("templateId")
	if templateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Template ID is required"})
		return
	}

	template, err := h.templateService.GetTemplate(templateID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}

	if template.GCSPathHTML == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "No HTML preview available for this template"})
		return
	}

	// Get HTML content from GCS
	htmlContent, err := h.templateService.GetHTMLPreview(c.Request.Context(), template.GCSPathHTML)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to retrieve HTML preview: %v", err)})
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(htmlContent))
}

func (h *DocxHandler) ProcessDocument(c *gin.Context) {
	templateID := c.Param("templateId")
	if templateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Template ID is required"})
		return
	}

	var req ProcessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	document, err := h.documentService.ProcessDocument(c.Request.Context(), templateID, req.Data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to process document: %v", err)})
		return
	}

	// Create temporary download link that expires in 24 hours
	expiresAt := time.Now().Add(24 * time.Hour)
	response := ProcessResponse{
		DocumentID:  document.ID,
		DownloadURL: fmt.Sprintf("/api/v1/documents/%s/download", document.ID),
		ExpiresAt:   expiresAt.Format(time.RFC3339),
		Message:     "Document processed successfully",
	}

	// Add PDF download URL if PDF was generated
	if document.GCSPathPdf != "" {
		response.DownloadPDFURL = fmt.Sprintf("/api/v1/documents/%s/download?format=pdf", document.ID)
	}

	c.JSON(http.StatusOK, response)
}

func (h *DocxHandler) DownloadDocument(c *gin.Context) {
	documentID := c.Param("documentId")
	if documentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Document ID is required"})
		return
	}

	format := c.DefaultQuery("format", "docx")

	reader, filename, mimeType, err := h.documentService.GetDocumentReader(c.Request.Context(), documentID, format)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Document not found"})
		return
	}
	defer reader.Close()

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", mimeType)

	// Stream the file to the client
	_, err = io.Copy(c.Writer, reader)
	if err != nil {
		// If streaming fails, don't delete the file
		fmt.Printf("Error streaming file: %v\n", err)
		return
	}

	// After successful download, delete the processed DOCX file from GCS
	// but keep the document record in database with user data
	go func() {
		if err := h.documentService.DeleteProcessedFile(c.Request.Context(), documentID, format); err != nil {
			fmt.Printf("Warning: failed to delete processed file for document %s: %v\n", documentID, err)
		}
	}()
}

func (h *DocxHandler) DeleteTemplate(c *gin.Context) {
	templateID := c.Param("templateId")
	if templateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Template ID is required"})
		return
	}

	err := h.templateService.DeleteTemplate(c.Request.Context(), templateID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to delete template: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Template deleted successfully"})
}

type UpdateTemplateRequest struct {
	DisplayName string            `json:"display_name"`
	Description string            `json:"description"`
	Author      string            `json:"author"`
	Aliases     map[string]string `json:"aliases,omitempty"`
}

func (h *DocxHandler) UpdateTemplate(c *gin.Context) {
	templateID := c.Param("templateId")
	if templateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Template ID is required"})
		return
	}

	// Check if this is a multipart form (for HTML file upload) or JSON
	contentType := c.GetHeader("Content-Type")

	if contentType == "application/json" || !strings.Contains(contentType, "multipart/form-data") {
		// JSON update (no HTML file)
		var req UpdateTemplateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
			return
		}

		if req.DisplayName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "display_name is required"})
			return
		}
		if req.Description == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "description is required"})
			return
		}
		if req.Author == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "author is required"})
			return
		}

		template, err := h.templateService.UpdateTemplate(c.Request.Context(), templateID, req.DisplayName, req.Description, req.Author, req.Aliases, nil, nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to update template: %v", err)})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":  "Template updated successfully",
			"template": template,
		})
	} else {
		// Multipart form update (with optional HTML file)
		displayName := c.PostForm("displayName")
		description := c.PostForm("description")
		author := c.PostForm("author")
		aliasesJSON := c.PostForm("aliases")

		if displayName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "displayName is required"})
			return
		}
		if description == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "description is required"})
			return
		}
		if author == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "author is required"})
			return
		}

		// Parse aliases if provided
		var aliases map[string]string
		if aliasesJSON != "" {
			if err := json.Unmarshal([]byte(aliasesJSON), &aliases); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid aliases JSON format"})
				return
			}
		}

		// Get optional HTML file
		var htmlFile multipart.File
		var htmlHeader *multipart.FileHeader
		htmlFile, htmlHeader, err := c.Request.FormFile("htmlPreview")
		if err == nil {
			defer htmlFile.Close()
			if filepath.Ext(htmlHeader.Filename) != ".html" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Preview file must be .html"})
				return
			}
		} else {
			htmlFile = nil
			htmlHeader = nil
		}

		template, err := h.templateService.UpdateTemplate(c.Request.Context(), templateID, displayName, description, author, aliases, htmlFile, htmlHeader)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to update template: %v", err)})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":  "Template updated successfully",
			"template": template,
		})
	}
}

// Legacy functions for backward compatibility - these will be removed
func UploadTemplate(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": "This endpoint is deprecated. Use dependency injection instead."})
}

func GetPlaceholders(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": "This endpoint is deprecated. Use dependency injection instead."})
}

func ProcessDocument(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": "This endpoint is deprecated. Use dependency injection instead."})
}

func DownloadDocument(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": "This endpoint is deprecated. Use dependency injection instead."})
}
