package services

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type OCRService struct {
	visionAPIKey   string
	geminiAPIKey   string
	addressAPIURL  string
	typhoonAPIKey  string
	typhoonAPIURL  string
}

func NewOCRService() *OCRService {
	return &OCRService{
		visionAPIKey:   os.Getenv("GOOGLE_VISION_API_KEY"),
		geminiAPIKey:   os.Getenv("GOOGLE_AI_API_KEY"),
		addressAPIURL:  os.Getenv("ADDRESS_API_URL"),
		typhoonAPIKey:  os.Getenv("TYPHOON_API_KEY"),
		typhoonAPIURL:  os.Getenv("TYPHOON_API_URL"), // Default: https://api.opentyphoon.ai/v1/ocr
	}
}

// AddressSearchResult represents address API response
type AddressSearchResult struct {
	ObjectID  int    `json:"OBJECTID"`
	AdminID1  string `json:"ADMIN_ID1"`
	AdminID2  string `json:"ADMIN_ID2"`
	AdminID3  string `json:"ADMIN_ID3"`
	Name1     string `json:"NAME1"`     // Province (Thai)
	NameEng1  string `json:"NAME_ENG1"` // Province (English)
	Name2     string `json:"NAME2"`     // District (Thai)
	NameEng2  string `json:"NAME_ENG2"` // District (English)
	Name3     string `json:"NAME3"`     // Sub-district (Thai)
	NameEng3  string `json:"NAME_ENG3"` // Sub-district (English)
}

// VisionRequest is the request structure for Google Cloud Vision API
type VisionRequest struct {
	Requests []VisionRequestItem `json:"requests"`
}

type VisionRequestItem struct {
	Image    VisionImage     `json:"image"`
	Features []VisionFeature `json:"features"`
}

type VisionImage struct {
	Content string `json:"content"`
}

type VisionFeature struct {
	Type       string `json:"type"`
	MaxResults int    `json:"maxResults"`
}

// VisionResponse is the response structure from Google Cloud Vision API
type VisionResponse struct {
	Responses []VisionResponseItem `json:"responses"`
}

type VisionResponseItem struct {
	FullTextAnnotation *FullTextAnnotation `json:"fullTextAnnotation"`
	Error              *VisionError        `json:"error"`
}

type FullTextAnnotation struct {
	Text string `json:"text"`
}

type VisionError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// OCRResult is the result of OCR processing
type OCRResult struct {
	RawText        string            `json:"raw_text"`
	ExtractedData  map[string]string `json:"extracted_data"`
	DetectionScore int               `json:"detection_score"`
}

// Thai digit to Arabic digit mapping
var thaiDigits = map[rune]rune{
	'๐': '0', '๑': '1', '๒': '2', '๓': '3', '๔': '4',
	'๕': '5', '๖': '6', '๗': '7', '๘': '8', '๙': '9',
}

// Thai month to English month mapping
var thaiMonths = map[string]string{
	"มกราคม": "Jan", "ม.ค.": "Jan", "ม.ค": "Jan",
	"กุมภาพันธ์": "Feb", "ก.พ.": "Feb", "ก.พ": "Feb",
	"มีนาคม": "Mar", "มี.ค.": "Mar", "มี.ค": "Mar",
	"เมษายน": "Apr", "เม.ย.": "Apr", "เม.ย": "Apr",
	"พฤษภาคม": "May", "พ.ค.": "May", "พ.ค": "May",
	"มิถุนายน": "Jun", "มิ.ย.": "Jun", "มิ.ย": "Jun",
	"กรกฎาคม": "Jul", "ก.ค.": "Jul", "ก.ค": "Jul",
	"สิงหาคม": "Aug", "ส.ค.": "Aug", "ส.ค": "Aug",
	"กันยายน": "Sep", "ก.ย.": "Sep", "ก.ย": "Sep",
	"ตุลาคม": "Oct", "ต.ค.": "Oct", "ต.ค": "Oct",
	"พฤศจิกายน": "Nov", "พ.ย.": "Nov", "พ.ย": "Nov",
	"ธันวาคม": "Dec", "ธ.ค.": "Dec", "ธ.ค": "Dec",
}

// normalizeThaiDigits converts Thai digits to Arabic digits
func normalizeThaiDigits(text string) string {
	var result strings.Builder
	for _, r := range text {
		if arabic, ok := thaiDigits[r]; ok {
			result.WriteRune(arabic)
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// convertThaiDate converts Thai Buddhist Era date to Gregorian
func convertThaiDate(thaiDate string) string {
	// Normalize Thai digits first
	normalized := normalizeThaiDigits(thaiDate)

	// Pattern: DD MMM YYYY (Thai)
	datePattern := regexp.MustCompile(`(\d{1,2})\s+([ก-๏\.]+)\s+(\d{4})`)
	matches := datePattern.FindStringSubmatch(normalized)

	if matches == nil {
		return thaiDate
	}

	day := matches[1]
	thaiMonth := strings.TrimSpace(matches[2])
	yearStr := matches[3]

	// Convert Thai month to English
	engMonth := "Jan"
	for thai, eng := range thaiMonths {
		if strings.Contains(thaiMonth, thai) || thaiMonth == thai {
			engMonth = eng
			break
		}
	}

	// Convert Buddhist Era to Gregorian (BE - 543 = CE)
	year, err := strconv.Atoi(yearStr)
	if err != nil {
		return thaiDate
	}
	if year > 2400 { // Likely Buddhist Era
		year -= 543
	}

	return fmt.Sprintf("%s %s %d", day, engMonth, year)
}

// validateThaiID validates Thai ID number checksum
func validateThaiID(id string) bool {
	// Remove non-digits
	digits := regexp.MustCompile(`\D`).ReplaceAllString(id, "")
	if len(digits) != 13 {
		return false
	}

	sum := 0
	for i := 0; i < 12; i++ {
		digit, _ := strconv.Atoi(string(digits[i]))
		sum += digit * (13 - i)
	}

	checksum := (11 - (sum % 11)) % 10
	lastDigit, _ := strconv.Atoi(string(digits[12]))

	return checksum == lastDigit
}

// GeminiRequest is the request structure for Gemini API
type GeminiRequest struct {
	Contents []GeminiContent `json:"contents"`
}

type GeminiContent struct {
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text string `json:"text,omitempty"`
}

// GeminiResponse is the response structure from Gemini API
type GeminiResponse struct {
	Candidates []GeminiCandidate `json:"candidates"`
	Error      *GeminiError      `json:"error,omitempty"`
}

type GeminiCandidate struct {
	Content GeminiContent `json:"content"`
}

type GeminiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ExtractTextFromImage calls Typhoon OCR API to extract text (OpenAI-compatible vision API)
func (s *OCRService) ExtractTextFromImage(imageBase64 string) (*OCRResult, error) {
	if s.typhoonAPIKey == "" {
		return nil, fmt.Errorf("TYPHOON_API_KEY not configured")
	}

	// Detect image type from base64 header or default to jpeg
	imageType := "jpeg"
	if strings.HasPrefix(imageBase64, "/9j/") {
		imageType = "jpeg"
	} else if strings.HasPrefix(imageBase64, "iVBOR") {
		imageType = "png"
	} else if strings.HasPrefix(imageBase64, "R0lGOD") {
		imageType = "gif"
	}

	// Build Typhoon OCR request (OpenAI-compatible vision format)
	ocrPrompt := `Extract all text from the image.

Instructions:
- Only return the clean text content.
- Do not include any explanation or extra text.
- You must include all information on the page.
- Preserve the original layout and structure as much as possible.
- For tables, format them clearly with proper alignment.
- For checkboxes, use ☐ for unchecked and ☑ for checked boxes.`

	chatReq := map[string]interface{}{
		"model": "typhoon-ocr",
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "image_url",
						"image_url": map[string]string{
							"url": fmt.Sprintf("data:image/%s;base64,%s", imageType, imageBase64),
						},
					},
					{
						"type": "text",
						"text": ocrPrompt,
					},
				},
			},
		},
		"max_tokens": 4096,
	}

	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Call Typhoon OCR API
	url := "https://api.opentyphoon.ai/v1/chat/completions"
	client := &http.Client{Timeout: 90 * time.Second}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.typhoonAPIKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Typhoon OCR API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Typhoon OCR API error (status %d): %s", resp.StatusCode, string(body))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from Typhoon OCR API")
	}

	rawText := chatResp.Choices[0].Message.Content

	// Parse the extracted text
	result := s.parseExtractedText(rawText)
	return result, nil
}

// parseExtractedText parses OCR text and extracts structured data
func (s *OCRService) parseExtractedText(rawText string) *OCRResult {
	result := &OCRResult{
		RawText:       rawText,
		ExtractedData: make(map[string]string),
	}

	// Normalize Thai digits
	normalizedText := normalizeThaiDigits(rawText)
	score := 0

	// Extract Thai ID number (13 digits)
	idPattern := regexp.MustCompile(`\b\d{1}[\s-]?\d{4}[\s-]?\d{5}[\s-]?\d{2}[\s-]?\d{1}\b|\b\d{13}\b`)
	if match := idPattern.FindString(normalizedText); match != "" {
		cleanID := regexp.MustCompile(`\D`).ReplaceAllString(match, "")
		result.ExtractedData["id_number"] = cleanID
		score += 25
		if validateThaiID(cleanID) {
			result.ExtractedData["id_valid"] = "true"
			score += 10
		}
	}

	// Extract Thai name with prefix
	thaiPrefixes := []string{"นาย", "นาง", "นางสาว", "เด็กชาย", "เด็กหญิง", "ด.ช.", "ด.ญ."}
	for _, prefix := range thaiPrefixes {
		pattern := regexp.MustCompile(prefix + `\s*([ก-๏\s]+)`)
		if match := pattern.FindStringSubmatch(rawText); match != nil {
			result.ExtractedData["name_prefix_th"] = prefix
			result.ExtractedData["name_th"] = strings.TrimSpace(match[1])
			score += 20
			break
		}
	}

	// Extract English name
	engNamePattern := regexp.MustCompile(`(?i)(?:Name|ชื่อ)\s*(Mr\.|Mrs\.|Miss|Ms\.)\s*([A-Za-z]+)\s*(?:Last\s*name|นามสกุล)\s*([A-Za-z]+)`)
	if match := engNamePattern.FindStringSubmatch(rawText); match != nil {
		result.ExtractedData["name_prefix_en"] = match[1]
		result.ExtractedData["first_name_en"] = match[2]
		result.ExtractedData["last_name_en"] = match[3]
		score += 15
	} else {
		// Try simpler pattern
		simpleEngPattern := regexp.MustCompile(`(Mr\.|Mrs\.|Miss|Ms\.)\s*([A-Za-z]+)\s+([A-Za-z]+)`)
		if match := simpleEngPattern.FindStringSubmatch(rawText); match != nil {
			result.ExtractedData["name_prefix_en"] = match[1]
			result.ExtractedData["first_name_en"] = match[2]
			result.ExtractedData["last_name_en"] = match[3]
			score += 15
		}
	}

	// Extract dates (birth, issue, expiry)
	datePattern := regexp.MustCompile(`\d{1,2}\s+[ก-๏\.]+\s+\d{4}`)
	dates := datePattern.FindAllString(rawText, -1)
	dateLabels := []string{"birth_date", "issue_date", "expiry_date"}
	for i, date := range dates {
		if i >= len(dateLabels) {
			break
		}
		result.ExtractedData[dateLabels[i]+"_th"] = date
		result.ExtractedData[dateLabels[i]] = convertThaiDate(date)
		score += 5
	}

	// Extract address
	addressPattern := regexp.MustCompile(`(?:ที่อยู่|บ้านเลขที่)\s*([ก-๏0-9\s/\-\.]+(?:หมู่|ม\.|ตำบล|ต\.|อำเภอ|อ\.|จังหวัด|จ\.)[ก-๏0-9\s/\-\.]+)`)
	if match := addressPattern.FindStringSubmatch(rawText); match != nil {
		result.ExtractedData["address"] = strings.TrimSpace(match[1])
		score += 10
	}

	// Extract religion
	religionPattern := regexp.MustCompile(`ศาสนา\s*([ก-๏]+)`)
	if match := religionPattern.FindStringSubmatch(rawText); match != nil {
		result.ExtractedData["religion"] = match[1]
		score += 5
	}

	// Extract phone number
	phonePattern := regexp.MustCompile(`(?:โทร|Tel|Phone)[\s.:]*(\d{2,3}[-\s]?\d{3}[-\s]?\d{4})`)
	if match := phonePattern.FindStringSubmatch(normalizedText); match != nil {
		result.ExtractedData["phone"] = regexp.MustCompile(`\D`).ReplaceAllString(match[1], "")
		score += 5
	}

	// Extract email
	emailPattern := regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	if match := emailPattern.FindString(rawText); match != "" {
		result.ExtractedData["email"] = match
		score += 5
	}

	result.DetectionScore = score
	return result
}

// ExtractFromImageFile extracts text from an image file
func (s *OCRService) ExtractFromImageFile(imageData []byte) (*OCRResult, error) {
	base64Image := base64.StdEncoding.EncodeToString(imageData)
	return s.ExtractTextFromImage(base64Image)
}

// MapToPlaceholders maps extracted OCR data to template placeholders using Gemini AI
func (s *OCRService) MapToPlaceholders(extractedData map[string]string, placeholders []string) map[string]string {
	return s.MapToPlaceholdersWithRawText(extractedData, placeholders, "")
}

// MapToPlaceholdersWithRawText maps OCR data to placeholders, using raw text for better AI context
func (s *OCRService) MapToPlaceholdersWithRawText(extractedData map[string]string, placeholders []string, rawText string) map[string]string {
	// If Gemini API key is available, use AI-powered mapping
	if s.geminiAPIKey != "" {
		mappings, err := s.mapWithGemini(extractedData, placeholders, rawText)
		if err == nil && len(mappings) > 0 {
			return mappings
		}
		// Fall back to rule-based mapping if Gemini fails
		fmt.Printf("Gemini mapping failed, falling back to rules: %v\n", err)
	}

	// Fallback: rule-based mapping
	return s.mapWithRules(extractedData, placeholders)
}

// mapWithGemini uses Gemini AI to intelligently map OCR data to placeholders
func (s *OCRService) mapWithGemini(extractedData map[string]string, placeholders []string, ocrRawText string) (map[string]string, error) {
	if s.geminiAPIKey == "" {
		return nil, fmt.Errorf("GOOGLE_AI_API_KEY not configured")
	}

	// Use OCR raw text - this is the most important input for Gemini
	rawText := ocrRawText
	if rawText == "" {
		return nil, fmt.Errorf("no raw text provided for Gemini mapping")
	}

	fmt.Printf("[Gemini] Starting mapping with %d placeholders\n", len(placeholders))
	fmt.Printf("[Gemini] Raw text length: %d characters\n", len(rawText))

	// Clean placeholders for the prompt
	var cleanPlaceholders []string
	for _, p := range placeholders {
		clean := strings.ReplaceAll(p, "{{", "")
		clean = strings.ReplaceAll(clean, "}}", "")
		cleanPlaceholders = append(cleanPlaceholders, clean)
	}

	// Build prompt for Gemini
	prompt := fmt.Sprintf(`You are an expert at extracting data from Thai documents (like Thai ID cards, birth certificates, government forms).

I scanned a Thai document and got this OCR text:
---
%s
---

I need to fill a form with these field names:
%s

Your task:
1. Extract ALL relevant information from the OCR text
2. TRANSLATE ALL Thai text to English (romanization/transliteration)
3. Convert Thai name prefixes: นาย→Mr., นาง→Mrs., นางสาว→Miss, เด็กชาย→Master, เด็กหญิง→Miss
4. Convert Buddhist Era years to Gregorian: subtract 543 (e.g., 2567→2024)
5. Format dates as YYYY-MM-DD
6. Match extracted data to the most appropriate field names

For Thai addresses - TRANSLATE to English:
- Province (จังหวัด): e.g., กรุงเทพมหานคร→Bangkok, เชียงใหม่→Chiang Mai, ภูเก็ต→Phuket
- District (อำเภอ/เขต): transliterate to English, e.g., บางรัก→Bang Rak
- Subdistrict (ตำบล/แขวง): transliterate to English, e.g., สีลม→Si Lom
- Full address: translate/transliterate everything to English

Return ONLY a JSON object. Map field names to their values.
Example: {"first_name": "Somchai", "last_name": "Jaidee", "name_prefix": "Mr.", "id_number": "1234567890123", "dob": "1990-01-15", "province": "Bangkok", "district": "Bang Rak", "subdistrict": "Si Lom", "address": "123 Silom Road, Si Lom, Bang Rak, Bangkok"}

Important:
- Use the EXACT field names from my list
- TRANSLATE everything to English including addresses
- Return valid JSON only, no explanation`, rawText, strings.Join(cleanPlaceholders, ", "))

	// Call Gemini API
	geminiReq := GeminiRequest{
		Contents: []GeminiContent{
			{
				Parts: []GeminiPart{
					{Text: prompt},
				},
			},
		},
	}

	reqBody, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Gemini request: %w", err)
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=%s", s.geminiAPIKey)
	client := &http.Client{Timeout: 60 * time.Second}

	resp, err := client.Post(url, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to call Gemini API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Gemini response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Gemini API error: %s", string(body))
	}

	var geminiResp GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini response: %w", err)
	}

	if geminiResp.Error != nil {
		return nil, fmt.Errorf("Gemini API error: %s", geminiResp.Error.Message)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no response from Gemini")
	}

	// Parse the JSON response from Gemini
	responseText := geminiResp.Candidates[0].Content.Parts[0].Text

	// Clean up the response (remove markdown code blocks if present)
	responseText = strings.TrimSpace(responseText)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	mappings := make(map[string]string)
	if err := json.Unmarshal([]byte(responseText), &mappings); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini JSON response: %w, response: %s", err, responseText)
	}

	return mappings, nil
}

// mapWithRules uses rule-based mapping as fallback
func (s *OCRService) mapWithRules(extractedData map[string]string, placeholders []string) map[string]string {
	mappings := make(map[string]string)

	// Define mapping rules from extracted data to common placeholder patterns
	mappingRules := map[string][]string{
		"id_number":      {"id", "id_number", "_id", "เลขบัตร", "บัตรประชาชน"},
		"name_th":        {"name_th", "thai_name", "ชื่อ", "first_name_th"},
		"name_prefix_th": {"prefix", "name_prefix", "คำนำหน้า"},
		"first_name_en":  {"first_name", "firstname", "en_name"},
		"last_name_en":   {"last_name", "lastname", "surname"},
		"name_prefix_en": {"prefix_en", "name_prefix_en"},
		"birth_date":     {"dob", "birth_date", "birthday", "วันเกิด"},
		"birth_date_th":  {"dob_th", "birth_date_th"},
		"address":        {"address", "ที่อยู่", "home_address"},
		"religion":       {"religion", "ศาสนา"},
		"phone":          {"phone", "tel", "telephone", "โทร"},
		"email":          {"email", "อีเมล"},
	}

	// For each placeholder, try to find a matching extracted value
	for _, placeholder := range placeholders {
		cleanPlaceholder := strings.ToLower(strings.ReplaceAll(placeholder, "{{", ""))
		cleanPlaceholder = strings.ReplaceAll(cleanPlaceholder, "}}", "")
		cleanPlaceholder = strings.TrimSpace(cleanPlaceholder)

		for dataKey, patterns := range mappingRules {
			value, exists := extractedData[dataKey]
			if !exists || value == "" {
				continue
			}

			for _, pattern := range patterns {
				if strings.Contains(cleanPlaceholder, pattern) ||
					strings.Contains(pattern, cleanPlaceholder) ||
					cleanPlaceholder == pattern {
					mappings[cleanPlaceholder] = value
					break
				}
			}
		}
	}

	return mappings
}

// containsThai checks if string contains Thai characters
func containsThai(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Thai) {
			return true
		}
	}
	return false
}

// searchAddress searches the address API for matching addresses
func (s *OCRService) searchAddress(keyword string) ([]AddressSearchResult, error) {
	if s.addressAPIURL == "" || keyword == "" {
		return nil, fmt.Errorf("address API not configured or empty keyword")
	}

	url := fmt.Sprintf("%s/search?q=%s", s.addressAPIURL, keyword)
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to call address API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("address API returned status %d", resp.StatusCode)
	}

	var results []AddressSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("failed to parse address API response: %w", err)
	}

	return results, nil
}

// EnhanceMappingsWithAddressAPI validates and enhances address fields using the address API
func (s *OCRService) EnhanceMappingsWithAddressAPI(mappings map[string]string, placeholders []string) map[string]string {
	if s.addressAPIURL == "" {
		return mappings
	}

	// Find address-related fields in mappings
	addressKeywords := []string{}
	for key, value := range mappings {
		lowerKey := strings.ToLower(key)
		if (strings.Contains(lowerKey, "province") || strings.Contains(lowerKey, "prov") ||
			strings.Contains(lowerKey, "district") || strings.Contains(lowerKey, "amphoe") ||
			strings.Contains(lowerKey, "subdistrict") || strings.Contains(lowerKey, "tambon") ||
			strings.Contains(lowerKey, "address")) && containsThai(value) {
			addressKeywords = append(addressKeywords, value)
		}
	}

	if len(addressKeywords) == 0 {
		return mappings
	}

	// Search for the first address keyword to validate
	for _, keyword := range addressKeywords {
		results, err := s.searchAddress(keyword)
		if err != nil {
			fmt.Printf("[Address API] Search error: %v\n", err)
			continue
		}

		if len(results) == 0 {
			continue
		}

		// Use the first match to enhance address fields
		match := results[0]
		fmt.Printf("[Address API] Found match: %s, %s, %s\n", match.Name3, match.Name2, match.Name1)

		// Map the matched address to placeholders
		for _, placeholder := range placeholders {
			cleanPlaceholder := strings.ToLower(strings.ReplaceAll(placeholder, "{{", ""))
			cleanPlaceholder = strings.ReplaceAll(cleanPlaceholder, "}}", "")
			cleanPlaceholder = strings.TrimSpace(cleanPlaceholder)

			// Province fields
			if strings.Contains(cleanPlaceholder, "province") || strings.Contains(cleanPlaceholder, "_prov") {
				if strings.Contains(cleanPlaceholder, "_en") || strings.Contains(cleanPlaceholder, "eng") {
					mappings[cleanPlaceholder] = match.NameEng1
				} else {
					mappings[cleanPlaceholder] = match.Name1
				}
			}

			// District fields
			if strings.Contains(cleanPlaceholder, "district") || strings.Contains(cleanPlaceholder, "amphoe") {
				if strings.Contains(cleanPlaceholder, "_en") || strings.Contains(cleanPlaceholder, "eng") {
					mappings[cleanPlaceholder] = match.NameEng2
				} else {
					mappings[cleanPlaceholder] = match.Name2
				}
			}

			// Sub-district fields
			if strings.Contains(cleanPlaceholder, "subdistrict") || strings.Contains(cleanPlaceholder, "tambon") {
				if strings.Contains(cleanPlaceholder, "_en") || strings.Contains(cleanPlaceholder, "eng") {
					mappings[cleanPlaceholder] = match.NameEng3
				} else {
					mappings[cleanPlaceholder] = match.Name3
				}
			}
		}

		// Found a match, stop searching
		break
	}

	return mappings
}

// ============================================================================
// Typhoon OCR Integration
// ============================================================================

// TyphoonOCRParams contains parameters for Typhoon OCR API
type TyphoonOCRParams struct {
	Model             string  `json:"model"`
	TaskType          string  `json:"task_type"`
	MaxTokens         int     `json:"max_tokens"`
	Temperature       float64 `json:"temperature"`
	TopP              float64 `json:"top_p"`
	RepetitionPenalty float64 `json:"repetition_penalty"`
}

// TyphoonOCRResponse represents the response from Typhoon OCR API
type TyphoonOCRResponse struct {
	TotalPages       int `json:"total_pages"`
	SuccessfulPages  int `json:"successful_pages"`
	FailedPages      int `json:"failed_pages"`
	ProcessingTime   float64 `json:"processing_time"`
	Results          []TyphoonResultItem `json:"results"`
}

// TyphoonResultItem represents a single result item from Typhoon OCR
type TyphoonResultItem struct {
	Filename string `json:"filename"`
	Success  bool   `json:"success"`
	Message  struct {
		ID      string `json:"id"`
		Created int64  `json:"created"`
		Model   string `json:"model"`
		Object  string `json:"object"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Index        int    `json:"index"`
			Message      struct {
				Content string `json:"content"`
				Role    string `json:"role"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	} `json:"message"`
	Error    *string `json:"error"`
	FileType string  `json:"file_type"`
	FileSize int     `json:"file_size"`
	Duration float64 `json:"duration"`
}

// TyphoonExtractedData contains structured data extracted from Thai documents
type TyphoonExtractedData struct {
	// Personal Information
	IDNumber      string `json:"id_number"`
	IDValid       bool   `json:"id_valid"`
	NamePrefixTH  string `json:"name_prefix_th"`
	FirstNameTH   string `json:"first_name_th"`
	LastNameTH    string `json:"last_name_th"`
	FullNameTH    string `json:"full_name_th"`
	NamePrefixEN  string `json:"name_prefix_en"`
	FirstNameEN   string `json:"first_name_en"`
	LastNameEN    string `json:"last_name_en"`
	FullNameEN    string `json:"full_name_en"`

	// Dates
	BirthDate      string `json:"birth_date"`
	BirthDateTH    string `json:"birth_date_th"`
	IssueDate      string `json:"issue_date"`
	IssueDateTH    string `json:"issue_date_th"`
	ExpiryDate     string `json:"expiry_date"`
	ExpiryDateTH   string `json:"expiry_date_th"`

	// Address
	Address        string `json:"address"`
	HouseNo        string `json:"house_no"`
	Moo            string `json:"moo"`
	Soi            string `json:"soi"`
	Road           string `json:"road"`
	Subdistrict    string `json:"subdistrict"`
	SubdistrictEN  string `json:"subdistrict_en"`
	District       string `json:"district"`
	DistrictEN     string `json:"district_en"`
	Province       string `json:"province"`
	ProvinceEN     string `json:"province_en"`
	PostalCode     string `json:"postal_code"`

	// Additional Information
	Religion       string `json:"religion"`
	Nationality    string `json:"nationality"`
	Gender         string `json:"gender"`
	BloodType      string `json:"blood_type"`
	Phone          string `json:"phone"`
	Email          string `json:"email"`

	// Document specific
	DocumentType   string `json:"document_type"`
	DocumentNumber string `json:"document_number"`

	// Raw data for unmapped fields
	RawFields      map[string]string `json:"raw_fields,omitempty"`
}

// TyphoonOCRResult combines raw text with structured extraction
type TyphoonOCRResult struct {
	RawText        string                `json:"raw_text"`
	ExtractedData  *TyphoonExtractedData `json:"extracted_data"`
	MappedFields   map[string]string     `json:"mapped_fields,omitempty"`
	DetectionScore int                   `json:"detection_score"`
	Provider       string                `json:"provider"` // "typhoon" or "vision"
}

// DefaultTyphoonParams returns default parameters for Typhoon OCR
func DefaultTyphoonParams() TyphoonOCRParams {
	return TyphoonOCRParams{
		Model:             "typhoon-ocr",
		TaskType:          "default",
		MaxTokens:         16384,
		Temperature:       0.1,
		TopP:              0.6,
		RepetitionPenalty: 1.2,
	}
}

// ExtractWithTyphoon calls Typhoon OCR API to extract text from image
func (s *OCRService) ExtractWithTyphoon(imageData []byte, params *TyphoonOCRParams) (*TyphoonOCRResult, error) {
	if s.typhoonAPIKey == "" {
		return nil, fmt.Errorf("TYPHOON_API_KEY not configured")
	}

	apiURL := s.typhoonAPIURL
	if apiURL == "" {
		apiURL = "https://api.opentyphoon.ai/v1/ocr"
	}

	// Use default params if not provided
	if params == nil {
		defaultParams := DefaultTyphoonParams()
		params = &defaultParams
	}

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add image file
	part, err := writer.CreateFormFile("file", "image.jpg")
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(imageData); err != nil {
		return nil, fmt.Errorf("failed to write image data: %w", err)
	}

	// Add parameters
	writer.WriteField("model", params.Model)
	writer.WriteField("task_type", params.TaskType)
	writer.WriteField("max_tokens", strconv.Itoa(params.MaxTokens))
	writer.WriteField("temperature", strconv.FormatFloat(params.Temperature, 'f', -1, 64))
	writer.WriteField("top_p", strconv.FormatFloat(params.TopP, 'f', -1, 64))
	writer.WriteField("repetition_penalty", strconv.FormatFloat(params.RepetitionPenalty, 'f', -1, 64))

	writer.Close()

	// Create request
	req, err := http.NewRequest("POST", apiURL, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+s.typhoonAPIKey)

	// Execute request
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Typhoon API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Typhoon API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Debug: log raw response
	fmt.Printf("[Typhoon] Raw API response: %s\n", string(body))

	// Parse response
	var typhoonResp TyphoonOCRResponse
	if err := json.Unmarshal(body, &typhoonResp); err != nil {
		return nil, fmt.Errorf("failed to parse Typhoon response: %w, body: %s", err, string(body))
	}

	// Debug: log parsed results
	fmt.Printf("[Typhoon] Parsed %d results, successful: %d\n", len(typhoonResp.Results), typhoonResp.SuccessfulPages)

	// Combine all page results - extract from nested message.choices[].message.content
	var rawText strings.Builder
	for i, result := range typhoonResp.Results {
		if !result.Success {
			fmt.Printf("[Typhoon] Result %d failed: %v\n", i, result.Error)
			continue
		}

		// Extract content from choices
		for _, choice := range result.Message.Choices {
			content := choice.Message.Content
			fmt.Printf("[Typhoon] Result %d, choice %d content length: %d\n", i, choice.Index, len(content))
			if content != "" {
				rawText.WriteString(content)
				rawText.WriteString("\n")
			}
		}
	}

	fmt.Printf("[Typhoon] Total extracted text length: %d\n", rawText.Len())

	// Parse and structure the extracted text
	extractedData := s.parseTyphoonText(rawText.String())

	// Calculate detection score
	score := s.calculateDetectionScore(extractedData)

	return &TyphoonOCRResult{
		RawText:        rawText.String(),
		ExtractedData:  extractedData,
		DetectionScore: score,
		Provider:       "typhoon",
	}, nil
}

// ExtractWithTyphoonBase64 extracts text from base64 encoded image using Typhoon OCR
func (s *OCRService) ExtractWithTyphoonBase64(imageBase64 string, params *TyphoonOCRParams) (*TyphoonOCRResult, error) {
	// Remove data URL prefix if present
	if strings.HasPrefix(imageBase64, "data:image") {
		parts := strings.SplitN(imageBase64, ",", 2)
		if len(parts) == 2 {
			imageBase64 = parts[1]
		}
	}

	imageData, err := base64.StdEncoding.DecodeString(imageBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 image: %w", err)
	}

	return s.ExtractWithTyphoon(imageData, params)
}

// parseTyphoonText parses OCR text from Typhoon and extracts structured data
func (s *OCRService) parseTyphoonText(rawText string) *TyphoonExtractedData {
	data := &TyphoonExtractedData{
		RawFields: make(map[string]string),
	}

	// Normalize Thai digits
	normalizedText := normalizeThaiDigits(rawText)

	// Extract Thai ID number (13 digits)
	idPattern := regexp.MustCompile(`\b(\d{1}[\s-]?\d{4}[\s-]?\d{5}[\s-]?\d{2}[\s-]?\d{1}|\d{13})\b`)
	if match := idPattern.FindString(normalizedText); match != "" {
		cleanID := regexp.MustCompile(`\D`).ReplaceAllString(match, "")
		data.IDNumber = cleanID
		data.IDValid = validateThaiID(cleanID)
	}

	// Extract Thai name with prefix
	thaiPrefixPatterns := []struct {
		prefix   string
		prefixEN string
	}{
		{"นาย", "Mr."},
		{"นาง", "Mrs."},
		{"นางสาว", "Miss"},
		{"เด็กชาย", "Master"},
		{"เด็กหญิง", "Miss"},
		{"ด.ช.", "Master"},
		{"ด.ญ.", "Miss"},
	}

	for _, p := range thaiPrefixPatterns {
		pattern := regexp.MustCompile(p.prefix + `\s*([ก-๏]+)\s+([ก-๏]+)`)
		if match := pattern.FindStringSubmatch(rawText); match != nil {
			data.NamePrefixTH = p.prefix
			data.NamePrefixEN = p.prefixEN
			data.FirstNameTH = strings.TrimSpace(match[1])
			data.LastNameTH = strings.TrimSpace(match[2])
			data.FullNameTH = p.prefix + " " + data.FirstNameTH + " " + data.LastNameTH
			break
		}
	}

	// Extract English name
	engNamePatterns := []struct {
		pattern *regexp.Regexp
	}{
		{regexp.MustCompile(`(?i)(Mr\.|Mrs\.|Miss|Ms\.)\s*([A-Za-z]+)\s+([A-Za-z]+)`)},
		{regexp.MustCompile(`(?i)Name\s*:?\s*(Mr\.|Mrs\.|Miss|Ms\.)?\s*([A-Za-z]+)\s+([A-Za-z]+)`)},
		{regexp.MustCompile(`(?i)([A-Z][a-z]+)\s+([A-Z][a-z]+)`)}, // Simple capitalized names
	}

	for _, p := range engNamePatterns {
		if match := p.pattern.FindStringSubmatch(rawText); match != nil {
			if len(match) == 4 {
				data.NamePrefixEN = match[1]
				data.FirstNameEN = match[2]
				data.LastNameEN = match[3]
			} else if len(match) == 3 {
				data.FirstNameEN = match[1]
				data.LastNameEN = match[2]
			}
			if data.FirstNameEN != "" && data.LastNameEN != "" {
				prefix := data.NamePrefixEN
				if prefix == "" {
					prefix = ""
				} else {
					prefix = prefix + " "
				}
				data.FullNameEN = strings.TrimSpace(prefix + data.FirstNameEN + " " + data.LastNameEN)
				break
			}
		}
	}

	// Extract dates
	datePattern := regexp.MustCompile(`(\d{1,2})\s+([ก-๏\.]+)\s+(\d{4})`)
	dates := datePattern.FindAllStringSubmatch(rawText, -1)
	dateLabels := []string{"birth", "issue", "expiry"}

	for i, match := range dates {
		if i >= len(dateLabels) {
			break
		}
		thaiDate := match[0]
		gregorianDate := convertThaiDate(thaiDate)

		switch dateLabels[i] {
		case "birth":
			data.BirthDateTH = thaiDate
			data.BirthDate = gregorianDate
		case "issue":
			data.IssueDateTH = thaiDate
			data.IssueDate = gregorianDate
		case "expiry":
			data.ExpiryDateTH = thaiDate
			data.ExpiryDate = gregorianDate
		}
	}

	// Extract address components
	s.extractAddressComponents(rawText, data)

	// Extract phone number
	phonePattern := regexp.MustCompile(`(?:โทร|Tel|Phone|เบอร์)[\s.:]*(\d{2,3}[-\s]?\d{3}[-\s]?\d{4})`)
	if match := phonePattern.FindStringSubmatch(normalizedText); match != nil {
		data.Phone = regexp.MustCompile(`\D`).ReplaceAllString(match[1], "")
	}

	// Extract email
	emailPattern := regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	if match := emailPattern.FindString(rawText); match != "" {
		data.Email = match
	}

	// Extract religion
	religionPattern := regexp.MustCompile(`ศาสนา\s*([ก-๏]+)`)
	if match := religionPattern.FindStringSubmatch(rawText); match != nil {
		data.Religion = match[1]
	}

	// Detect document type
	data.DocumentType = s.detectDocumentType(rawText)

	return data
}

// extractAddressComponents extracts address parts from Thai text
func (s *OCRService) extractAddressComponents(text string, data *TyphoonExtractedData) {
	// House number
	housePattern := regexp.MustCompile(`(?:บ้านเลขที่|เลขที่)\s*(\d+[/\d]*)`)
	if match := housePattern.FindStringSubmatch(text); match != nil {
		data.HouseNo = match[1]
	}

	// Moo (village number)
	mooPattern := regexp.MustCompile(`(?:หมู่|ม\.)\s*(\d+)`)
	if match := mooPattern.FindStringSubmatch(text); match != nil {
		data.Moo = match[1]
	}

	// Soi
	soiPattern := regexp.MustCompile(`(?:ซอย|ซ\.)\s*([ก-๏\w\d]+)`)
	if match := soiPattern.FindStringSubmatch(text); match != nil {
		data.Soi = match[1]
	}

	// Road
	roadPattern := regexp.MustCompile(`(?:ถนน|ถ\.)\s*([ก-๏]+)`)
	if match := roadPattern.FindStringSubmatch(text); match != nil {
		data.Road = match[1]
	}

	// Subdistrict
	subdistrictPattern := regexp.MustCompile(`(?:ตำบล|ต\.|แขวง)\s*([ก-๏]+)`)
	if match := subdistrictPattern.FindStringSubmatch(text); match != nil {
		data.Subdistrict = match[1]
	}

	// District
	districtPattern := regexp.MustCompile(`(?:อำเภอ|อ\.|เขต)\s*([ก-๏]+)`)
	if match := districtPattern.FindStringSubmatch(text); match != nil {
		data.District = match[1]
	}

	// Province
	provincePattern := regexp.MustCompile(`(?:จังหวัด|จ\.)\s*([ก-๏]+)`)
	if match := provincePattern.FindStringSubmatch(text); match != nil {
		data.Province = match[1]
	}

	// Postal code
	postalPattern := regexp.MustCompile(`\b(\d{5})\b`)
	if match := postalPattern.FindStringSubmatch(text); match != nil {
		data.PostalCode = match[1]
	}

	// Build full address
	var addressParts []string
	if data.HouseNo != "" {
		addressParts = append(addressParts, "บ้านเลขที่ "+data.HouseNo)
	}
	if data.Moo != "" {
		addressParts = append(addressParts, "หมู่ "+data.Moo)
	}
	if data.Soi != "" {
		addressParts = append(addressParts, "ซอย "+data.Soi)
	}
	if data.Road != "" {
		addressParts = append(addressParts, "ถนน "+data.Road)
	}
	if data.Subdistrict != "" {
		addressParts = append(addressParts, "ตำบล "+data.Subdistrict)
	}
	if data.District != "" {
		addressParts = append(addressParts, "อำเภอ "+data.District)
	}
	if data.Province != "" {
		addressParts = append(addressParts, "จังหวัด "+data.Province)
	}
	if data.PostalCode != "" {
		addressParts = append(addressParts, data.PostalCode)
	}

	if len(addressParts) > 0 {
		data.Address = strings.Join(addressParts, " ")
	}
}

// detectDocumentType detects the type of Thai document from OCR text
func (s *OCRService) detectDocumentType(text string) string {
	text = strings.ToLower(text)

	if strings.Contains(text, "บัตรประจำตัวประชาชน") || strings.Contains(text, "identification card") {
		return "thai_id_card"
	}
	if strings.Contains(text, "หนังสือเดินทาง") || strings.Contains(text, "passport") {
		return "passport"
	}
	if strings.Contains(text, "สูติบัตร") || strings.Contains(text, "birth certificate") {
		return "birth_certificate"
	}
	if strings.Contains(text, "ทะเบียนบ้าน") || strings.Contains(text, "house registration") {
		return "house_registration"
	}
	if strings.Contains(text, "ใบขับขี่") || strings.Contains(text, "driver") {
		return "driver_license"
	}
	if strings.Contains(text, "ใบเสร็จ") || strings.Contains(text, "receipt") {
		return "receipt"
	}
	if strings.Contains(text, "ใบกำกับภาษี") || strings.Contains(text, "invoice") || strings.Contains(text, "tax invoice") {
		return "invoice"
	}

	return "unknown"
}

// calculateDetectionScore calculates confidence score based on extracted data
func (s *OCRService) calculateDetectionScore(data *TyphoonExtractedData) int {
	score := 0

	if data.IDNumber != "" {
		score += 25
		if data.IDValid {
			score += 10
		}
	}
	if data.FirstNameTH != "" || data.FirstNameEN != "" {
		score += 15
	}
	if data.LastNameTH != "" || data.LastNameEN != "" {
		score += 10
	}
	if data.BirthDate != "" {
		score += 10
	}
	if data.Address != "" || data.Province != "" {
		score += 10
	}
	if data.Phone != "" {
		score += 5
	}
	if data.Email != "" {
		score += 5
	}
	if data.DocumentType != "unknown" {
		score += 10
	}

	return score
}

// ============================================================================
// Form Field Mapping
// ============================================================================

// FormFieldMapping maps extracted OCR data to form input fields
type FormFieldMapping struct {
	FieldName    string `json:"field_name"`    // Form field name/placeholder
	Value        string `json:"value"`         // Extracted value
	Confidence   int    `json:"confidence"`    // Confidence score (0-100)
	Source       string `json:"source"`        // Source field from OCR extraction
	InputType    string `json:"input_type"`    // Suggested input type (text, select, date, etc.)
	NeedsReview  bool   `json:"needs_review"`  // Whether the field needs human review
}

// MapTyphoonToFormFields maps Typhoon OCR extracted data to template form fields
func (s *OCRService) MapTyphoonToFormFields(result *TyphoonOCRResult, placeholders []string) map[string]FormFieldMapping {
	mappings := make(map[string]FormFieldMapping)

	if result == nil || result.ExtractedData == nil {
		return mappings
	}

	data := result.ExtractedData

	// Build a lookup map from extracted data
	extractedMap := map[string]struct {
		value      string
		confidence int
		inputType  string
	}{
		// ID fields
		"id_number":      {data.IDNumber, s.boolToConfidence(data.IDValid, 95, 70), "text"},
		"id":             {data.IDNumber, s.boolToConfidence(data.IDValid, 95, 70), "text"},

		// Thai name fields
		"name_prefix":    {data.NamePrefixTH, 90, "select"},
		"name_prefix_th": {data.NamePrefixTH, 90, "select"},
		"first_name":     {data.FirstNameTH, 85, "text"},
		"first_name_th":  {data.FirstNameTH, 85, "text"},
		"last_name":      {data.LastNameTH, 85, "text"},
		"last_name_th":   {data.LastNameTH, 85, "text"},
		"full_name":      {data.FullNameTH, 85, "text"},
		"full_name_th":   {data.FullNameTH, 85, "text"},
		"name_th":        {data.FullNameTH, 85, "text"},

		// English name fields
		"name_prefix_en": {data.NamePrefixEN, 90, "select"},
		"first_name_en":  {data.FirstNameEN, 80, "text"},
		"last_name_en":   {data.LastNameEN, 80, "text"},
		"full_name_en":   {data.FullNameEN, 80, "text"},
		"name_en":        {data.FullNameEN, 80, "text"},

		// Date fields
		"dob":            {data.BirthDate, 85, "date"},
		"birth_date":     {data.BirthDate, 85, "date"},
		"date_of_birth":  {data.BirthDate, 85, "date"},
		"birth_date_th":  {data.BirthDateTH, 85, "text"},
		"issue_date":     {data.IssueDate, 80, "date"},
		"issue_date_th":  {data.IssueDateTH, 80, "text"},
		"expiry_date":    {data.ExpiryDate, 80, "date"},
		"expiry_date_th": {data.ExpiryDateTH, 80, "text"},

		// Address fields
		"address":        {data.Address, 75, "textarea"},
		"house_no":       {data.HouseNo, 80, "text"},
		"house_number":   {data.HouseNo, 80, "text"},
		"moo":            {data.Moo, 80, "text"},
		"village":        {data.Moo, 80, "text"},
		"soi":            {data.Soi, 75, "text"},
		"road":           {data.Road, 75, "text"},
		"subdistrict":    {data.Subdistrict, 80, "text"},
		"tambon":         {data.Subdistrict, 80, "text"},
		"subdistrict_en": {data.SubdistrictEN, 75, "text"},
		"district":       {data.District, 80, "text"},
		"amphoe":         {data.District, 80, "text"},
		"district_en":    {data.DistrictEN, 75, "text"},
		"province":       {data.Province, 85, "select"},
		"province_th":    {data.Province, 85, "select"},
		"province_en":    {data.ProvinceEN, 80, "text"},
		"postal_code":    {data.PostalCode, 90, "text"},
		"zip_code":       {data.PostalCode, 90, "text"},

		// Other fields
		"religion":       {data.Religion, 80, "text"},
		"nationality":    {data.Nationality, 80, "text"},
		"gender":         {data.Gender, 85, "select"},
		"phone":          {data.Phone, 85, "text"},
		"telephone":      {data.Phone, 85, "text"},
		"tel":            {data.Phone, 85, "text"},
		"email":          {data.Email, 90, "text"},
	}

	// Match placeholders to extracted data
	for _, placeholder := range placeholders {
		cleanPlaceholder := strings.ReplaceAll(placeholder, "{{", "")
		cleanPlaceholder = strings.ReplaceAll(cleanPlaceholder, "}}", "")
		cleanPlaceholder = strings.TrimSpace(cleanPlaceholder)
		lowerPlaceholder := strings.ToLower(cleanPlaceholder)

		// Remove entity prefixes for matching (m_, f_, b_, r_, etc.)
		matchKey := lowerPlaceholder
		for _, prefix := range []string{"m_", "f_", "b_", "r_", "c_"} {
			if strings.HasPrefix(lowerPlaceholder, prefix) {
				matchKey = strings.TrimPrefix(lowerPlaceholder, prefix)
				break
			}
		}

		// Direct match
		if extracted, ok := extractedMap[matchKey]; ok && extracted.value != "" {
			mappings[cleanPlaceholder] = FormFieldMapping{
				FieldName:   cleanPlaceholder,
				Value:       extracted.value,
				Confidence:  extracted.confidence,
				Source:      matchKey,
				InputType:   extracted.inputType,
				NeedsReview: extracted.confidence < 80,
			}
			continue
		}

		// Fuzzy match - check if placeholder contains any key
		for key, extracted := range extractedMap {
			if extracted.value == "" {
				continue
			}
			if strings.Contains(matchKey, key) || strings.Contains(key, matchKey) {
				mappings[cleanPlaceholder] = FormFieldMapping{
					FieldName:   cleanPlaceholder,
					Value:       extracted.value,
					Confidence:  extracted.confidence - 10, // Lower confidence for fuzzy match
					Source:      key,
					InputType:   extracted.inputType,
					NeedsReview: true,
				}
				break
			}
		}
	}

	return mappings
}

// MapTyphoonToPlaceholders converts form field mappings to simple placeholder->value map
func (s *OCRService) MapTyphoonToPlaceholders(result *TyphoonOCRResult, placeholders []string) map[string]string {
	fieldMappings := s.MapTyphoonToFormFields(result, placeholders)

	mappings := make(map[string]string)
	for placeholder, fieldMapping := range fieldMappings {
		if fieldMapping.Value != "" {
			mappings[placeholder] = fieldMapping.Value
		}
	}

	return mappings
}

// boolToConfidence converts a boolean validation result to confidence score
func (s *OCRService) boolToConfidence(valid bool, highScore, lowScore int) int {
	if valid {
		return highScore
	}
	return lowScore
}

// ExtractWithTyphoonAndMap performs OCR and maps results to form fields in one call
func (s *OCRService) ExtractWithTyphoonAndMap(imageData []byte, placeholders []string, params *TyphoonOCRParams) (*TyphoonOCRResult, error) {
	result, err := s.ExtractWithTyphoon(imageData, params)
	if err != nil {
		return nil, err
	}

	// Map to placeholders
	result.MappedFields = s.MapTyphoonToPlaceholders(result, placeholders)

	// Enhance with Gemini AI if available
	if s.geminiAPIKey != "" && len(placeholders) > 0 {
		aiMappings, err := s.mapWithGemini(nil, placeholders, result.RawText)
		if err == nil {
			// Merge AI mappings with rule-based mappings (AI takes precedence)
			for key, value := range aiMappings {
				if value != "" {
					result.MappedFields[key] = value
				}
			}
		}
	}

	// Enhance with address API if available
	if s.addressAPIURL != "" {
		result.MappedFields = s.EnhanceMappingsWithAddressAPI(result.MappedFields, placeholders)
	}

	return result, nil
}

// mapWithTyphoonChat uses Typhoon's chat API for intelligent field mapping (replaces Gemini)
func (s *OCRService) mapWithTyphoonChat(ocrRawText string, placeholders []string) (map[string]string, error) {
	if s.typhoonAPIKey == "" {
		return nil, fmt.Errorf("TYPHOON_API_KEY not configured")
	}

	if ocrRawText == "" {
		return nil, fmt.Errorf("no raw text provided for mapping")
	}

	fmt.Printf("[Typhoon Chat] Starting mapping with %d placeholders\n", len(placeholders))
	fmt.Printf("[Typhoon Chat] Raw text length: %d characters\n", len(ocrRawText))

	// Clean placeholders for the prompt
	var cleanPlaceholders []string
	for _, p := range placeholders {
		clean := strings.ReplaceAll(p, "{{", "")
		clean = strings.ReplaceAll(clean, "}}", "")
		cleanPlaceholders = append(cleanPlaceholders, clean)
	}

	// Build prompt for Typhoon Chat
	prompt := fmt.Sprintf(`You are an expert at extracting data from Thai documents (like Thai ID cards, birth certificates, government forms).

I scanned a Thai document and got this OCR text:
---
%s
---

I need to fill a form with these field names:
%s

Your task:
1. Extract ALL relevant information from the OCR text
2. TRANSLATE ALL Thai text to English (romanization/transliteration)
3. Convert Thai name prefixes: นาย→Mr., นาง→Mrs., นางสาว→Miss, เด็กชาย→Master, เด็กหญิง→Miss
4. Convert Buddhist Era years to Gregorian: subtract 543 (e.g., 2567→2024)
5. Format dates as YYYY-MM-DD
6. Match extracted data to the most appropriate field names

For Thai addresses - TRANSLATE to English:
- Province (จังหวัด): e.g., กรุงเทพมหานคร→Bangkok, เชียงใหม่→Chiang Mai
- District (อำเภอ/เขต): transliterate to English
- Subdistrict (ตำบล/แขวง): transliterate to English

Return ONLY a JSON object. Map field names to their values.
Example: {"first_name": "Somchai", "last_name": "Jaidee", "name_prefix": "Mr.", "id_number": "1234567890123", "dob": "1990-01-15"}

Important:
- Use the EXACT field names from my list
- TRANSLATE everything to English
- Return valid JSON only, no explanation`, ocrRawText, strings.Join(cleanPlaceholders, ", "))

	// Build Typhoon Chat API request
	// Available models: typhoon-v2.1-12b-instruct, typhoon-v2.5-30b-a3b-instruct
	chatReq := map[string]interface{}{
		"model": "typhoon-v2.1-12b-instruct",
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"max_tokens":   2048,
		"temperature":  0.1,
		"top_p":        0.9,
	}

	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Typhoon Chat request: %w", err)
	}

	// Call Typhoon Chat API
	url := "https://api.opentyphoon.ai/v1/chat/completions"
	client := &http.Client{Timeout: 60 * time.Second}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.typhoonAPIKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Typhoon Chat API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Typhoon Chat response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Typhoon Chat API error: %s", string(body))
	}

	// Parse response (OpenAI-compatible format)
	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to parse Typhoon Chat response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from Typhoon Chat")
	}

	// Parse the JSON response
	responseText := chatResp.Choices[0].Message.Content
	responseText = strings.TrimSpace(responseText)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	fmt.Printf("[Typhoon Chat] Response: %s\n", responseText)

	mappings := make(map[string]string)
	if err := json.Unmarshal([]byte(responseText), &mappings); err != nil {
		return nil, fmt.Errorf("failed to parse Typhoon Chat JSON response: %w, response: %s", err, responseText)
	}

	fmt.Printf("[Typhoon Chat] Extracted %d field mappings\n", len(mappings))
	return mappings, nil
}

// ExtractWithTyphoonVision uses Typhoon Vision model to do OCR + structured extraction in ONE call
// This is more cost-effective than separate OCR + Chat calls
func (s *OCRService) ExtractWithTyphoonVision(imageBase64 string, placeholders []string) (*TyphoonOCRResult, map[string]FormFieldMapping, error) {
	if s.typhoonAPIKey == "" {
		return nil, nil, fmt.Errorf("TYPHOON_API_KEY not configured")
	}

	fmt.Printf("[Typhoon Vision] Single-call extraction with %d placeholders\n", len(placeholders))

	// Clean placeholders
	var cleanPlaceholders []string
	for _, p := range placeholders {
		clean := strings.ReplaceAll(p, "{{", "")
		clean = strings.ReplaceAll(clean, "}}", "")
		cleanPlaceholders = append(cleanPlaceholders, clean)
	}

	// Build prompt for structured extraction
	prompt := fmt.Sprintf(`Extract ALL text from this Thai document image and map to these form fields: %s

Instructions:
1. Extract all visible text from the document
2. Identify document type (thai_id_card, passport, birth_certificate, etc.)
3. Map extracted data to the field names provided
4. TRANSLATE all Thai text to English
5. Convert Thai name prefixes: นาย→Mr., นาง→Mrs., นางสาว→Miss
6. Convert Buddhist Era years to Gregorian (subtract 543)
7. Format dates as YYYY-MM-DD

Return JSON format:
{
  "raw_text": "all extracted text here",
  "document_type": "thai_id_card",
  "mapped_fields": {
    "field_name": "extracted_value_in_english",
    ...
  }
}

IMPORTANT: Return ONLY valid JSON, no explanation.`, strings.Join(cleanPlaceholders, ", "))

	// Build request with image
	imageData := imageBase64
	if strings.HasPrefix(imageData, "data:image") {
		parts := strings.SplitN(imageData, ",", 2)
		if len(parts) == 2 {
			imageData = parts[1]
		}
	}

	// Use Typhoon Vision API (chat completions with image)
	chatReq := map[string]interface{}{
		"model": "typhoon-v2-72b-vision-instruct",
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": prompt,
					},
					{
						"type": "image_url",
						"image_url": map[string]string{
							"url": "data:image/jpeg;base64," + imageData,
						},
					},
				},
			},
		},
		"max_tokens":  4096,
		"temperature": 0.1,
	}

	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Call Typhoon Vision API
	url := "https://api.opentyphoon.ai/v1/chat/completions"
	client := &http.Client{Timeout: 120 * time.Second}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.typhoonAPIKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to call Typhoon Vision API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response: %w", err)
	}

	fmt.Printf("[Typhoon Vision] Response status: %d\n", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("Typhoon Vision API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, nil, fmt.Errorf("no response from Typhoon Vision")
	}

	// Parse the JSON response
	responseText := chatResp.Choices[0].Message.Content
	responseText = strings.TrimSpace(responseText)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	fmt.Printf("[Typhoon Vision] Response: %s\n", responseText)

	// Parse structured response
	var structuredResp struct {
		RawText      string            `json:"raw_text"`
		DocumentType string            `json:"document_type"`
		MappedFields map[string]string `json:"mapped_fields"`
	}

	if err := json.Unmarshal([]byte(responseText), &structuredResp); err != nil {
		// If JSON parsing fails, try to extract what we can
		fmt.Printf("[Typhoon Vision] JSON parse failed, using raw response: %v\n", err)
		structuredResp.RawText = responseText
		structuredResp.MappedFields = make(map[string]string)
	}

	// Build result
	result := &TyphoonOCRResult{
		RawText:        structuredResp.RawText,
		ExtractedData:  s.parseTyphoonText(structuredResp.RawText),
		MappedFields:   structuredResp.MappedFields,
		DetectionScore: 85,
		Provider:       "typhoon-vision",
	}

	if structuredResp.DocumentType != "" {
		result.ExtractedData.DocumentType = structuredResp.DocumentType
	}

	// Build field mappings with confidence
	fieldMappings := make(map[string]FormFieldMapping)
	for key, value := range structuredResp.MappedFields {
		if value != "" {
			fieldMappings[key] = FormFieldMapping{
				FieldName:   key,
				Value:       value,
				Confidence:  85,
				Source:      "typhoon-vision",
				InputType:   "text",
				NeedsReview: false,
			}
		}
	}

	fmt.Printf("[Typhoon Vision] Extracted %d field mappings in single call\n", len(fieldMappings))

	return result, fieldMappings, nil
}

// ExtractAndMapToForm is a convenience method that combines extraction and form mapping
// Uses single Typhoon Vision call for cost efficiency
func (s *OCRService) ExtractAndMapToForm(imageBase64 string, placeholders []string) (*TyphoonOCRResult, map[string]FormFieldMapping, error) {
	// Typhoon is required
	if s.typhoonAPIKey == "" {
		return nil, nil, fmt.Errorf("TYPHOON_API_KEY not configured - Typhoon is required")
	}

	// Try single-call Vision extraction first (more cost-effective)
	if len(placeholders) > 0 {
		result, fieldMappings, err := s.ExtractWithTyphoonVision(imageBase64, placeholders)
		if err == nil && len(result.MappedFields) > 0 {
			// Enhance with address API if available
			if s.addressAPIURL != "" {
				result.MappedFields = s.EnhanceMappingsWithAddressAPI(result.MappedFields, placeholders)
			}
			return result, fieldMappings, nil
		}
		fmt.Printf("[Typhoon] Vision extraction failed or empty, falling back to OCR + Chat: %v\n", err)
	}

	// Fallback: Extract with Typhoon OCR
	result, err := s.ExtractWithTyphoonBase64(imageBase64, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("Typhoon OCR failed: %w", err)
	}

	// Map to form fields using rule-based matching
	fieldMappings := s.MapTyphoonToFormFields(result, placeholders)
	result.MappedFields = s.MapTyphoonToPlaceholders(result, placeholders)

	// Enhance with Typhoon Chat for intelligent field mapping
	if result.RawText != "" && len(placeholders) > 0 {
		aiMappings, err := s.mapWithTyphoonChat(result.RawText, placeholders)
		if err != nil {
			fmt.Printf("[Typhoon Chat] Mapping failed: %v\n", err)
		} else {
			for key, value := range aiMappings {
				if value != "" {
					result.MappedFields[key] = value
					if fm, ok := fieldMappings[key]; ok {
						fm.Value = value
						fm.Source = "typhoon-ai"
						fieldMappings[key] = fm
					} else {
						fieldMappings[key] = FormFieldMapping{
							FieldName:   key,
							Value:       value,
							Confidence:  85,
							Source:      "typhoon-ai",
							InputType:   "text",
							NeedsReview: false,
						}
					}
				}
			}
		}
	}

	// Enhance with address API if available
	if s.addressAPIURL != "" {
		result.MappedFields = s.EnhanceMappingsWithAddressAPI(result.MappedFields, placeholders)
	}

	return result, fieldMappings, nil
}

// ============================================================================
// Placeholder Alias Inference using Typhoon
// ============================================================================

// PlaceholderContext holds a placeholder with its surrounding text context
type PlaceholderContext struct {
	Placeholder   string `json:"placeholder"`    // e.g., "{{participant_name}}"
	CleanName     string `json:"clean_name"`     // e.g., "participant_name"
	ContextBefore string `json:"context_before"` // Text before placeholder
	ContextAfter  string `json:"context_after"`  // Text after placeholder
	FullContext   string `json:"full_context"`   // Combined context
}

// AliasSuggestion represents an AI-suggested alias for a placeholder
type AliasSuggestion struct {
	Placeholder    string  `json:"placeholder"`     // Original placeholder
	SuggestedAlias string  `json:"suggested_alias"` // AI-suggested Thai alias
	Confidence     float64 `json:"confidence"`      // Confidence score 0-1
	Reasoning      string  `json:"reasoning"`       // Why this alias was suggested
}

// AliasSuggestionResult contains all alias suggestions for a template
type AliasSuggestionResult struct {
	Suggestions []AliasSuggestion `json:"suggestions"`
	Model       string            `json:"model"`
	Provider    string            `json:"provider"`
}

// ExtractPlaceholdersWithContext extracts placeholders with surrounding text context
// contextChars specifies how many characters to capture before and after each placeholder
func ExtractPlaceholdersWithContext(content string, contextChars int) []PlaceholderContext {
	var results []PlaceholderContext
	seen := make(map[string]bool)

	// Remove XML/HTML tags for cleaner context
	cleanContent := removeHTMLTags(content)

	pos := 0
	for {
		startIdx := strings.Index(cleanContent[pos:], "{{")
		if startIdx == -1 {
			break
		}
		startIdx += pos

		endIdx := strings.Index(cleanContent[startIdx:], "}}")
		if endIdx == -1 {
			break
		}
		endIdx += startIdx + 2

		placeholder := cleanContent[startIdx:endIdx]

		// Skip if already seen
		if seen[placeholder] {
			pos = endIdx
			continue
		}
		seen[placeholder] = true

		// Extract context before
		contextStart := startIdx - contextChars
		if contextStart < 0 {
			contextStart = 0
		}
		contextBefore := strings.TrimSpace(cleanContent[contextStart:startIdx])

		// Extract context after
		contextEnd := endIdx + contextChars
		if contextEnd > len(cleanContent) {
			contextEnd = len(cleanContent)
		}
		contextAfter := strings.TrimSpace(cleanContent[endIdx:contextEnd])

		// Clean placeholder name
		cleanName := strings.TrimPrefix(placeholder, "{{")
		cleanName = strings.TrimSuffix(cleanName, "}}")
		cleanName = strings.TrimSpace(cleanName)

		results = append(results, PlaceholderContext{
			Placeholder:   placeholder,
			CleanName:     cleanName,
			ContextBefore: contextBefore,
			ContextAfter:  contextAfter,
			FullContext:   fmt.Sprintf("%s [%s] %s", contextBefore, placeholder, contextAfter),
		})

		pos = endIdx
	}

	return results
}

// removeHTMLTags removes HTML/XML tags from content
func removeHTMLTags(content string) string {
	var result strings.Builder
	inTag := false

	for _, char := range content {
		if char == '<' {
			inTag = true
		} else if char == '>' {
			inTag = false
			result.WriteRune(' ') // Replace tag with space
		} else if !inTag {
			result.WriteRune(char)
		}
	}

	// Clean up multiple spaces
	text := result.String()
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}

	return text
}

// SuggestAliases uses Typhoon AI to infer meaningful aliases for placeholders
// based on their surrounding context in the document
func (s *OCRService) SuggestAliases(contexts []PlaceholderContext) (*AliasSuggestionResult, error) {
	if s.typhoonAPIKey == "" {
		return nil, fmt.Errorf("TYPHOON_API_KEY not configured")
	}

	if len(contexts) == 0 {
		return &AliasSuggestionResult{
			Suggestions: []AliasSuggestion{},
			Model:       "typhoon-v2.1-12b-instruct",
			Provider:    "typhoon",
		}, nil
	}

	fmt.Printf("[Typhoon Alias] Inferring aliases for %d placeholders\n", len(contexts))

	// Build context descriptions for the prompt
	var contextDescriptions []string
	for i, ctx := range contexts {
		contextDescriptions = append(contextDescriptions, fmt.Sprintf(
			"%d. Placeholder: %s\n   Context: \"%s\"",
			i+1, ctx.Placeholder, ctx.FullContext,
		))
	}

	// Build prompt for Typhoon
	prompt := fmt.Sprintf(`คุณเป็นผู้เชี่ยวชาญในการวิเคราะห์เอกสารภาษาไทย

ฉันมี placeholders ในเอกสาร template และต้องการให้คุณแนะนำ "ชื่อที่แสดง" (alias) ที่เหมาะสมเป็นภาษาไทย โดยดูจากบริบทรอบๆ placeholder

%s

กรุณาวิเคราะห์บริบทและแนะนำชื่อภาษาไทยที่เหมาะสมสำหรับแต่ละ placeholder

ตอบเป็น JSON format เท่านั้น:
{
  "suggestions": [
    {
      "placeholder": "{{placeholder_name}}",
      "suggested_alias": "ชื่อภาษาไทยที่แนะนำ",
      "confidence": 0.9,
      "reasoning": "เหตุผลสั้นๆ"
    }
  ]
}

สำคัญ:
- ใช้ชื่อ placeholder เดิมให้ตรงกับที่ให้มา
- แนะนำ alias เป็นภาษาไทยที่กระชับและเข้าใจง่าย
- confidence เป็นตัวเลข 0-1 (1 = มั่นใจมาก)
- ตอบเป็น JSON เท่านั้น ไม่ต้องมีคำอธิบายอื่น`, strings.Join(contextDescriptions, "\n\n"))

	// Build Typhoon Chat API request
	chatReq := map[string]interface{}{
		"model": "typhoon-v2.1-12b-instruct",
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"max_tokens":  2048,
		"temperature": 0.3,
		"top_p":       0.9,
	}

	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Call Typhoon Chat API
	url := "https://api.opentyphoon.ai/v1/chat/completions"
	client := &http.Client{Timeout: 60 * time.Second}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.typhoonAPIKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Typhoon API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Typhoon API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from Typhoon")
	}

	// Parse JSON from response
	responseText := chatResp.Choices[0].Message.Content
	responseText = strings.TrimSpace(responseText)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	fmt.Printf("[Typhoon Alias] Response: %s\n", responseText)

	// Parse suggestions
	var result AliasSuggestionResult
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		// Try to extract suggestions array directly
		var suggestionsOnly struct {
			Suggestions []AliasSuggestion `json:"suggestions"`
		}
		if err2 := json.Unmarshal([]byte(responseText), &suggestionsOnly); err2 != nil {
			return nil, fmt.Errorf("failed to parse suggestions: %w, response: %s", err, responseText)
		}
		result.Suggestions = suggestionsOnly.Suggestions
	}

	result.Model = "typhoon-v2.1-12b-instruct"
	result.Provider = "typhoon"

	fmt.Printf("[Typhoon Alias] Generated %d alias suggestions\n", len(result.Suggestions))

	return &result, nil
}

// SuggestAliasesFromHTML extracts placeholders from HTML content and suggests aliases
func (s *OCRService) SuggestAliasesFromHTML(htmlContent string) (*AliasSuggestionResult, error) {
	// Extract placeholders with context (100 chars before/after)
	contexts := ExtractPlaceholdersWithContext(htmlContent, 100)

	if len(contexts) == 0 {
		return &AliasSuggestionResult{
			Suggestions: []AliasSuggestion{},
			Model:       "typhoon-v2.1-12b-instruct",
			Provider:    "typhoon",
		}, nil
	}

	return s.SuggestAliases(contexts)
}

// SuggestAliasesFromPlaceholders suggests aliases for placeholders without context
// Uses placeholder names to infer meaning
func (s *OCRService) SuggestAliasesFromPlaceholders(placeholders []string) (*AliasSuggestionResult, error) {
	if s.typhoonAPIKey == "" {
		return nil, fmt.Errorf("TYPHOON_API_KEY not configured")
	}

	if len(placeholders) == 0 {
		return &AliasSuggestionResult{
			Suggestions: []AliasSuggestion{},
			Model:       "typhoon-v2.1-12b-instruct",
			Provider:    "typhoon",
		}, nil
	}

	fmt.Printf("[Typhoon Alias] Inferring aliases for %d placeholders (no context)\n", len(placeholders))

	// Clean placeholder names
	var cleanNames []string
	for _, p := range placeholders {
		clean := strings.TrimPrefix(p, "{{")
		clean = strings.TrimSuffix(clean, "}}")
		cleanNames = append(cleanNames, clean)
	}

	// Build prompt
	prompt := fmt.Sprintf(`คุณเป็นผู้เชี่ยวชาญในการวิเคราะห์เอกสารภาษาไทย

ฉันมี placeholders ในเอกสาร template ต่อไปนี้:
%s

กรุณาแนะนำ "ชื่อที่แสดง" (alias) เป็นภาษาไทยที่เหมาะสมสำหรับแต่ละ placeholder โดยดูจากชื่อ placeholder

ตอบเป็น JSON format เท่านั้น:
{
  "suggestions": [
    {
      "placeholder": "{{placeholder_name}}",
      "suggested_alias": "ชื่อภาษาไทยที่แนะนำ",
      "confidence": 0.8,
      "reasoning": "เหตุผลสั้นๆ"
    }
  ]
}

สำคัญ:
- ใช้ชื่อ placeholder เดิมให้ตรงกับที่ให้มา (รวม {{ และ }})
- แนะนำ alias เป็นภาษาไทยที่กระชับและเข้าใจง่าย
- ถ้า placeholder มี prefix เช่น m_ (แม่), f_ (พ่อ), c_ (เด็ก), b_ (ทารก) ให้ระบุในชื่อด้วย
- confidence เป็นตัวเลข 0-1
- ตอบเป็น JSON เท่านั้น`, strings.Join(placeholders, "\n"))

	// Build request
	chatReq := map[string]interface{}{
		"model": "typhoon-v2.1-12b-instruct",
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"max_tokens":  2048,
		"temperature": 0.3,
		"top_p":       0.9,
	}

	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := "https://api.opentyphoon.ai/v1/chat/completions"
	client := &http.Client{Timeout: 60 * time.Second}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.typhoonAPIKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Typhoon API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Typhoon API error (status %d): %s", resp.StatusCode, string(body))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from Typhoon")
	}

	responseText := chatResp.Choices[0].Message.Content
	responseText = strings.TrimSpace(responseText)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	fmt.Printf("[Typhoon Alias] Response: %s\n", responseText)

	var result AliasSuggestionResult
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		var suggestionsOnly struct {
			Suggestions []AliasSuggestion `json:"suggestions"`
		}
		if err2 := json.Unmarshal([]byte(responseText), &suggestionsOnly); err2 != nil {
			return nil, fmt.Errorf("failed to parse suggestions: %w, response: %s", err, responseText)
		}
		result.Suggestions = suggestionsOnly.Suggestions
	}

	result.Model = "typhoon-v2.1-12b-instruct"
	result.Provider = "typhoon"

	fmt.Printf("[Typhoon Alias] Generated %d alias suggestions\n", len(result.Suggestions))

	return &result, nil
}

// ============================================================================
// Field Type Inference using Typhoon
// ============================================================================

// FieldTypeSuggestion represents an AI-suggested field type for a placeholder
type FieldTypeSuggestion struct {
	Placeholder    string  `json:"placeholder"`      // Original placeholder
	SuggestedAlias string  `json:"suggested_alias"`  // AI-suggested Thai alias
	DataType       string  `json:"data_type"`        // Suggested data type
	InputType      string  `json:"input_type"`       // Suggested input type
	Entity         string  `json:"entity"`           // Suggested entity
	Confidence     float64 `json:"confidence"`       // Confidence score 0-1
	Reasoning      string  `json:"reasoning"`        // Why this type was suggested
}

// FieldTypeSuggestionResult contains all field type suggestions for a template
type FieldTypeSuggestionResult struct {
	Suggestions []FieldTypeSuggestion `json:"suggestions"`
	Model       string                `json:"model"`
	Provider    string                `json:"provider"`
}

// SuggestFieldTypes uses Typhoon AI to infer field types (data type, input type, entity, alias)
func (s *OCRService) SuggestFieldTypes(placeholders []string, contexts []PlaceholderContext) (*FieldTypeSuggestionResult, error) {
	if s.typhoonAPIKey == "" {
		return nil, fmt.Errorf("TYPHOON_API_KEY not configured")
	}

	if len(placeholders) == 0 {
		return &FieldTypeSuggestionResult{
			Suggestions: []FieldTypeSuggestion{},
			Model:       "typhoon-v2.1-12b-instruct",
			Provider:    "typhoon",
		}, nil
	}

	fmt.Printf("[Typhoon FieldType] Inferring field types for %d placeholders\n", len(placeholders))

	// Build context descriptions if available
	contextMap := make(map[string]string)
	for _, ctx := range contexts {
		contextMap[ctx.Placeholder] = ctx.FullContext
	}

	var placeholderDescriptions []string
	for i, p := range placeholders {
		desc := fmt.Sprintf("%d. %s", i+1, p)
		if ctx, ok := contextMap[p]; ok && ctx != "" {
			desc += fmt.Sprintf("\n   บริบท: \"%s\"", ctx)
		}
		placeholderDescriptions = append(placeholderDescriptions, desc)
	}

	prompt := fmt.Sprintf(`คุณเป็นผู้เชี่ยวชาญในการวิเคราะห์เอกสารและแบบฟอร์มภาษาไทย

ฉันมี placeholders ในเอกสาร template ต่อไปนี้:
%s

กรุณาวิเคราะห์และแนะนำข้อมูลสำหรับแต่ละ placeholder:
1. suggested_alias: ชื่อภาษาไทยที่เหมาะสม
2. data_type: ประเภทข้อมูล (เลือกจาก: text, id_number, date, time, number, address, province, district, subdistrict, country, name_prefix, name, weekday, phone, email, house_code, zodiac, lunar_month)
3. input_type: ประเภท input (เลือกจาก: text, select, date, time, number, textarea, checkbox)
4. entity: หมวดหมู่บุคคล (เลือกจาก: child, mother, father, informant, registrar, general)

คำแนะนำ:
- ถ้า placeholder มี prefix เช่น m_ หรือ mother_ → entity = "mother"
- ถ้า placeholder มี prefix เช่น f_ หรือ father_ → entity = "father"
- ถ้า placeholder มี prefix เช่น c_ หรือ child_ หรือ b_ หรือ baby_ → entity = "child"
- ถ้า placeholder มี prefix เช่น inf_ หรือ informant_ → entity = "informant"
- ถ้า placeholder มี prefix เช่น reg_ หรือ registrar_ → entity = "registrar"
- ถ้ามี date หรือ วัน เดือน ปี → data_type = "date", input_type = "date"
- ถ้ามี id หรือ บัตรประชาชน → data_type = "id_number"
- ถ้ามี phone หรือ โทรศัพท์ → data_type = "phone"
- ถ้ามี province หรือ จังหวัด → data_type = "province"
- ถ้ามี subdistrict หรือ sub_district หรือ tambon หรือ ตำบล หรือ แขวง → data_type = "subdistrict"
- ถ้ามี district หรือ amphoe หรือ อำเภอ หรือ เขต (แต่ไม่ใช่ subdistrict) → data_type = "district"
- ถ้ามี address หรือ ที่อยู่ (และไม่มี district/subdistrict/province) → data_type = "address", input_type = "textarea"
- ถ้ามี name หรือ ชื่อ → data_type = "name"
- ถ้ามี prefix หรือ คำนำหน้า → data_type = "name_prefix", input_type = "select"

ตอบเป็น JSON format เท่านั้น:
{
  "suggestions": [
    {
      "placeholder": "{{placeholder_name}}",
      "suggested_alias": "ชื่อภาษาไทย",
      "data_type": "text",
      "input_type": "text",
      "entity": "general",
      "confidence": 0.9,
      "reasoning": "เหตุผลสั้นๆ"
    }
  ]
}

สำคัญ: ตอบเป็น JSON เท่านั้น`, strings.Join(placeholderDescriptions, "\n\n"))

	chatReq := map[string]interface{}{
		"model": "typhoon-v2.1-12b-instruct",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens":  4096,
		"temperature": 0.3,
	}

	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := "https://api.opentyphoon.ai/v1/chat/completions"
	client := &http.Client{Timeout: 90 * time.Second}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.typhoonAPIKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Typhoon API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Typhoon API error (status %d): %s", resp.StatusCode, string(body))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from Typhoon")
	}

	responseText := chatResp.Choices[0].Message.Content
	responseText = strings.TrimSpace(responseText)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	fmt.Printf("[Typhoon FieldType] Response: %s\n", responseText)

	var result FieldTypeSuggestionResult
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		var suggestionsOnly struct {
			Suggestions []FieldTypeSuggestion `json:"suggestions"`
		}
		if err2 := json.Unmarshal([]byte(responseText), &suggestionsOnly); err2 != nil {
			return nil, fmt.Errorf("failed to parse suggestions: %w, response: %s", err, responseText)
		}
		result.Suggestions = suggestionsOnly.Suggestions
	}

	result.Model = "typhoon-v2.1-12b-instruct"
	result.Provider = "typhoon"

	fmt.Printf("[Typhoon FieldType] Generated %d field type suggestions\n", len(result.Suggestions))

	return &result, nil
}

// SuggestFieldTypesFromPlaceholders suggests field types from placeholder names only
func (s *OCRService) SuggestFieldTypesFromPlaceholders(placeholders []string) (*FieldTypeSuggestionResult, error) {
	return s.SuggestFieldTypes(placeholders, nil)
}

// DataTypeInfo represents a configurable data type for AI prompts
type DataTypeInfo struct {
	Code        string
	Name        string
	Description string
	Pattern     string
}

// SuggestFieldTypesWithDataTypes suggests field types using configurable data types from database
func (s *OCRService) SuggestFieldTypesWithDataTypes(placeholders []string, contexts []PlaceholderContext, dataTypes []DataTypeInfo) (*FieldTypeSuggestionResult, error) {
	if s.typhoonAPIKey == "" {
		return nil, fmt.Errorf("TYPHOON_API_KEY not configured")
	}

	if len(placeholders) == 0 {
		return &FieldTypeSuggestionResult{
			Suggestions: []FieldTypeSuggestion{},
			Model:       "typhoon-v2.1-12b-instruct",
			Provider:    "typhoon",
		}, nil
	}

	fmt.Printf("[Typhoon FieldType] Inferring field types for %d placeholders with %d data types\n", len(placeholders), len(dataTypes))

	// Build context descriptions if available
	contextMap := make(map[string]string)
	for _, ctx := range contexts {
		contextMap[ctx.Placeholder] = ctx.FullContext
	}

	var placeholderDescriptions []string
	for i, p := range placeholders {
		desc := fmt.Sprintf("%d. %s", i+1, p)
		if ctx, ok := contextMap[p]; ok && ctx != "" {
			desc += fmt.Sprintf("\n   บริบท: \"%s\"", ctx)
		}
		placeholderDescriptions = append(placeholderDescriptions, desc)
	}

	// Build data type list dynamically
	var dataTypeCodes []string
	var dataTypeHints []string
	for _, dt := range dataTypes {
		dataTypeCodes = append(dataTypeCodes, dt.Code)
		if dt.Pattern != "" && dt.Description != "" {
			dataTypeHints = append(dataTypeHints, fmt.Sprintf("- ถ้าตรงกับ pattern %s → data_type = \"%s\" (%s)", dt.Pattern, dt.Code, dt.Description))
		}
	}

	dataTypeList := strings.Join(dataTypeCodes, ", ")
	hintsText := strings.Join(dataTypeHints, "\n")

	prompt := fmt.Sprintf(`คุณเป็นผู้เชี่ยวชาญในการวิเคราะห์เอกสารและแบบฟอร์มภาษาไทย

ฉันมี placeholders ในเอกสาร template ต่อไปนี้:
%s

กรุณาวิเคราะห์และแนะนำข้อมูลสำหรับแต่ละ placeholder:
1. suggested_alias: ชื่อภาษาไทยที่เหมาะสม
2. data_type: ประเภทข้อมูล (เลือกจาก: %s)
3. input_type: ประเภท input (เลือกจาก: text, select, date, time, number, textarea, checkbox)
4. entity: หมวดหมู่บุคคล (เลือกจาก: child, mother, father, informant, registrar, general)

คำแนะนำ:
- ถ้า placeholder มี prefix เช่น m_ หรือ mother_ → entity = "mother"
- ถ้า placeholder มี prefix เช่น f_ หรือ father_ → entity = "father"
- ถ้า placeholder มี prefix เช่น c_ หรือ child_ หรือ b_ หรือ baby_ → entity = "child"
- ถ้า placeholder มี prefix เช่น inf_ หรือ informant_ → entity = "informant"
- ถ้า placeholder มี prefix เช่น reg_ หรือ registrar_ → entity = "registrar"
%s

ตอบเป็น JSON format เท่านั้น:
{
  "suggestions": [
    {
      "placeholder": "{{placeholder_name}}",
      "suggested_alias": "ชื่อภาษาไทย",
      "data_type": "text",
      "input_type": "text",
      "entity": "general",
      "confidence": 0.9,
      "reasoning": "เหตุผลสั้นๆ"
    }
  ]
}

สำคัญ: ตอบเป็น JSON เท่านั้น`, strings.Join(placeholderDescriptions, "\n\n"), dataTypeList, hintsText)

	chatReq := map[string]interface{}{
		"model": "typhoon-v2.1-12b-instruct",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.2,
		"max_tokens":  4096,
	}

	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Use hardcoded URL like the original function for consistency
	url := "https://api.opentyphoon.ai/v1/chat/completions"
	client := &http.Client{Timeout: 90 * time.Second}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.typhoonAPIKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Typhoon API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Typhoon API error (status %d): %s", resp.StatusCode, string(body))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from Typhoon API")
	}

	content := chatResp.Choices[0].Message.Content
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var result FieldTypeSuggestionResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		fmt.Printf("[Typhoon FieldType] Failed to parse response: %s\n", content)
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	result.Model = "typhoon-v2.1-12b-instruct"
	result.Provider = "typhoon"

	return &result, nil
}
