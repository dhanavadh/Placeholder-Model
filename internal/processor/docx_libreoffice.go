package processor

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// LibreOfficeProcessor uses LibreOffice headless mode for DOCX processing
// with excellent format preservation
type LibreOfficeProcessor struct {
	inputFile  string
	outputFile string
	tempDir    string
	loPath     string // Path to LibreOffice executable
}

// NewLibreOfficeProcessor creates a new LibreOffice-based processor
func NewLibreOfficeProcessor(inputFile, outputFile string) *LibreOfficeProcessor {
	loPath := FindLibreOffice()
	return &LibreOfficeProcessor{
		inputFile:  inputFile,
		outputFile: outputFile,
		tempDir:    fmt.Sprintf("temp_lo_%d", time.Now().UnixNano()),
		loPath:     loPath,
	}
}

// FindLibreOffice locates the LibreOffice executable on the system
func FindLibreOffice() string {
	var paths []string

	switch runtime.GOOS {
	case "darwin":
		paths = []string{
			"/Applications/LibreOffice.app/Contents/MacOS/soffice",
			"/usr/local/bin/soffice",
			"/opt/homebrew/bin/soffice",
		}
	case "linux":
		paths = []string{
			"/usr/bin/soffice",
			"/usr/bin/libreoffice",
			"/usr/local/bin/soffice",
			"/snap/bin/libreoffice",
		}
	case "windows":
		paths = []string{
			"C:\\Program Files\\LibreOffice\\program\\soffice.exe",
			"C:\\Program Files (x86)\\LibreOffice\\program\\soffice.exe",
		}
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Try to find in PATH
	if path, err := exec.LookPath("soffice"); err == nil {
		return path
	}
	if path, err := exec.LookPath("libreoffice"); err == nil {
		return path
	}

	return "" // Not found
}

// IsLibreOfficeAvailable checks if LibreOffice is available on the system
func IsLibreOfficeAvailable() bool {
	return FindLibreOffice() != ""
}

// ProcessWithPlaceholders processes the DOCX file using LibreOffice for normalization
// LibreOffice normalizes split XML runs, then standard processor does find/replace
// This provides better format preservation than raw XML manipulation
func (p *LibreOfficeProcessor) ProcessWithPlaceholders(placeholders map[string]string) error {
	if p.loPath == "" {
		return fmt.Errorf("LibreOffice not found on system")
	}

	if err := os.MkdirAll(p.tempDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	fmt.Printf("[DEBUG] LibreOffice processor: Starting with %d replacements\n", len(placeholders))

	// Use soffice with a unique user profile to avoid conflicts
	userProfile := filepath.Join(p.tempDir, "libreoffice_profile")
	if err := os.MkdirAll(userProfile, 0755); err != nil {
		return fmt.Errorf("failed to create user profile directory: %w", err)
	}

	// Step 1: Use LibreOffice to normalize the document (fixes split XML runs)
	normalizedPath := filepath.Join(p.tempDir, "normalized.docx")
	err := p.normalizeWithLibreOffice(p.inputFile, normalizedPath, userProfile)
	if err != nil {
		fmt.Printf("[DEBUG] LibreOffice normalization failed: %v, using original file\n", err)
		normalizedPath = p.inputFile
	} else {
		fmt.Printf("[DEBUG] LibreOffice normalization completed\n")
	}

	// Step 2: Use standard DocxProcessor for find/replace on normalized document
	proc := NewDocxProcessor(normalizedPath, p.outputFile)
	if err := proc.UnzipDocx(); err != nil {
		return fmt.Errorf("failed to unzip normalized document: %w", err)
	}
	defer proc.Cleanup()

	if err := proc.FindAndReplaceInDocument(placeholders); err != nil {
		return fmt.Errorf("failed to replace placeholders: %w", err)
	}

	if err := proc.ReZipDocx(); err != nil {
		return fmt.Errorf("failed to rezip document: %w", err)
	}

	fmt.Printf("[DEBUG] LibreOffice processor: Processing completed successfully\n")
	return nil
}

// normalizeWithLibreOffice uses LibreOffice to open and resave the document
// This normalizes split XML runs and fixes formatting issues
func (p *LibreOfficeProcessor) normalizeWithLibreOffice(inputPath, outputPath, userProfile string) error {
	absInputPath, err := filepath.Abs(inputPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute input path: %w", err)
	}

	// Create output directory with absolute path
	outDir, err := filepath.Abs(filepath.Dir(outputPath))
	if err != nil {
		return fmt.Errorf("failed to get absolute output dir: %w", err)
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Get absolute user profile path
	absUserProfile, err := filepath.Abs(userProfile)
	if err != nil {
		return fmt.Errorf("failed to get absolute user profile path: %w", err)
	}

	// Use LibreOffice to convert DOCX to DOCX (normalizes the document)
	cmd := exec.Command(p.loPath,
		"--headless",
		"--invisible",
		"--nofirststartwizard",
		"--norestore",
		"--convert-to", "docx:MS Word 2007 XML",
		"--outdir", outDir,
		fmt.Sprintf("-env:UserInstallation=file://%s", absUserProfile),
		absInputPath,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	fmt.Printf("[DEBUG] Running LibreOffice: %s\n", cmd.String())

	// Set timeout - use 30 seconds for faster feedback
	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("LibreOffice execution failed: %v, stdout: %s, stderr: %s", err, stdout.String(), stderr.String())
		}
		fmt.Printf("[DEBUG] LibreOffice output: %s\n", stdout.String())
	case <-time.After(30 * time.Second):
		cmd.Process.Kill()
		return fmt.Errorf("LibreOffice execution timed out after 30 seconds")
	}

	// LibreOffice outputs with the original filename in outDir
	baseName := filepath.Base(inputPath)
	convertedPath := filepath.Join(outDir, baseName)

	// Rename to expected output path if different
	if convertedPath != outputPath {
		if err := os.Rename(convertedPath, outputPath); err != nil {
			// If rename fails, try copy
			if err := copyFile(convertedPath, outputPath); err != nil {
				return fmt.Errorf("failed to move converted file: %w", err)
			}
			os.Remove(convertedPath)
		}
	}

	// Verify output exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		return fmt.Errorf("output file was not created")
	}

	return nil
}

// Cleanup removes temporary files
func (p *LibreOfficeProcessor) Cleanup() {
	if p.tempDir != "" {
		os.RemoveAll(p.tempDir)
	}
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// ExtractPlaceholders extracts placeholders from the DOCX file
// This uses the regular DocxProcessor internally as LibreOffice isn't needed for reading
func (p *LibreOfficeProcessor) ExtractPlaceholders() ([]string, error) {
	// Use the standard DocxProcessor for extraction
	proc := NewDocxProcessor(p.inputFile, p.outputFile)
	if err := proc.UnzipDocx(); err != nil {
		return nil, fmt.Errorf("failed to unzip for placeholder extraction: %w", err)
	}
	defer proc.Cleanup()

	return proc.ExtractPlaceholders()
}

// DetectOrientation detects if the document is in landscape orientation
func (p *LibreOfficeProcessor) DetectOrientation() (bool, error) {
	// Use the standard DocxProcessor for orientation detection
	proc := NewDocxProcessor(p.inputFile, p.outputFile)
	if err := proc.UnzipDocx(); err != nil {
		return false, fmt.Errorf("failed to unzip for orientation detection: %w", err)
	}
	defer proc.Cleanup()

	return proc.DetectOrientation()
}
