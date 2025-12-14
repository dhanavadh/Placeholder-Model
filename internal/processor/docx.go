package processor

import (
	"archive/zip"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type DocxProcessor struct {
	inputFile  string
	outputFile string
	tempDir    string
}

func NewDocxProcessor(inputFile, outputFile string) *DocxProcessor {
	return &DocxProcessor{
		inputFile:  inputFile,
		outputFile: outputFile,
		tempDir:    fmt.Sprintf("temp_docx_%d", time.Now().UnixNano()),
	}
}

func (dp *DocxProcessor) UnzipDocx() error {
	fmt.Printf("[DEBUG] Starting DOCX unzip for file: %s\n", dp.inputFile)
	reader, err := zip.OpenReader(dp.inputFile)
	if err != nil {
		return fmt.Errorf("failed to open docx file: %w", err)
	}
	defer reader.Close()
	fmt.Printf("[DEBUG] DOCX file opened successfully\n")

	fmt.Printf("[DEBUG] Creating temp directory: %s\n", dp.tempDir)
	err = os.MkdirAll(dp.tempDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	fmt.Printf("[DEBUG] Temp directory created successfully\n")

	fmt.Printf("[DEBUG] Found %d files in DOCX archive\n", len(reader.File))
	for i, file := range reader.File {
		fmt.Printf("[DEBUG] Extracting file %d/%d: %s\n", i+1, len(reader.File), file.Name)
		err := dp.extractFile(file)
		if err != nil {
			return fmt.Errorf("failed to extract file %s: %w", file.Name, err)
		}
		fmt.Printf("[DEBUG] Successfully extracted: %s\n", file.Name)
	}

	fmt.Printf("[DEBUG] DOCX unzip completed successfully\n")
	return nil
}

func (dp *DocxProcessor) extractFile(file *zip.File) error {
	rc, err := file.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	// Security: Sanitize file path to prevent ZIP Slip attack
	cleanName := filepath.Clean(file.Name)

	// Reject paths with parent directory references
	if strings.HasPrefix(cleanName, "..") || strings.Contains(cleanName, string(os.PathSeparator)+"..") || strings.Contains(cleanName, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("invalid file path in archive (path traversal attempt): %s", file.Name)
	}

	path := filepath.Join(dp.tempDir, cleanName)

	// Security: Verify the final path is within tempDir
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}
	absTempDir, err := filepath.Abs(dp.tempDir)
	if err != nil {
		return fmt.Errorf("failed to resolve temp dir: %w", err)
	}
	if !strings.HasPrefix(absPath, absTempDir+string(os.PathSeparator)) && absPath != absTempDir {
		return fmt.Errorf("file path escapes temp directory: %s", file.Name)
	}

	if file.FileInfo().IsDir() {
		os.MkdirAll(path, file.FileInfo().Mode())
		return nil
	}

	os.MkdirAll(filepath.Dir(path), 0755)

	outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.FileInfo().Mode())
	if err != nil {
		return err
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, rc)
	return err
}

// escapeXML escapes special XML characters to prevent corrupting the document
func escapeXML(s string) string {
	return html.EscapeString(s)
}

func (dp *DocxProcessor) FindAndReplaceInDocument(placeholders map[string]string) error {
	documentPath := filepath.Join(dp.tempDir, "word", "document.xml")
	fmt.Printf("[DEBUG] Reading document.xml for replacement from: %s\n", documentPath)

	content, err := os.ReadFile(documentPath)
	if err != nil {
		return fmt.Errorf("failed to read document.xml: %w", err)
	}
	fmt.Printf("[DEBUG] Document.xml read for replacement, size: %d bytes\n", len(content))

	contentStr := string(content)
	fmt.Printf("[DEBUG] Starting replacement for %d placeholders\n", len(placeholders))

	i := 0
	for placeholder, value := range placeholders {
		i++
		// Escape XML special characters to prevent document corruption
		escapedValue := escapeXML(value)
		fmt.Printf("[DEBUG] Replacing placeholder %d/%d: %s -> '%s' (escaped: '%s')\n", i, len(placeholders), placeholder, value, escapedValue)
		contentStr = dp.replaceWithXMLHandling(contentStr, placeholder, escapedValue)
		fmt.Printf("[DEBUG] Replacement %d/%d completed\n", i, len(placeholders))
	}

	fmt.Printf("[DEBUG] All replacements completed, writing back to file...\n")
	err = os.WriteFile(documentPath, []byte(contentStr), 0644)
	if err != nil {
		return fmt.Errorf("failed to write document.xml: %w", err)
	}
	fmt.Printf("[DEBUG] Document.xml written successfully\n")

	return nil
}

func (dp *DocxProcessor) replaceWithXMLHandling(content, placeholder, value string) string {
	fmt.Printf("[DEBUG] Starting XML-aware replacement for placeholder: %s\n", placeholder)

	// First try simple replacement if placeholder exists as-is
	if strings.Contains(content, placeholder) {
		fmt.Printf("[DEBUG] Found exact match, using simple replacement\n")
		return strings.ReplaceAll(content, placeholder, value)
	}

	fmt.Printf("[DEBUG] No exact match found, attempting XML-safe replacement\n")

	// Use the safer approach that properly handles XML structure
	result, replaced := dp.replaceXMLSplit(content, placeholder, value)
	if replaced {
		fmt.Printf("[DEBUG] XML-safe replacement completed successfully\n")
	} else {
		fmt.Printf("[DEBUG] WARNING: Could not replace split placeholder %s - document may need manual fixing\n", placeholder)
	}

	return result
}

// replaceXMLSplit handles placeholders that are split across XML text nodes
// It finds text within <w:t> tags and handles placeholders that span multiple tags
func (dp *DocxProcessor) replaceXMLSplit(content, placeholder, value string) (string, bool) {
	// Find all text content between <w:t> tags and track their positions
	type textSpan struct {
		start int    // Start position in content (after <w:t>)
		end   int    // End position in content (before </w:t>)
		text  string // The text content
	}

	var spans []textSpan
	pos := 0

	for {
		// Find <w:t> or <w:t ...>
		tagStart := strings.Index(content[pos:], "<w:t")
		if tagStart == -1 {
			break
		}
		tagStart += pos

		// Find end of opening tag
		tagEnd := strings.Index(content[tagStart:], ">")
		if tagEnd == -1 {
			break
		}
		tagEnd += tagStart + 1

		// Find </w:t>
		closeTag := strings.Index(content[tagEnd:], "</w:t>")
		if closeTag == -1 {
			pos = tagEnd
			continue
		}
		closeTag += tagEnd

		spans = append(spans, textSpan{
			start: tagEnd,
			end:   closeTag,
			text:  content[tagEnd:closeTag],
		})

		pos = closeTag + 6
	}

	if len(spans) == 0 {
		return content, false
	}

	// Concatenate all text to find placeholder
	var fullText strings.Builder
	for _, span := range spans {
		fullText.WriteString(span.text)
	}
	concatenated := fullText.String()

	// Find placeholder in concatenated text
	idx := strings.Index(concatenated, placeholder)
	if idx == -1 {
		return content, false
	}

	// Find which spans contain the placeholder
	charCount := 0
	startSpanIdx := -1
	startOffset := 0
	endSpanIdx := -1
	endOffset := 0

	placeholderEnd := idx + len(placeholder)

	for i, span := range spans {
		spanStart := charCount
		spanEnd := charCount + len(span.text)

		// Check if placeholder starts in this span
		if startSpanIdx == -1 && idx >= spanStart && idx < spanEnd {
			startSpanIdx = i
			startOffset = idx - spanStart
		}

		// Check if placeholder ends in this span
		if placeholderEnd > spanStart && placeholderEnd <= spanEnd {
			endSpanIdx = i
			endOffset = placeholderEnd - spanStart
			break
		}

		charCount = spanEnd
	}

	if startSpanIdx == -1 || endSpanIdx == -1 {
		return content, false
	}

	// Build the result by modifying the spans
	var result strings.Builder
	lastEnd := 0

	for i, span := range spans {
		// Copy content before this span
		result.WriteString(content[lastEnd:span.start])

		if i == startSpanIdx && i == endSpanIdx {
			// Placeholder is within a single span
			newText := span.text[:startOffset] + value + span.text[endOffset:]
			result.WriteString(newText)
		} else if i == startSpanIdx {
			// Start of placeholder - put replacement value here
			newText := span.text[:startOffset] + value
			result.WriteString(newText)
		} else if i > startSpanIdx && i < endSpanIdx {
			// Middle spans - empty them
			// (text is already consumed by the replacement)
		} else if i == endSpanIdx {
			// End of placeholder - keep text after placeholder
			newText := span.text[endOffset:]
			result.WriteString(newText)
		} else {
			// Not part of placeholder - keep as-is
			result.WriteString(span.text)
		}

		lastEnd = span.end
	}

	// Copy remaining content after last span
	result.WriteString(content[lastEnd:])

	// Recursively replace if there are more occurrences
	resultStr := result.String()
	if strings.Contains(dp.removeXMLTags(resultStr), placeholder) {
		return dp.replaceXMLSplit(resultStr, placeholder, value)
	}

	return resultStr, true
}

func (dp *DocxProcessor) checkPlaceholderMatch(content string, startPos int, placeholder string) (bool, int) {
	placeholderChars := []rune(placeholder)
	contentRunes := []rune(content)

	if startPos >= len(contentRunes) {
		return false, startPos
	}

	matchIndex := 0
	pos := startPos
	inTag := false

	for pos < len(contentRunes) && matchIndex < len(placeholderChars) {
		char := contentRunes[pos]

		if char == '<' {
			inTag = true
		} else if char == '>' {
			inTag = false
		} else if !inTag {
			if char == placeholderChars[matchIndex] {
				matchIndex++
			} else {
				return false, startPos
			}
		}

		pos++
	}

	return matchIndex == len(placeholderChars), pos
}

func (dp *DocxProcessor) ReZipDocx() error {
	outputFile, err := os.Create(dp.outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	zipWriter := zip.NewWriter(outputFile)
	defer zipWriter.Close()

	return filepath.Walk(dp.tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(dp.tempDir, path)
		if err != nil {
			return err
		}

		relPath = filepath.ToSlash(relPath)

		zipFile, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(zipFile, file)
		return err
	})
}

func (dp *DocxProcessor) Cleanup() {
	os.RemoveAll(dp.tempDir)
}

func (dp *DocxProcessor) ExtractPlaceholders() ([]string, error) {
	documentPath := filepath.Join(dp.tempDir, "word", "document.xml")
	content, err := os.ReadFile(documentPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read document.xml: %w", err)
	}

	cleanText := dp.removeXMLTags(string(content))

	var placeholders []string
	seen := make(map[string]bool)
	pos := 0

	for {
		startIdx := strings.Index(cleanText[pos:], "{{")
		if startIdx == -1 {
			break
		}
		startIdx += pos

		endIdx := strings.Index(cleanText[startIdx:], "}}")
		if endIdx == -1 {
			break
		}
		endIdx += startIdx + 2

		placeholder := cleanText[startIdx:endIdx]
		if !seen[placeholder] {
			placeholders = append(placeholders, placeholder)
			seen[placeholder] = true
		}
		pos = endIdx
	}

	return placeholders, nil
}

func (dp *DocxProcessor) DetectOrientation() (bool, error) {
	contentPath := filepath.Join(dp.tempDir, "word", "document.xml")
	content, err := os.ReadFile(contentPath)
	if err != nil {
		return false, fmt.Errorf("failed to read document.xml: %w", err)
	}

	contentStr := string(content)

	// Look for section properties (w:sectPr)
	sectStart := strings.Index(contentStr, "<w:sectPr")
	if sectStart == -1 {
		return false, nil
	}

	sectEnd := strings.Index(contentStr[sectStart:], "</w:sectPr>")
	if sectEnd == -1 {
		return false, nil
	}
	sectContent := contentStr[sectStart : sectStart+sectEnd]

	// Check for explicit orientation setting (w:orient attribute in w:pgSz)
	pgSzStart := strings.Index(sectContent, "<w:pgSz")
	if pgSzStart == -1 {
		return false, nil
	}

	pgSzEnd := strings.Index(sectContent[pgSzStart:], "/>")
	if pgSzEnd == -1 {
		return false, nil
	}
	pgSzTag := sectContent[pgSzStart : pgSzStart+pgSzEnd]

	// Check for w:orient attribute
	orientStart := strings.Index(pgSzTag, `w:orient="`)
	if orientStart != -1 {
		orientStart += 10
		orientEnd := strings.Index(pgSzTag[orientStart:], `"`)
		if orientEnd != -1 {
			return pgSzTag[orientStart:orientStart+orientEnd] == "landscape", nil
		}
	}

	// If no explicit orientation, check width vs height
	width := dp.parseAttributeValue(pgSzTag, "w:w")
	height := dp.parseAttributeValue(pgSzTag, "w:h")
	if width > 0 && height > 0 {
		return width > height, nil
	}

	return false, nil
}

func (dp *DocxProcessor) parseAttributeValue(tag, attr string) float64 {
	start := strings.Index(tag, attr+`="`)
	if start == -1 {
		return 0
	}
	start += len(attr) + 2
	end := strings.Index(tag[start:], `"`)
	if end == -1 {
		return 0
	}

	var result float64
	for _, r := range tag[start : start+end] {
		if r >= '0' && r <= '9' {
			result = result*10 + float64(r-'0')
		}
	}
	return result
}

func (dp *DocxProcessor) removeXMLTags(content string) string {
	var result strings.Builder
	inTag := false

	for _, char := range content {
		if char == '<' {
			inTag = true
		} else if char == '>' {
			inTag = false
		} else if !inTag {
			result.WriteRune(char)
		}
	}

	return result.String()
}

