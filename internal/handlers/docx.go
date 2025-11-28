package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"DF-PLCH/internal/models"
	"DF-PLCH/internal/services"

	"github.com/gin-gonic/gin"
)

type DocxHandler struct {
	templateService *services.TemplateService
	documentService *services.DocumentService
	gcsBucketName   string
}

func NewDocxHandler(templateService *services.TemplateService, documentService *services.DocumentService) *DocxHandler {
	return &DocxHandler{
		templateService: templateService,
		documentService: documentService,
	}
}

// SetGCSBucketName sets the GCS bucket name for document registration
func (h *DocxHandler) SetGCSBucketName(bucketName string) {
	h.gcsBucketName = bucketName
}

type PlaceholderResponse struct {
	Placeholders []string `json:"placeholders"`
}

type TemplatesResponse struct {
	Templates []models.Template `json:"templates"`
}

type ProcessRequest struct {
	Data           map[string]string `json:"data"`
	OrganizationID string            `json:"organization_id,omitempty"`
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

type ValidationError struct {
	Missing []string `json:"missing,omitempty"`
	Extra   []string `json:"extra,omitempty"`
}

func (e *ValidationError) HasErrors() bool {
	return len(e.Missing) > 0 || len(e.Extra) > 0
}

func (e *ValidationError) Error() string {
	var parts []string
	if len(e.Missing) > 0 {
		parts = append(parts, fmt.Sprintf("missing placeholders: %v", e.Missing))
	}
	if len(e.Extra) > 0 {
		parts = append(parts, fmt.Sprintf("unknown placeholders: %v", e.Extra))
	}
	return strings.Join(parts, "; ")
}

// validateRequestData checks if the request data matches the template placeholders
func (h *DocxHandler) validateRequestData(templatePlaceholders []string, requestData map[string]string) *ValidationError {
	validationErr := &ValidationError{}

	// Create a set of template placeholders for fast lookup
	placeholderSet := make(map[string]bool)
	for _, p := range templatePlaceholders {
		placeholderSet[p] = true
	}

	// Check for missing placeholders (in template but not in request)
	for _, placeholder := range templatePlaceholders {
		if _, exists := requestData[placeholder]; !exists {
			validationErr.Missing = append(validationErr.Missing, placeholder)
		}
	}

	// Check for extra keys (in request but not in template)
	for key := range requestData {
		if !placeholderSet[key] {
			validationErr.Extra = append(validationErr.Extra, key)
		}
	}

	return validationErr
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

	// Get template placeholders for validation
	placeholders, err := h.templateService.GetPlaceholders(templateID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}

	// Validate request data against template placeholders
	if validationErr := h.validateRequestData(placeholders, req.Data); validationErr.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Request data does not match template placeholders",
			"details": validationErr,
		})
		return
	}

	document, err := h.documentService.ProcessDocument(c.Request.Context(), templateID, req.Data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to process document: %v", err)})
		return
	}

	// If organization_id is provided, register the document with organization service
	if req.OrganizationID != "" {
		// Get user ID from X-User-ID header (set by API gateway)
		userID := c.GetHeader("X-User-ID")
		authToken := c.GetHeader("Authorization")
		fmt.Printf("[DEBUG] Registration check - OrgID: '%s', UserID: '%s', HasAuthToken: %v\n",
			req.OrganizationID, userID, authToken != "")
		if userID != "" && authToken != "" {
			// Use context.Background() for async call to avoid context cancellation
			go h.registerDocumentWithOrganization(context.Background(), req.OrganizationID, document, userID, authToken)
		} else {
			fmt.Printf("[DEBUG] Registration skipped - userID='%s', authToken!=\"\"=%v\n", userID, authToken != "")
		}
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

// registerDocumentWithOrganization calls the organization service to register the document
func (h *DocxHandler) registerDocumentWithOrganization(ctx context.Context, orgID string, document *models.Document, userID string, authToken string) {
	// This will be called asynchronously to not block the response

	// Prepare request body
	requestBody := map[string]interface{}{
		"document_id":  document.ID,
		"template_id":  document.TemplateID,
		"file_name":    document.Filename,
		"file_size":    document.FileSize,
		"mime_type":    document.MimeType,
		"gcs_bucket":   h.gcsBucketName,
		"gcs_path":     document.GCSPathDocx,
		"gcs_path_pdf": document.GCSPathPdf,
		"form_data":    map[string]interface{}{}, // Parse from document.Data if needed
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		fmt.Printf("[ERROR] Failed to marshal request body for document registration: %v\n", err)
		return
	}

	// Make HTTP request to organization service
	// Use localhost:8085 for direct service-to-service communication
	url := fmt.Sprintf("http://localhost:8085/api/v1/organizations/%s/documents/register", orgID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		fmt.Printf("[ERROR] Failed to create request for document registration: %v\n", err)
		return
	}

	// Set headers - forward the user's JWT token for authentication
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authToken)

	fmt.Printf("[INFO] Registering document %s with organization %s for user %v at %s\n",
		document.ID, orgID, userID, url)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("[ERROR] Failed to call organization service: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("[ERROR] Organization service returned status %d: %s\n", resp.StatusCode, string(body))
		return
	}

	fmt.Printf("[SUCCESS] Document %s registered with organization %s\n", document.ID, orgID)
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
