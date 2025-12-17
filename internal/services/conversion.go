package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
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

// GenerateThumbnailFromPDF generates a PNG thumbnail from the first page of a PDF
// Uses pdftoppm (poppler-utils) or ImageMagick convert as fallback
func (s *ConversionService) GenerateThumbnailFromPDF(ctx context.Context, pdfPath string, width int) ([]byte, error) {
	// Create temp directory for output
	tempDir, err := os.MkdirTemp("", "pdf_thumbnail_*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	outputBase := filepath.Join(tempDir, "thumb")
	var outputPath string

	// Try pdftoppm first (usually better quality and faster)
	if pdftoppmPath, err := exec.LookPath("pdftoppm"); err == nil {
		// pdftoppm generates files like thumb-1.png for first page
		cmd := exec.CommandContext(ctx, pdftoppmPath,
			"-png",                             // Output PNG format
			"-f", "1",                          // First page
			"-l", "1",                          // Last page (same as first = only first page)
			"-scale-to", fmt.Sprintf("%d", width), // Scale to width
			pdfPath,
			outputBase,
		)
		if err := cmd.Run(); err == nil {
			outputPath = outputBase + "-1.png"
			if _, err := os.Stat(outputPath); err == nil {
				content, err := os.ReadFile(outputPath)
				if err != nil {
					return nil, fmt.Errorf("failed to read thumbnail: %w", err)
				}
				return content, nil
			}
		}
	}

	// Fallback to ImageMagick convert
	if convertPath, err := exec.LookPath("convert"); err == nil {
		outputPath = filepath.Join(tempDir, "thumb.png")
		cmd := exec.CommandContext(ctx, convertPath,
			"-density", "150",                       // DPI for rendering
			"-background", "white",                  // White background
			"-alpha", "remove",                      // Remove alpha channel
			"-resize", fmt.Sprintf("%dx", width),    // Resize to width
			pdfPath+"[0]",                           // First page only
			outputPath,
		)
		if err := cmd.Run(); err == nil {
			content, err := os.ReadFile(outputPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read thumbnail: %w", err)
			}
			return content, nil
		}
	}

	// Fallback to sips on macOS
	if sipsPath, err := exec.LookPath("sips"); err == nil {
		outputPath = filepath.Join(tempDir, "thumb.png")
		cmd := exec.CommandContext(ctx, sipsPath,
			"-s", "format", "png",
			"-Z", fmt.Sprintf("%d", width),
			pdfPath,
			"--out", outputPath,
		)
		if err := cmd.Run(); err == nil {
			content, err := os.ReadFile(outputPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read thumbnail: %w", err)
			}
			return content, nil
		}
	}

	return nil, fmt.Errorf("no PDF to image converter available (tried pdftoppm, convert, sips)")
}

// IsThumbnailGenerationAvailable checks if thumbnail generation is available
func (s *ConversionService) IsThumbnailGenerationAvailable() bool {
	// Check for pdftoppm
	if _, err := exec.LookPath("pdftoppm"); err == nil {
		return true
	}
	// Check for ImageMagick convert
	if _, err := exec.LookPath("convert"); err == nil {
		return true
	}
	return false
}
