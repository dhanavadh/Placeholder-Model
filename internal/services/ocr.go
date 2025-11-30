package services

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type OCRService struct {
	visionAPIKey  string
	geminiAPIKey  string
	addressAPIURL string
}

func NewOCRService() *OCRService {
	return &OCRService{
		visionAPIKey:  os.Getenv("GOOGLE_VISION_API_KEY"),
		geminiAPIKey:  os.Getenv("GOOGLE_AI_API_KEY"),
		addressAPIURL: os.Getenv("ADDRESS_API_URL"),
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

// ExtractTextFromImage calls Google Cloud Vision API to extract text
func (s *OCRService) ExtractTextFromImage(imageBase64 string) (*OCRResult, error) {
	if s.visionAPIKey == "" {
		return nil, fmt.Errorf("GOOGLE_VISION_API_KEY not configured")
	}

	// Build Vision API request
	visionReq := VisionRequest{
		Requests: []VisionRequestItem{
			{
				Image: VisionImage{
					Content: imageBase64,
				},
				Features: []VisionFeature{
					{
						Type:       "DOCUMENT_TEXT_DETECTION",
						MaxResults: 1,
					},
				},
			},
		},
	}

	reqBody, err := json.Marshal(visionReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Call Vision API
	url := fmt.Sprintf("https://vision.googleapis.com/v1/images:annotate?key=%s", s.visionAPIKey)
	client := &http.Client{Timeout: 30 * time.Second}

	resp, err := client.Post(url, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to call Vision API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Vision API error: %s", string(body))
	}

	var visionResp VisionResponse
	if err := json.Unmarshal(body, &visionResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(visionResp.Responses) == 0 {
		return nil, fmt.Errorf("no response from Vision API")
	}

	if visionResp.Responses[0].Error != nil {
		return nil, fmt.Errorf("Vision API error: %s", visionResp.Responses[0].Error.Message)
	}

	rawText := ""
	if visionResp.Responses[0].FullTextAnnotation != nil {
		rawText = visionResp.Responses[0].FullTextAnnotation.Text
	}

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
