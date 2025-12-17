package services

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"DF-PLCH/internal/processor"
)

// ConversionService handles document format conversions (DOCX to HTML, PDF)
// Uses LibreOffice exclusively for all conversions
type ConversionService struct {
	libreOfficePath string
	available       bool
}

// NewConversionService creates a new conversion service using LibreOffice
func NewConversionService(gotenbergURL string, timeoutStr string) (*ConversionService, error) {
	loPath := processor.FindLibreOffice()
	available := loPath != ""

	if !available {
		return nil, fmt.Errorf("LibreOffice not found on system - required for document conversion")
	}

	return &ConversionService{
		libreOfficePath: loPath,
		available:       available,
	}, nil
}

// SetLibreOfficeConfig configures LibreOffice settings
func (s *ConversionService) SetLibreOfficeConfig(enabled bool, path string) {
	if path != "" {
		s.libreOfficePath = path
	}
	s.available = enabled && processor.IsLibreOfficeAvailable()
}

// ConvertDocxToHTML converts a DOCX file to HTML using LibreOffice
func (s *ConversionService) ConvertDocxToHTML(ctx context.Context, docxPath string) ([]byte, error) {
	if !s.available {
		return nil, fmt.Errorf("LibreOffice is not available for conversion")
	}

	return processor.ConvertToHTMLBytes(docxPath)
}

// ConvertDocxToHTMLFromReader converts DOCX from reader to HTML
func (s *ConversionService) ConvertDocxToHTMLFromReader(ctx context.Context, reader io.Reader, filename string) ([]byte, error) {
	// Create temp file for the DOCX
	tempFile, err := os.CreateTemp("", "docx_convert_*.docx")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Copy reader to temp file
	if _, err := io.Copy(tempFile, reader); err != nil {
		return nil, fmt.Errorf("failed to write temp file: %w", err)
	}
	tempFile.Close()

	return s.ConvertDocxToHTML(ctx, tempFile.Name())
}

// ConvertDocxToPDF converts a DOCX file to PDF using LibreOffice
func (s *ConversionService) ConvertDocxToPDF(ctx context.Context, docxPath string) ([]byte, error) {
	if !s.available {
		return nil, fmt.Errorf("LibreOffice is not available for conversion")
	}

	// Create temp directory for output
	tempDir, err := os.MkdirTemp("", "docx_pdf_*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Generate output path
	baseName := filepath.Base(docxPath)
	pdfName := strings.TrimSuffix(baseName, filepath.Ext(baseName)) + ".pdf"
	outputPath := filepath.Join(tempDir, pdfName)

	// Convert using LibreOffice
	if err := processor.ConvertToPDF(docxPath, outputPath); err != nil {
		return nil, fmt.Errorf("LibreOffice PDF conversion failed: %w", err)
	}

	// Read the PDF content
	content, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF file: %w", err)
	}

	return content, nil
}

// ConvertDocxToPDFFromReader converts DOCX from reader to PDF
func (s *ConversionService) ConvertDocxToPDFFromReader(ctx context.Context, reader io.Reader, filename string) ([]byte, error) {
	// Create temp file for the DOCX
	tempFile, err := os.CreateTemp("", "docx_pdf_*.docx")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Copy reader to temp file
	if _, err := io.Copy(tempFile, reader); err != nil {
		return nil, fmt.Errorf("failed to write temp file: %w", err)
	}
	tempFile.Close()

	return s.ConvertDocxToPDF(ctx, tempFile.Name())
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
			"-png",           // Output PNG format
			"-f", "1",        // First page
			"-l", "1",        // Last page (same as first = only first page)
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
			"-density", "150",           // DPI for rendering
			"-background", "white",      // White background
			"-alpha", "remove",          // Remove alpha channel
			"-resize", fmt.Sprintf("%dx", width), // Resize to width
			pdfPath+"[0]",               // First page only
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
		// sips can't directly convert PDF to PNG, but we can try
		// This is a last resort fallback
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
