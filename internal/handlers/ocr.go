package handlers

import (
	"encoding/base64"
	"encoding/json"
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
