package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"DF-PLCH/internal/models"
	"DF-PLCH/internal/services"
	"DF-PLCH/internal/utils"

	"github.com/gin-gonic/gin"
)

type DocxHandler struct {
	templateService   *services.TemplateService
	documentService   *services.DocumentService
	statisticsService *services.StatisticsService
	storageInfo       string // Storage identifier (bucket name for GCS, path for local)
	baseURL           string // Base URL for generating public URLs
}

func NewDocxHandler(templateService *services.TemplateService, documentService *services.DocumentService, statisticsService *services.StatisticsService) *DocxHandler {
	return &DocxHandler{
		templateService:   templateService,
		documentService:   documentService,
		statisticsService: statisticsService,
	}
}

// SetStorageInfo sets the storage identifier for logging/metadata purposes
func (h *DocxHandler) SetStorageInfo(info string) {
	h.storageInfo = info
}

// SetBaseURL sets the base URL for generating public API URLs
func (h *DocxHandler) SetBaseURL(url string) {
	h.baseURL = url
}

type PlaceholderResponse struct {
	Placeholders []string `json:"placeholders"`
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
	// Check if filtering is requested
	documentTypeID := c.Query("document_type_id")
	templateType := c.Query("type")
	tier := c.Query("tier")
	category := c.Query("category")
	search := c.Query("search")
	isVerifiedStr := c.Query("is_verified")
	includeDocumentType := c.Query("include_document_type") == "true"
	grouped := c.Query("grouped") == "true"
	sort := c.Query("sort")       // "popular", "recent", "name"
	limitStr := c.Query("limit")  // Limit number of results

	// If grouped view is requested, return templates grouped by document type
	if grouped {
		documentTypes, orphanTemplates, err := h.templateService.GetTemplatesGroupedByDocumentType()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get templates: %v", err)})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"document_types":    documentTypes,
			"orphan_templates": orphanTemplates,
		})
		return
	}

	// Parse limit
	var limit int
	if limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
	}

	// If any filter/sort is specified, use filtered query
	if documentTypeID != "" || templateType != "" || tier != "" || category != "" || search != "" || isVerifiedStr != "" || includeDocumentType || sort != "" || limit > 0 {
		filter := &services.TemplateFilter{
			DocumentTypeID:      documentTypeID,
			Type:                templateType,
			Tier:                tier,
			Category:            category,
			Search:              search,
			IncludeDocumentType: includeDocumentType,
			Sort:                sort,
			Limit:               limit,
		}

		// Parse is_verified boolean
		if isVerifiedStr != "" {
			isVerified := isVerifiedStr == "true"
			filter.IsVerified = &isVerified
		}

		templates, err := h.templateService.GetTemplatesWithFilter(filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get templates: %v", err)})
			return
		}

		// Convert to clean DTO format for public API
		templateItems := models.ToListItems(templates, h.baseURL)
		c.JSON(http.StatusOK, models.TemplateListResponse{
			Templates: templateItems,
			Total:     len(templateItems),
		})
		return
	}

	// Default: return all templates
	templates, err := h.templateService.GetAllTemplates()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get templates: %v", err)})
		return
	}

	// Convert to clean DTO format for public API
	templateItems := models.ToListItems(templates, h.baseURL)
	c.JSON(http.StatusOK, models.TemplateListResponse{
		Templates: templateItems,
		Total:     len(templateItems),
	})
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

// GetPDFPreview returns the PDF preview of a template
// If PDF doesn't exist, it generates one on-the-fly from the DOCX
func (h *DocxHandler) GetPDFPreview(c *gin.Context) {
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

	// If PDF doesn't exist, try to generate it on-the-fly
	if template.GCSPathPDF == "" {
		fmt.Printf("[INFO] PDF preview not found for template %s, generating on-the-fly...\n", templateID)

		// Generate and store PDF preview
		pdfContent, newPdfPath, err := h.templateService.GenerateAndStorePDFPreview(c.Request.Context(), template)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to generate PDF preview: %v", err)})
			return
		}

		// Return the generated PDF directly
		filename := strings.TrimSuffix(template.Filename, filepath.Ext(template.Filename)) + ".pdf"
		c.Header("Content-Type", "application/pdf")
		c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%s", filename))
		c.Data(http.StatusOK, "application/pdf", pdfContent)

		fmt.Printf("[INFO] Generated PDF preview on-the-fly for template %s, stored at %s\n", templateID, newPdfPath)
		return
	}

	// Get PDF content from storage
	reader, err := h.templateService.GetPDFPreview(c.Request.Context(), template.GCSPathPDF)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to retrieve PDF preview: %v", err)})
		return
	}
	defer reader.Close()

	// Set headers for PDF
	filename := strings.TrimSuffix(template.Filename, filepath.Ext(template.Filename)) + ".pdf"
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%s", filename))

	// Stream the PDF to the client
	io.Copy(c.Writer, reader)
}

// GetThumbnail returns the thumbnail image for a template
func (h *DocxHandler) GetThumbnail(c *gin.Context) {
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

	// Check if thumbnail exists
	if template.GCSPathThumbnail == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Thumbnail not found for this template"})
		return
	}

	// Get thumbnail content from storage
	reader, err := h.templateService.GetThumbnail(c.Request.Context(), template.GCSPathThumbnail)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to retrieve thumbnail: %v", err)})
		return
	}
	defer reader.Close()

	// Set headers for PNG image
	filename := strings.TrimSuffix(template.Filename, filepath.Ext(template.Filename)) + "_thumb.png"
	c.Header("Content-Type", "image/png")
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%s", filename))
	c.Header("Cache-Control", "public, max-age=86400") // Cache for 24 hours

	// Stream the thumbnail to the client
	io.Copy(c.Writer, reader)
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

	// Get user ID from X-User-ID header (set by API gateway)
	userID := c.GetHeader("X-User-ID")

	document, err := h.documentService.ProcessDocument(c.Request.Context(), templateID, req.Data, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to process document: %v", err)})
		return
	}

	// Record form submission statistics
	if h.statisticsService != nil {
		go func() {
			if err := h.statisticsService.RecordFormSubmit(templateID); err != nil {
				fmt.Printf("Warning: failed to record form submit statistics: %v\n", err)
			}
		}()
	}

	// Create temporary download link that expires in 10 minutes
	expiresAt := time.Now().Add(10 * time.Minute)
	response := ProcessResponse{
		DocumentID:  document.ID,
		DownloadURL: fmt.Sprintf("/api/v1/documents/%s/download", document.ID),
		ExpiresAt:   expiresAt.Format(time.RFC3339),
		Message:     "Document processed successfully. File will be deleted after 10 minutes. You can regenerate from history anytime.",
	}

	// Schedule file deletion after 10 minutes
	go func() {
		time.Sleep(10 * time.Minute)
		ctx := context.Background()
		if err := h.documentService.DeleteProcessedFiles(ctx, document.ID); err != nil {
			fmt.Printf("Warning: failed to auto-delete processed files for document %s: %v\n", document.ID, err)
		} else {
			fmt.Printf("Auto-deleted processed files for document %s after 10 minutes\n", document.ID)
		}
	}()

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
		"storage":      h.storageInfo,
		"storage_path": document.GCSPathDocx,
		"pdf_path":     document.GCSPathPdf,
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

	// Security: Get user ID from header (set by API gateway after JWT validation)
	userID := c.GetHeader("X-User-ID")

	format := c.DefaultQuery("format", "docx")

	var reader io.ReadCloser
	var filename, mimeType string
	var err error
	var templateID string

	// Security: Use authorized reader if user is authenticated
	if userID != "" {
		reader, filename, mimeType, err = h.documentService.GetDocumentReaderWithAuth(c.Request.Context(), documentID, userID, format)
		if err != nil {
			// Check if it's an authorization error
			if strings.Contains(err.Error(), "unauthorized") {
				c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
				return
			}
			c.JSON(http.StatusNotFound, gin.H{"error": "Document not found"})
			return
		}
		// Get template ID for statistics
		if doc, docErr := h.documentService.GetDocumentWithAuth(documentID, userID); docErr == nil {
			templateID = doc.TemplateID
		}
	} else {
		// For backwards compatibility, allow unauthenticated access if no user header
		// This should be restricted at the API gateway level
		reader, filename, mimeType, err = h.documentService.GetDocumentReader(c.Request.Context(), documentID, format)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Document not found"})
			return
		}
		// Get template ID for statistics
		if doc, docErr := h.documentService.GetDocument(documentID); docErr == nil {
			templateID = doc.TemplateID
		}
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

	// Record download/export statistics
	if h.statisticsService != nil {
		go func() {
			// Record as export (document download is effectively an export)
			if err := h.statisticsService.RecordExport(templateID); err != nil {
				fmt.Printf("Warning: failed to record export statistics: %v\n", err)
			}
		}()
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
	DisplayName    string            `json:"display_name"`
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	Author         string            `json:"author"`
	Category       string            `json:"category"`
	OriginalSource string            `json:"original_source"`
	Remarks        string            `json:"remarks"`
	IsVerified     *bool             `json:"is_verified"`
	IsAIAvailable  *bool             `json:"is_ai_available"`
	Type           string            `json:"type"`
	Tier           string            `json:"tier"`
	Group          string            `json:"group"`
	Aliases        map[string]string `json:"aliases,omitempty"`
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

		updateReq := &services.TemplateUpdateRequest{
			DisplayName:    req.DisplayName,
			Name:           req.Name,
			Description:    req.Description,
			Author:         req.Author,
			Category:       req.Category,
			OriginalSource: req.OriginalSource,
			Remarks:        req.Remarks,
			IsVerified:     req.IsVerified,
			IsAIAvailable:  req.IsAIAvailable,
			Type:           req.Type,
			Tier:           req.Tier,
			Group:          req.Group,
			Aliases:        req.Aliases,
		}

		template, err := h.templateService.UpdateTemplate(c.Request.Context(), templateID, updateReq, nil, nil)
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
		name := c.PostForm("name")
		description := c.PostForm("description")
		author := c.PostForm("author")
		category := c.PostForm("category")
		originalSource := c.PostForm("original_source")
		remarks := c.PostForm("remarks")
		isVerifiedStr := c.PostForm("is_verified")
		isAIAvailableStr := c.PostForm("is_ai_available")
		templateType := c.PostForm("type")
		tier := c.PostForm("tier")
		group := c.PostForm("group")
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

		// Parse boolean fields
		var isVerified, isAIAvailable *bool
		if isVerifiedStr != "" {
			v := isVerifiedStr == "true"
			isVerified = &v
		}
		if isAIAvailableStr != "" {
			v := isAIAvailableStr == "true"
			isAIAvailable = &v
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

		updateReq := &services.TemplateUpdateRequest{
			DisplayName:    displayName,
			Name:           name,
			Description:    description,
			Author:         author,
			Category:       category,
			OriginalSource: originalSource,
			Remarks:        remarks,
			IsVerified:     isVerified,
			IsAIAvailable:  isAIAvailable,
			Type:           templateType,
			Tier:           tier,
			Group:          group,
			Aliases:        aliases,
		}

		template, err := h.templateService.UpdateTemplate(c.Request.Context(), templateID, updateReq, htmlFile, htmlHeader)
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

// ReplaceTemplateFiles handles replacing DOCX and/or HTML files for an existing template
func (h *DocxHandler) ReplaceTemplateFiles(c *gin.Context) {
	templateID := c.Param("templateId")
	if templateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Template ID is required"})
		return
	}

	// Parse multipart form
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil { // 32MB max
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse multipart form"})
		return
	}

	// Get optional DOCX file
	var docxFile multipart.File
	var docxHeader *multipart.FileHeader
	docxFile, docxHeader, err := c.Request.FormFile("docx")
	if err == nil {
		defer docxFile.Close()
		if filepath.Ext(docxHeader.Filename) != ".docx" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Document file must be .docx"})
			return
		}
	} else {
		docxFile = nil
		docxHeader = nil
	}

	// Get optional HTML file
	var htmlFile multipart.File
	var htmlHeader *multipart.FileHeader
	htmlFile, htmlHeader, err = c.Request.FormFile("html")
	if err == nil {
		defer htmlFile.Close()
		ext := strings.ToLower(filepath.Ext(htmlHeader.Filename))
		if ext != ".html" && ext != ".htm" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Preview file must be .html or .htm"})
			return
		}
	} else {
		htmlFile = nil
		htmlHeader = nil
	}

	// Get optional thumbnail file
	var thumbnailFile multipart.File
	var thumbnailHeader *multipart.FileHeader
	thumbnailFile, thumbnailHeader, err = c.Request.FormFile("thumbnail")
	if err == nil {
		defer thumbnailFile.Close()
		ext := strings.ToLower(filepath.Ext(thumbnailHeader.Filename))
		if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".webp" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Thumbnail must be .png, .jpg, .jpeg, or .webp"})
			return
		}
	} else {
		thumbnailFile = nil
		thumbnailHeader = nil
	}

	// At least one file must be provided
	if docxFile == nil && htmlFile == nil && thumbnailFile == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least one file (docx, html, or thumbnail) must be provided"})
		return
	}

	// Check if field definitions should be regenerated
	regenerateFields := c.PostForm("regenerate_fields") == "true"

	// Replace files
	template, err := h.templateService.ReplaceTemplateFiles(c.Request.Context(), templateID, docxFile, docxHeader, htmlFile, htmlHeader, thumbnailFile, thumbnailHeader, regenerateFields)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to replace files: %v", err)})
		return
	}

	// Parse placeholders for response
	var placeholders []string
	if template.Placeholders != "" {
		json.Unmarshal([]byte(template.Placeholders), &placeholders)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "Files replaced successfully",
		"template_id":  template.ID,
		"filename":     template.Filename,
		"placeholders": placeholders,
		"template":     template,
	})
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

// GetUserDocumentHistory returns the document history for the authenticated user
func (h *DocxHandler) GetUserDocumentHistory(c *gin.Context) {
	// Get user ID from X-User-ID header (set by API gateway)
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	// Parse pagination parameters
	page := 1
	limit := 20
	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	documents, total, err := h.documentService.GetUserDocuments(userID, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get document history: %v", err)})
		return
	}

	// Calculate total pages
	totalPages := (total + int64(limit) - 1) / int64(limit)

	c.JSON(http.StatusOK, gin.H{
		"documents": documents,
		"pagination": gin.H{
			"page":   page,
			"limit":  limit,
			"total":  total,
			"pages":  totalPages,
		},
	})
}

// GetFieldDefinitions returns the field definitions for a template
func (h *DocxHandler) GetFieldDefinitions(c *gin.Context) {
	templateID := c.Param("templateId")
	if templateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Template ID is required"})
		return
	}

	definitions, err := h.templateService.GetFieldDefinitions(templateID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"field_definitions": definitions,
	})
}

// UpdateFieldDefinitionsRequest represents the request body for updating field definitions
type UpdateFieldDefinitionsRequest struct {
	FieldDefinitions map[string]utils.FieldDefinition `json:"field_definitions"`
}

// UpdateFieldDefinitions updates the field definitions for a template
func (h *DocxHandler) UpdateFieldDefinitions(c *gin.Context) {
	templateID := c.Param("templateId")
	if templateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Template ID is required"})
		return
	}

	var req UpdateFieldDefinitionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	if req.FieldDefinitions == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "field_definitions is required"})
		return
	}

	template, err := h.templateService.UpdateFieldDefinitions(templateID, req.FieldDefinitions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to update field definitions: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Field definitions updated successfully",
		"template": template,
	})
}

// RegenerateDocument regenerates a document from stored data in history
func (h *DocxHandler) RegenerateDocument(c *gin.Context) {
	documentID := c.Param("documentId")
	if documentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Document ID is required"})
		return
	}

	// Get user ID from X-User-ID header (set by API gateway)
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	document, err := h.documentService.RegenerateDocument(c.Request.Context(), documentID, userID)
	if err != nil {
		if strings.Contains(err.Error(), "unauthorized") {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to regenerate document: %v", err)})
		return
	}

	// Create temporary download link that expires in 10 minutes
	expiresAt := time.Now().Add(10 * time.Minute)
	response := ProcessResponse{
		DocumentID:  document.ID,
		DownloadURL: fmt.Sprintf("/api/v1/documents/%s/download", document.ID),
		ExpiresAt:   expiresAt.Format(time.RFC3339),
		Message:     "Document regenerated successfully. File will be deleted after 10 minutes.",
	}

	// Add PDF download URL if PDF was generated
	if document.GCSPathPdf != "" {
		response.DownloadPDFURL = fmt.Sprintf("/api/v1/documents/%s/download?format=pdf", document.ID)
	}

	// Schedule file deletion after 10 minutes
	go func() {
		time.Sleep(10 * time.Minute)
		ctx := context.Background()
		if err := h.documentService.DeleteProcessedFiles(ctx, document.ID); err != nil {
			fmt.Printf("Warning: failed to auto-delete regenerated files for document %s: %v\n", document.ID, err)
		} else {
			fmt.Printf("Auto-deleted regenerated files for document %s after 10 minutes\n", document.ID)
		}
	}()

	c.JSON(http.StatusOK, response)
}

// RegenerateFieldDefinitions regenerates field definitions from placeholders
func (h *DocxHandler) RegenerateFieldDefinitions(c *gin.Context) {
	templateID := c.Param("templateId")
	if templateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Template ID is required"})
		return
	}

	template, err := h.templateService.RegenerateFieldDefinitions(templateID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to regenerate field definitions: %v", err)})
		return
	}

	// Parse the regenerated field definitions for response
	var definitions map[string]utils.FieldDefinition
	if template.FieldDefinitions != "" {
		if err := json.Unmarshal([]byte(template.FieldDefinitions), &definitions); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse field definitions"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":           "Field definitions regenerated successfully",
		"field_definitions": definitions,
	})
}

// RegenerateThumbnails regenerates thumbnails for all templates missing them
func (h *DocxHandler) RegenerateThumbnails(c *gin.Context) {
	// Check if force regenerate is requested
	forceRegenerate := c.Query("force") == "true"

	// Optional: regenerate for a single template
	templateID := c.Query("template_id")
	if templateID != "" {
		template, err := h.templateService.GetTemplate(templateID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Template not found: %v", err)})
			return
		}

		thumbnailPath, err := h.templateService.GenerateThumbnailForTemplate(c.Request.Context(), template)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to generate thumbnail: %v", err)})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":        "Thumbnail generated successfully",
			"template_id":    templateID,
			"thumbnail_path": thumbnailPath,
		})
		return
	}

	// Regenerate for all templates
	successCount, failCount, errors := h.templateService.RegenerateThumbnailsForAllTemplates(c.Request.Context(), forceRegenerate)

	c.JSON(http.StatusOK, gin.H{
		"message":       "Thumbnail regeneration completed",
		"success_count": successCount,
		"fail_count":    failCount,
		"errors":        errors,
		"force":         forceRegenerate,
	})
}
