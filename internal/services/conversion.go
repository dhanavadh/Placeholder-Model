package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// ConversionService handles document format conversions (DOCX to HTML, PDF)
// Uses remote LibreOffice service via HTTP (Gotenberg-compatible API)
type ConversionService struct {
	serviceURL string
	httpClient *http.Client
	available  bool
}

// NewConversionService creates a new conversion service using remote LibreOffice service
func NewConversionService(serviceURL string, timeoutStr string) (*ConversionService, error) {
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		timeout = 60 * time.Second
	}

	httpClient := &http.Client{
		Timeout: timeout,
	}

	service := &ConversionService{
		serviceURL: serviceURL,
		httpClient: httpClient,
		available:  false,
	}

	// Check if service is available
	if serviceURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "GET", serviceURL+"/health", nil)
		if err == nil {
			resp, err := httpClient.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					service.available = true
					fmt.Printf("[INFO] LibreOffice service available at: %s\n", serviceURL)
				}
			}
		}
	}

	if !service.available {
		return nil, fmt.Errorf("LibreOffice service not available at %s", serviceURL)
	}

	return service, nil
}

// SetLibreOfficeConfig configures LibreOffice settings (kept for compatibility)
func (s *ConversionService) SetLibreOfficeConfig(enabled bool, path string) {
	// No longer needed - using remote service
}

// ConvertDocxToHTML converts a DOCX file to HTML using remote LibreOffice service
func (s *ConversionService) ConvertDocxToHTML(ctx context.Context, docxPath string) ([]byte, error) {
	if !s.available {
		return nil, fmt.Errorf("LibreOffice service is not available")
	}

	// Read the DOCX file
	docxContent, err := os.ReadFile(docxPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read DOCX file: %w", err)
	}

	return s.ConvertDocxToHTMLFromBytes(ctx, docxContent, filepath.Base(docxPath))
}

// ConvertDocxToHTMLFromBytes converts DOCX bytes to HTML
func (s *ConversionService) ConvertDocxToHTMLFromBytes(ctx context.Context, docxContent []byte, filename string) ([]byte, error) {
	if !s.available {
		return nil, fmt.Errorf("LibreOffice service is not available")
	}

	// Create multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("files", filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := part.Write(docxContent); err != nil {
		return nil, fmt.Errorf("failed to write file content: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Make request to LibreOffice service
	req, err := http.NewRequestWithContext(ctx, "POST", s.serviceURL+"/forms/libreoffice/convert/html", body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call LibreOffice service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LibreOffice service returned error: %s - %s", resp.Status, string(bodyBytes))
	}

	return io.ReadAll(resp.Body)
}

// ConvertDocxToHTMLFromReader converts DOCX from reader to HTML
func (s *ConversionService) ConvertDocxToHTMLFromReader(ctx context.Context, reader io.Reader, filename string) ([]byte, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}
	return s.ConvertDocxToHTMLFromBytes(ctx, content, filename)
}

// ConvertDocxToPDF converts a DOCX file to PDF using remote LibreOffice service
func (s *ConversionService) ConvertDocxToPDF(ctx context.Context, docxPath string) ([]byte, error) {
	if !s.available {
		return nil, fmt.Errorf("LibreOffice service is not available")
	}

	// Read the DOCX file
	docxContent, err := os.ReadFile(docxPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read DOCX file: %w", err)
	}

	return s.ConvertDocxToPDFFromBytes(ctx, docxContent, filepath.Base(docxPath))
}

// ConvertDocxToPDFFromBytes converts DOCX bytes to PDF
func (s *ConversionService) ConvertDocxToPDFFromBytes(ctx context.Context, docxContent []byte, filename string) ([]byte, error) {
	if !s.available {
		return nil, fmt.Errorf("LibreOffice service is not available")
	}

	// Create multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("files", filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := part.Write(docxContent); err != nil {
		return nil, fmt.Errorf("failed to write file content: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Make request to LibreOffice service (Gotenberg-compatible endpoint)
	req, err := http.NewRequestWithContext(ctx, "POST", s.serviceURL+"/forms/libreoffice/convert", body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call LibreOffice service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LibreOffice service returned error: %s - %s", resp.Status, string(bodyBytes))
	}

	return io.ReadAll(resp.Body)
}

// ConvertDocxToPDFFromReader converts DOCX from reader to PDF
func (s *ConversionService) ConvertDocxToPDFFromReader(ctx context.Context, reader io.Reader, filename string) ([]byte, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}
	return s.ConvertDocxToPDFFromBytes(ctx, content, filename)
}

// IsHTMLConversionAvailable checks if HTML conversion is available
func (s *ConversionService) IsHTMLConversionAvailable() bool {
	return s.available
}

// IsPDFConversionAvailable checks if PDF conversion is available
func (s *ConversionService) IsPDFConversionAvailable() bool {
	return s.available
}

// Close closes the conversion service
func (s *ConversionService) Close() error {
	return nil
}

// ThumbnailQuality represents the quality level for thumbnail generation
type ThumbnailQuality string

const (
	// ThumbnailQualityNormal is the default thumbnail quality (faster, smaller)
	ThumbnailQualityNormal ThumbnailQuality = "normal"
	// ThumbnailQualityHD is high-definition thumbnail quality (pixel-perfect, larger)
	ThumbnailQualityHD ThumbnailQuality = "hd"
)

// GenerateThumbnailFromPDF generates a PNG thumbnail from the first page of a PDF
// Uses remote LibreOffice service
func (s *ConversionService) GenerateThumbnailFromPDF(ctx context.Context, pdfPath string, width int) ([]byte, error) {
	return s.GenerateThumbnailFromPDFWithQuality(ctx, pdfPath, width, ThumbnailQualityNormal)
}

// GenerateThumbnailFromPDFWithQuality generates a PNG thumbnail with specified quality
func (s *ConversionService) GenerateThumbnailFromPDFWithQuality(ctx context.Context, pdfPath string, width int, quality ThumbnailQuality) ([]byte, error) {
	if !s.available {
		return nil, fmt.Errorf("LibreOffice service is not available")
	}

	// Read the PDF file
	pdfContent, err := os.ReadFile(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF file: %w", err)
	}

	return s.GenerateThumbnailFromPDFBytesWithQuality(ctx, pdfContent, filepath.Base(pdfPath), width, quality)
}

// GenerateThumbnailFromPDFBytes generates thumbnail from PDF bytes (normal quality)
func (s *ConversionService) GenerateThumbnailFromPDFBytes(ctx context.Context, pdfContent []byte, filename string, width int) ([]byte, error) {
	return s.GenerateThumbnailFromPDFBytesWithQuality(ctx, pdfContent, filename, width, ThumbnailQualityNormal)
}

// GenerateThumbnailFromPDFBytesWithQuality generates thumbnail from PDF bytes with specified quality
func (s *ConversionService) GenerateThumbnailFromPDFBytesWithQuality(ctx context.Context, pdfContent []byte, filename string, width int, quality ThumbnailQuality) ([]byte, error) {
	if !s.available {
		return nil, fmt.Errorf("LibreOffice service is not available")
	}

	// Create multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("files", filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := part.Write(pdfContent); err != nil {
		return nil, fmt.Errorf("failed to write file content: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Make request to LibreOffice service thumbnail endpoint with quality parameter
	url := fmt.Sprintf("%s/forms/libreoffice/thumbnail?width=%d&quality=%s", s.serviceURL, width, quality)
	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call LibreOffice service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LibreOffice service returned error: %s - %s", resp.Status, string(bodyBytes))
	}

	return io.ReadAll(resp.Body)
}

// IsThumbnailGenerationAvailable checks if thumbnail generation is available
func (s *ConversionService) IsThumbnailGenerationAvailable() bool {
	return s.available
}
