package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"DF-PLCH/internal"
	"DF-PLCH/internal/models"
	"DF-PLCH/internal/services"

	"github.com/gin-gonic/gin"
)

type OCRHandler struct {
	ocrService *services.OCRService
}

func NewOCRHandler() *OCRHandler {
	return &OCRHandler{
		ocrService: services.NewOCRService(),
	}
}

// getTemplate retrieves a template by ID from database
func (h *OCRHandler) getTemplate(templateID string) (*models.Template, error) {
	var template models.Template
	if err := internal.DB.First(&template, "id = ?", templateID).Error; err != nil {
		return nil, err
	}
	return &template, nil
}

// parsePlaceholders parses placeholders JSON string to slice
func (h *OCRHandler) parsePlaceholders(placeholdersJSON string) []string {
	var placeholders []string
	if err := json.Unmarshal([]byte(placeholdersJSON), &placeholders); err != nil {
		return []string{}
	}
	return placeholders
}

// OCRRequest is the request body for OCR endpoint
type OCRRequest struct {
	Image      string `json:"image"`       // Base64 encoded image
	TemplateID string `json:"template_id"` // Optional: for auto-mapping to placeholders
}

// OCRResponse is the response from OCR endpoint
type OCRResponse struct {
	RawText        string            `json:"raw_text"`
	ExtractedData  map[string]string `json:"extracted_data"`
	MappedFields   map[string]string `json:"mapped_fields,omitempty"`
	DetectionScore int               `json:"detection_score"`
	Message        string            `json:"message"`
}

// TyphoonOCRRequest is the request body for Typhoon OCR endpoint
type TyphoonOCRRequest struct {
	Image      string                   `json:"image"`       // Base64 encoded image
	TemplateID string                   `json:"template_id"` // Optional: for auto-mapping to placeholders
	Params     *services.TyphoonOCRParams `json:"params,omitempty"` // Optional OCR parameters
}

// TyphoonOCRResponse is the response from Typhoon OCR endpoint
type TyphoonOCRResponse struct {
	RawText        string                          `json:"raw_text"`
	ExtractedData  *services.TyphoonExtractedData  `json:"extracted_data"`
	MappedFields   map[string]string               `json:"mapped_fields,omitempty"`
	FieldMappings  map[string]services.FormFieldMapping `json:"field_mappings,omitempty"`
	DetectionScore int                             `json:"detection_score"`
	DocumentType   string                          `json:"document_type"`
	Provider       string                          `json:"provider"`
	Message        string                          `json:"message"`
}

// ExtractText handles OCR text extraction from image
// POST /api/v1/ocr/extract
func (h *OCRHandler) ExtractText(c *gin.Context) {
	var req OCRRequest

	// Check if it's multipart form (file upload) or JSON
	contentType := c.GetHeader("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Handle file upload
		file, err := c.FormFile("image")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Image file is required"})
			return
		}

		// Validate file type
		if !strings.HasPrefix(file.Header.Get("Content-Type"), "image/") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "File must be an image"})
			return
		}

		// Read file content
		f, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
			return
		}
		defer f.Close()

		imageData, err := io.ReadAll(f)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read file"})
			return
		}

		req.Image = base64.StdEncoding.EncodeToString(imageData)
		req.TemplateID = c.PostForm("template_id")
	} else {
		// Handle JSON request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}
	}

	if req.Image == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Image is required"})
		return
	}

	// Remove data URL prefix if present
	if strings.HasPrefix(req.Image, "data:image") {
		parts := strings.SplitN(req.Image, ",", 2)
		if len(parts) == 2 {
			req.Image = parts[1]
		}
	}

	// Extract text from image
	result, err := h.ocrService.ExtractTextFromImage(req.Image)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := OCRResponse{
		RawText:        result.RawText,
		ExtractedData:  result.ExtractedData,
		DetectionScore: result.DetectionScore,
		Message:        "Text extracted successfully",
	}

	// If template ID provided, map to placeholders using Gemini AI
	if req.TemplateID != "" {
		template, err := h.getTemplate(req.TemplateID)
		if err == nil && template != nil {
			placeholders := h.parsePlaceholders(template.Placeholders)
			response.MappedFields = h.ocrService.MapToPlaceholdersWithRawText(result.ExtractedData, placeholders, result.RawText)
		}
	}

	c.JSON(http.StatusOK, response)
}

// ExtractForTemplate handles OCR extraction and mapping for a specific template
// POST /api/v1/templates/:templateId/ocr
func (h *OCRHandler) ExtractForTemplate(c *gin.Context) {
	templateID := c.Param("templateId")
	if templateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Template ID is required"})
		return
	}

	// Get template
	template, err := h.getTemplate(templateID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}

	// Get image from request
	var imageBase64 string

	contentType := c.GetHeader("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		file, err := c.FormFile("image")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Image file is required"})
			return
		}

		f, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
			return
		}
		defer f.Close()

		imageData, err := io.ReadAll(f)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read file"})
			return
		}

		imageBase64 = base64.StdEncoding.EncodeToString(imageData)
	} else {
		var req struct {
			Image string `json:"image"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}
		imageBase64 = req.Image

		// Remove data URL prefix if present
		if strings.HasPrefix(imageBase64, "data:image") {
			parts := strings.SplitN(imageBase64, ",", 2)
			if len(parts) == 2 {
				imageBase64 = parts[1]
			}
		}
	}

	if imageBase64 == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Image is required"})
		return
	}

	// Extract text from image
	result, err := h.ocrService.ExtractTextFromImage(imageBase64)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Map to template placeholders using Gemini AI
	placeholders := h.parsePlaceholders(template.Placeholders)
	mappedFields := h.ocrService.MapToPlaceholdersWithRawText(result.ExtractedData, placeholders, result.RawText)

	response := OCRResponse{
		RawText:        result.RawText,
		ExtractedData:  result.ExtractedData,
		MappedFields:   mappedFields,
		DetectionScore: result.DetectionScore,
		Message:        "Text extracted and mapped successfully",
	}

	c.JSON(http.StatusOK, response)
}

// ============================================================================
// Typhoon OCR Endpoints
// ============================================================================

// ExtractWithTyphoon handles OCR extraction using Typhoon OCR API
// POST /api/v1/ocr/typhoon
func (h *OCRHandler) ExtractWithTyphoon(c *gin.Context) {
	var req TyphoonOCRRequest

	// Check if it's multipart form (file upload) or JSON
	contentType := c.GetHeader("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Handle file upload
		file, err := c.FormFile("image")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Image file is required"})
			return
		}

		// Validate file type
		if !strings.HasPrefix(file.Header.Get("Content-Type"), "image/") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "File must be an image"})
			return
		}

		// Read file content
		f, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
			return
		}
		defer f.Close()

		imageData, err := io.ReadAll(f)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read file"})
			return
		}

		req.Image = base64.StdEncoding.EncodeToString(imageData)
		req.TemplateID = c.PostForm("template_id")
	} else {
		// Handle JSON request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}
	}

	if req.Image == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Image is required"})
		return
	}

	// Remove data URL prefix if present
	if strings.HasPrefix(req.Image, "data:image") {
		parts := strings.SplitN(req.Image, ",", 2)
		if len(parts) == 2 {
			req.Image = parts[1]
		}
	}

	// Get placeholders if template ID provided
	var placeholders []string
	if req.TemplateID != "" {
		template, err := h.getTemplate(req.TemplateID)
		if err == nil && template != nil {
			placeholders = h.parsePlaceholders(template.Placeholders)
		}
	}

	// Extract and map using Typhoon OCR
	result, fieldMappings, err := h.ocrService.ExtractAndMapToForm(req.Image, placeholders)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Build response
	response := TyphoonOCRResponse{
		RawText:        result.RawText,
		ExtractedData:  result.ExtractedData,
		MappedFields:   result.MappedFields,
		FieldMappings:  fieldMappings,
		DetectionScore: result.DetectionScore,
		Provider:       result.Provider,
		Message:        "Text extracted and mapped successfully",
	}

	if result.ExtractedData != nil {
		response.DocumentType = result.ExtractedData.DocumentType
	}

	c.JSON(http.StatusOK, response)
}

// ExtractTyphoonForTemplate handles Typhoon OCR extraction for a specific template
// POST /api/v1/templates/:templateId/ocr/typhoon
func (h *OCRHandler) ExtractTyphoonForTemplate(c *gin.Context) {
	templateID := c.Param("templateId")
	if templateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Template ID is required"})
		return
	}

	// Get template
	template, err := h.getTemplate(templateID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}

	// Get image from request
	var imageBase64 string
	var params *services.TyphoonOCRParams

	contentType := c.GetHeader("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		file, err := c.FormFile("image")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Image file is required"})
			return
		}

		f, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
			return
		}
		defer f.Close()

		imageData, err := io.ReadAll(f)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read file"})
			return
		}

		imageBase64 = base64.StdEncoding.EncodeToString(imageData)
	} else {
		var req TyphoonOCRRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}
		imageBase64 = req.Image
		params = req.Params

		// Remove data URL prefix if present
		if strings.HasPrefix(imageBase64, "data:image") {
			parts := strings.SplitN(imageBase64, ",", 2)
			if len(parts) == 2 {
				imageBase64 = parts[1]
			}
		}
	}

	if imageBase64 == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Image is required"})
		return
	}

	// Get placeholders
	placeholders := h.parsePlaceholders(template.Placeholders)

	// Extract and map using Typhoon OCR
	result, fieldMappings, err := h.ocrService.ExtractAndMapToForm(imageBase64, placeholders)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Log params usage (for future enhancement)
	_ = params

	// Build response
	response := TyphoonOCRResponse{
		RawText:        result.RawText,
		ExtractedData:  result.ExtractedData,
		MappedFields:   result.MappedFields,
		FieldMappings:  fieldMappings,
		DetectionScore: result.DetectionScore,
		Provider:       result.Provider,
		Message:        fmt.Sprintf("Text extracted and mapped to template '%s'", template.DisplayName),
	}

	if result.ExtractedData != nil {
		response.DocumentType = result.ExtractedData.DocumentType
	}

	c.JSON(http.StatusOK, response)
}

// GetFormFieldMappings returns field mappings for OCR results
// POST /api/v1/ocr/map-fields
func (h *OCRHandler) GetFormFieldMappings(c *gin.Context) {
	var req struct {
		Image        string   `json:"image"`        // Base64 encoded image
		Placeholders []string `json:"placeholders"` // List of placeholder names to map to
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.Image == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Image is required"})
		return
	}

	if len(req.Placeholders) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Placeholders are required"})
		return
	}

	// Remove data URL prefix if present
	if strings.HasPrefix(req.Image, "data:image") {
		parts := strings.SplitN(req.Image, ",", 2)
		if len(parts) == 2 {
			req.Image = parts[1]
		}
	}

	// Extract and map
	result, fieldMappings, err := h.ocrService.ExtractAndMapToForm(req.Image, req.Placeholders)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"raw_text":        result.RawText,
		"extracted_data":  result.ExtractedData,
		"mapped_fields":   result.MappedFields,
		"field_mappings":  fieldMappings,
		"detection_score": result.DetectionScore,
		"document_type":   result.ExtractedData.DocumentType,
		"provider":        result.Provider,
		"message":         "Fields mapped successfully",
	})
}

// SmartOCRExtract intelligently selects the best OCR provider and maps to form fields
// POST /api/v1/ocr/smart
func (h *OCRHandler) SmartOCRExtract(c *gin.Context) {
	var req struct {
		Image      string `json:"image"`       // Base64 encoded image
		TemplateID string `json:"template_id"` // Optional: for auto-mapping
		Provider   string `json:"provider"`    // Optional: "typhoon", "vision", or "auto" (default)
	}

	// Handle both JSON and multipart form
	contentType := c.GetHeader("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		file, err := c.FormFile("image")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Image file is required"})
			return
		}

		f, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
			return
		}
		defer f.Close()

		imageData, err := io.ReadAll(f)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read file"})
			return
		}

		req.Image = base64.StdEncoding.EncodeToString(imageData)
		req.TemplateID = c.PostForm("template_id")
		req.Provider = c.PostForm("provider")
	} else {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}
	}

	if req.Image == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Image is required"})
		return
	}

	// Remove data URL prefix if present
	if strings.HasPrefix(req.Image, "data:image") {
		parts := strings.SplitN(req.Image, ",", 2)
		if len(parts) == 2 {
			req.Image = parts[1]
		}
	}

	// Get placeholders if template ID provided
	var placeholders []string
	var templateName string
	if req.TemplateID != "" {
		template, err := h.getTemplate(req.TemplateID)
		if err == nil && template != nil {
			placeholders = h.parsePlaceholders(template.Placeholders)
			templateName = template.DisplayName
		}
	}

	// Use the smart extraction method
	result, fieldMappings, err := h.ocrService.ExtractAndMapToForm(req.Image, placeholders)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Build response
	response := gin.H{
		"raw_text":        result.RawText,
		"extracted_data":  result.ExtractedData,
		"mapped_fields":   result.MappedFields,
		"field_mappings":  fieldMappings,
		"detection_score": result.DetectionScore,
		"provider":        result.Provider,
		"message":         "OCR extraction completed successfully",
	}

	if result.ExtractedData != nil {
		response["document_type"] = result.ExtractedData.DocumentType
	}

	if templateName != "" {
		response["template_name"] = templateName
	}

	// Add confidence summary
	if len(fieldMappings) > 0 {
		highConfidence := 0
		lowConfidence := 0
		for _, fm := range fieldMappings {
			if fm.Confidence >= 80 {
				highConfidence++
			} else {
				lowConfidence++
			}
		}
		response["confidence_summary"] = gin.H{
			"high_confidence": highConfidence,
			"low_confidence":  lowConfidence,
			"total_mapped":    len(fieldMappings),
		}
	}

	c.JSON(http.StatusOK, response)
}
