package services

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"DF-PLCH/internal"
	"DF-PLCH/internal/models"
)

type AutoSuggestService struct{}

func NewAutoSuggestService() *AutoSuggestService {
	return &AutoSuggestService{}
}

// SuggestedGroup represents a suggested document type grouping
type SuggestedGroup struct {
	SuggestedName     string              `json:"suggested_name"`
	SuggestedCode     string              `json:"suggested_code"`
	SuggestedCategory string              `json:"suggested_category"`
	Templates         []SuggestedTemplate `json:"templates"`
	Confidence        float64             `json:"confidence"`
	ExistingTypeID    string              `json:"existing_type_id"`
	ExistingTypeName  string              `json:"existing_type_name"`
}

// SuggestedTemplate represents a template with suggested variant info
type SuggestedTemplate struct {
	ID               string `json:"id"`
	DisplayName      string `json:"display_name"`
	Filename         string `json:"filename"`
	SuggestedVariant string `json:"suggested_variant"`
	VariantOrder     int    `json:"variant_order"`
}

// Regex patterns to extract base name and variant from template names
// Pattern format: base_name + separator + variant
var namePatterns = []*regexp.Regexp{
	// Thai patterns with various separators
	regexp.MustCompile(`^(.+?)[\s_\-]+?(ด้านหน้า|ด้านหลัง|หน้า|หลัง|สำเนา|ฉบับจริง|ต้นฉบับ)(.*)$`),
	regexp.MustCompile(`^(.+?)[\s_\-]+?(แบบ\s*[กขคงจฉชซฌญ]|แบบ\s*\d+|รูปแบบ\s*\d+)(.*)$`),
	regexp.MustCompile(`^(.+?)[\s_\-]+?(v\d+|version\s*\d+|เวอร์ชัน\s*\d+)(.*)$`),
	regexp.MustCompile(`^(.+?)[\s_\-]+?(\d+)$`), // Ends with number
	regexp.MustCompile(`^(.+?)[\s_\-]+?(front|back|copy|original)(.*)$`),
	// Parentheses patterns
	regexp.MustCompile(`^(.+?)\s*[\(（](.+?)[\)）]$`),
	// Underscore/dash separated with short suffix
	regexp.MustCompile(`^(.+?)[\s_\-]+([^\s_\-]{1,10})$`),
}

// Common Thai document patterns for category detection
var thaiDocumentPatterns = map[string]string{
	"บัตรประชาชน":    "identification",
	"บัตรประจำตัว":   "identification",
	"หนังสือเดินทาง":  "identification",
	"passport":       "identification",
	"id card":        "identification",
	"สูติบัตร":        "certificate",
	"มรณบัตร":        "certificate",
	"ทะเบียนสมรส":    "certificate",
	"ใบสมรส":         "certificate",
	"ทะเบียนหย่า":    "certificate",
	"ทะเบียนบ้าน":    "government",
	"สำเนาทะเบียน":   "government",
	"สัญญา":          "contract",
	"ใบรับรอง":       "certificate",
	"ใบอนุญาต":       "government",
	"แบบฟอร์ม":       "application",
	"คำขอ":           "application",
	"ใบสมัคร":        "application",
	"ใบเสร็จ":        "financial",
	"ใบแจ้งหนี้":     "financial",
	"ใบกำกับ":        "financial",
	"บันทึก":         "government",
	"หนังสือ":        "government",
	"รายงาน":         "other",
}

// Variant order mapping (lower = first)
var variantOrderMap = map[string]int{
	"ด้านหน้า":  0,
	"หน้า":      0,
	"front":    0,
	"ด้านหลัง":  1,
	"หลัง":      1,
	"back":     1,
	"สำเนา":     2,
	"copy":     2,
	"ฉบับจริง":  0,
	"ต้นฉบับ":   0,
	"original": 0,
	"แบบ ก":    0,
	"แบบก":     0,
	"แบบ ข":    1,
	"แบบข":     1,
	"แบบ ค":    2,
	"แบบค":     2,
	"แบบ 1":    0,
	"แบบ1":     0,
	"แบบ 2":    1,
	"แบบ2":     1,
	"แบบ 3":    2,
	"แบบ3":     2,
	"1":        0,
	"2":        1,
	"3":        2,
	"4":        3,
	"5":        4,
}

// GetAutoSuggestions analyzes ALL templates (assigned or not) and suggests groupings
func (s *AutoSuggestService) GetAutoSuggestions() ([]SuggestedGroup, error) {
	// Get ALL templates (not just unassigned ones)
	var templates []models.Template
	if err := internal.DB.Find(&templates).Error; err != nil {
		return nil, err
	}

	if len(templates) == 0 {
		return []SuggestedGroup{}, nil
	}

	// Get existing document types
	var existingTypes []models.DocumentType
	internal.DB.Find(&existingTypes)

	// Build name -> document type map for matching
	existingTypeMap := make(map[string]*models.DocumentType)
	for i := range existingTypes {
		normalizedName := normalizeForMatching(existingTypes[i].Name)
		existingTypeMap[normalizedName] = &existingTypes[i]
		if existingTypes[i].NameEN != "" {
			existingTypeMap[normalizeForMatching(existingTypes[i].NameEN)] = &existingTypes[i]
		}
	}

	// Group templates by extracted base name
	groupMap := make(map[string][]templateWithInfo)

	for _, t := range templates {
		// Skip already assigned templates
		if t.DocumentTypeID != "" {
			continue
		}

		name := t.DisplayName
		if name == "" {
			name = t.Filename
		}

		baseName, variantName := extractBaseAndVariantRegex(name)
		if baseName == "" {
			baseName = cleanTemplateName(name)
		}

		normalizedBase := normalizeForMatching(baseName)

		groupMap[normalizedBase] = append(groupMap[normalizedBase], templateWithInfo{
			Template:    t,
			BaseName:    baseName,
			VariantName: variantName,
		})
	}

	// Merge similar groups (fuzzy matching)
	groupMap = mergeSimilarGroups(groupMap)

	// Convert to suggestions
	var suggestions []SuggestedGroup

	for _, templateInfos := range groupMap {
		if len(templateInfos) == 0 {
			continue
		}

		// Find the best base name (most common or longest)
		baseName := findBestBaseName(templateInfos)
		normalizedBase := normalizeForMatching(baseName)

		// Check if matches existing document type
		var existingType *models.DocumentType
		for key, dt := range existingTypeMap {
			if strings.Contains(normalizedBase, key) || strings.Contains(key, normalizedBase) {
				existingType = dt
				break
			}
		}

		// Build suggested templates list with proper ordering
		suggestedTemplates := make([]SuggestedTemplate, len(templateInfos))
		for i, info := range templateInfos {
			variantName := info.VariantName
			if variantName == "" {
				variantName = guessVariantFromName(info.Template.DisplayName, info.Template.Filename, i)
			}

			order := getVariantOrder(variantName, i)

			suggestedTemplates[i] = SuggestedTemplate{
				ID:               info.Template.ID,
				DisplayName:      info.Template.DisplayName,
				Filename:         info.Template.Filename,
				SuggestedVariant: variantName,
				VariantOrder:     order,
			}
		}

		// Sort by variant order
		sort.Slice(suggestedTemplates, func(i, j int) bool {
			return suggestedTemplates[i].VariantOrder < suggestedTemplates[j].VariantOrder
		})

		// Update order after sorting
		for i := range suggestedTemplates {
			suggestedTemplates[i].VariantOrder = i
		}

		// Calculate confidence
		confidence := calculateConfidence(templateInfos, baseName)

		suggestion := SuggestedGroup{
			SuggestedName:     baseName,
			SuggestedCode:     generateCode(baseName),
			SuggestedCategory: guessCategory(baseName),
			Templates:         suggestedTemplates,
			Confidence:        confidence,
		}

		if existingType != nil {
			suggestion.ExistingTypeID = existingType.ID
			suggestion.ExistingTypeName = existingType.Name
			suggestion.Confidence = 0.95
		}

		// Only suggest groups with multiple templates or high confidence
		if len(templateInfos) >= 2 || existingType != nil {
			suggestions = append(suggestions, suggestion)
		}
	}

	// Sort by confidence descending, then by template count
	sort.Slice(suggestions, func(i, j int) bool {
		if suggestions[i].Confidence != suggestions[j].Confidence {
			return suggestions[i].Confidence > suggestions[j].Confidence
		}
		return len(suggestions[i].Templates) > len(suggestions[j].Templates)
	})

	return suggestions, nil
}

// SuggestForTemplate suggests document type for a single template
func (s *AutoSuggestService) SuggestForTemplate(templateID string) (*SuggestedGroup, error) {
	var template models.Template
	if err := internal.DB.First(&template, "id = ?", templateID).Error; err != nil {
		return nil, err
	}

	name := template.DisplayName
	if name == "" {
		name = template.Filename
	}

	baseName, variantName := extractBaseAndVariantRegex(name)
	if baseName == "" {
		baseName = cleanTemplateName(name)
	}

	// Get existing document types for matching
	var existingTypes []models.DocumentType
	internal.DB.Where("is_active = ?", true).Find(&existingTypes)

	normalizedBase := normalizeForMatching(baseName)

	// Find best matching existing document type
	var bestMatch *models.DocumentType
	bestScore := 0.0

	for i := range existingTypes {
		dt := &existingTypes[i]
		score := calculateMatchScore(normalizedBase, dt.Name, dt.NameEN)
		if score > bestScore {
			bestScore = score
			bestMatch = dt
		}
	}

	// Find other templates with similar names
	var similarTemplates []models.Template
	internal.DB.Where("id != ? AND (document_type_id IS NULL OR document_type_id = '')", templateID).Find(&similarTemplates)

	var relatedTemplates []SuggestedTemplate
	relatedTemplates = append(relatedTemplates, SuggestedTemplate{
		ID:               template.ID,
		DisplayName:      template.DisplayName,
		Filename:         template.Filename,
		SuggestedVariant: variantName,
		VariantOrder:     0,
	})

	for _, t := range similarTemplates {
		tName := t.DisplayName
		if tName == "" {
			tName = t.Filename
		}
		tBase, tVariant := extractBaseAndVariantRegex(tName)
		if tBase == "" {
			tBase = cleanTemplateName(tName)
		}

		// Check if similar base name
		if normalizeForMatching(tBase) == normalizedBase ||
			strings.Contains(normalizeForMatching(tBase), normalizedBase) ||
			strings.Contains(normalizedBase, normalizeForMatching(tBase)) {
			relatedTemplates = append(relatedTemplates, SuggestedTemplate{
				ID:               t.ID,
				DisplayName:      t.DisplayName,
				Filename:         t.Filename,
				SuggestedVariant: tVariant,
				VariantOrder:     len(relatedTemplates),
			})
		}
	}

	suggestion := &SuggestedGroup{
		SuggestedName:     baseName,
		SuggestedCode:     generateCode(baseName),
		SuggestedCategory: guessCategory(baseName),
		Templates:         relatedTemplates,
		Confidence:        bestScore,
	}

	if bestMatch != nil && bestScore > 0.5 {
		suggestion.ExistingTypeID = bestMatch.ID
		suggestion.ExistingTypeName = bestMatch.Name
	}

	return suggestion, nil
}

// Helper types and functions

type templateWithInfo struct {
	Template    models.Template
	BaseName    string
	VariantName string
}

// extractBaseAndVariantRegex uses regex patterns to extract base name and variant
func extractBaseAndVariantRegex(name string) (baseName, variantName string) {
	if name == "" {
		return "", ""
	}

	// Clean the name first
	name = cleanTemplateName(name)

	// Try each pattern
	for _, pattern := range namePatterns {
		matches := pattern.FindStringSubmatch(name)
		if len(matches) >= 3 {
			baseName = strings.TrimSpace(matches[1])
			variantName = strings.TrimSpace(matches[2])
			if len(matches) > 3 && matches[3] != "" {
				variantName = variantName + strings.TrimSpace(matches[3])
			}
			// Validate: base name should be meaningful
			if len(baseName) >= 2 && len(variantName) >= 1 {
				return baseName, variantName
			}
		}
	}

	return name, ""
}

// cleanTemplateName removes file extensions and common noise
func cleanTemplateName(name string) string {
	// Remove file extension
	name = strings.TrimSuffix(name, ".docx")
	name = strings.TrimSuffix(name, ".DOCX")
	name = strings.TrimSuffix(name, ".doc")
	name = strings.TrimSuffix(name, ".DOC")

	// Remove common prefixes/suffixes
	name = strings.TrimSpace(name)

	return name
}

// normalizeForMatching normalizes a string for comparison
func normalizeForMatching(s string) string {
	s = strings.ToLower(s)
	// Remove spaces, underscores, dashes
	s = regexp.MustCompile(`[\s_\-]+`).ReplaceAllString(s, "")
	return s
}

// mergeSimilarGroups merges groups with similar base names
func mergeSimilarGroups(groupMap map[string][]templateWithInfo) map[string][]templateWithInfo {
	keys := make([]string, 0, len(groupMap))
	for k := range groupMap {
		keys = append(keys, k)
	}

	// Sort by length descending (prefer longer base names)
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j])
	})

	merged := make(map[string][]templateWithInfo)
	used := make(map[string]bool)

	for _, key := range keys {
		if used[key] {
			continue
		}

		templates := groupMap[key]
		used[key] = true

		// Find similar keys to merge
		for _, otherKey := range keys {
			if used[otherKey] || otherKey == key {
				continue
			}

			// Check if one contains the other (substring match)
			if strings.Contains(key, otherKey) || strings.Contains(otherKey, key) {
				templates = append(templates, groupMap[otherKey]...)
				used[otherKey] = true
			}
		}

		if len(templates) > 0 {
			merged[key] = templates
		}
	}

	return merged
}

// findBestBaseName finds the most appropriate base name from a group
func findBestBaseName(templates []templateWithInfo) string {
	if len(templates) == 0 {
		return ""
	}

	// Count occurrences of each base name
	counts := make(map[string]int)
	for _, t := range templates {
		counts[t.BaseName]++
	}

	// Find the most common and longest
	var bestName string
	bestScore := 0

	for name, count := range counts {
		// Score = count * 10 + length (prefer common names, then longer names)
		score := count*10 + len(name)
		if score > bestScore {
			bestScore = score
			bestName = name
		}
	}

	return bestName
}

// guessVariantFromName guesses a variant name from template name
func guessVariantFromName(displayName, filename string, index int) string {
	name := displayName
	if name == "" {
		name = filename
	}

	_, variant := extractBaseAndVariantRegex(name)
	if variant != "" {
		return variant
	}

	// Default variant names
	defaults := []string{"รูปแบบ 1", "รูปแบบ 2", "รูปแบบ 3", "รูปแบบ 4", "รูปแบบ 5"}
	if index < len(defaults) {
		return defaults[index]
	}

	return ""
}

// getVariantOrder returns the display order for a variant
func getVariantOrder(variant string, defaultOrder int) int {
	normalized := strings.ToLower(strings.TrimSpace(variant))

	for pattern, order := range variantOrderMap {
		if strings.Contains(normalized, strings.ToLower(pattern)) {
			return order
		}
	}

	return defaultOrder + 10 // Put unknown variants at the end
}

// generateCode creates a URL-safe code from a name
func generateCode(name string) string {
	code := strings.ToLower(name)
	// Replace non-alphanumeric characters with underscores
	code = regexp.MustCompile(`[^\w]+`).ReplaceAllString(code, "_")
	code = regexp.MustCompile(`_+`).ReplaceAllString(code, "_")
	code = strings.Trim(code, "_")

	if len(code) > 50 {
		code = code[:50]
	}

	// If code is empty (e.g., all Thai characters), generate a unique code
	if code == "" {
		// Generate a short hash from the name
		hash := fmt.Sprintf("%x", []byte(name))
		if len(hash) > 12 {
			hash = hash[:12]
		}
		code = "doc_" + hash
	}
	return code
}

// guessCategory guesses the document category from name
func guessCategory(name string) string {
	lowerName := strings.ToLower(name)

	for pattern, category := range thaiDocumentPatterns {
		if strings.Contains(lowerName, strings.ToLower(pattern)) {
			return category
		}
	}

	return "other"
}

// calculateConfidence calculates confidence score for a group
func calculateConfidence(templates []templateWithInfo, baseName string) float64 {
	confidence := 0.5

	// More templates = higher confidence
	if len(templates) >= 2 {
		confidence += 0.15
	}
	if len(templates) >= 3 {
		confidence += 0.1
	}
	if len(templates) >= 4 {
		confidence += 0.05
	}

	// Longer base name = higher confidence
	if len(baseName) > 5 {
		confidence += 0.1
	}
	if len(baseName) > 10 {
		confidence += 0.05
	}

	// Known document pattern = higher confidence
	for pattern := range thaiDocumentPatterns {
		if strings.Contains(strings.ToLower(baseName), strings.ToLower(pattern)) {
			confidence += 0.15
			break
		}
	}

	// All templates have variants = higher confidence
	hasVariants := true
	for _, t := range templates {
		if t.VariantName == "" {
			hasVariants = false
			break
		}
	}
	if hasVariants && len(templates) > 1 {
		confidence += 0.1
	}

	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// AutoGroupAllTemplates automatically groups all unassigned templates
// Creates document types and assigns templates in one operation
func (s *AutoSuggestService) AutoGroupAllTemplates() ([]models.DocumentType, error) {
	suggestions, err := s.GetAutoSuggestions()
	if err != nil {
		return nil, err
	}

	var createdTypes []models.DocumentType

	for _, suggestion := range suggestions {
		// Skip if no templates or low confidence
		if len(suggestion.Templates) < 2 && suggestion.Confidence < 0.7 {
			continue
		}

		var documentTypeID string

		// Use existing type or create new one
		if suggestion.ExistingTypeID != "" {
			documentTypeID = suggestion.ExistingTypeID
		} else {
			// Create new document type
			newDocType := models.DocumentType{
				ID:          generateUUID(),
				Code:        suggestion.SuggestedCode,
				Name:        suggestion.SuggestedName,
				Category:    suggestion.SuggestedCategory,
				Color:       getCategoryColor(suggestion.SuggestedCategory),
				IsActive:    true,
				Metadata:    "{}",
			}

			if err := internal.DB.Create(&newDocType).Error; err != nil {
				// Skip if code already exists
				continue
			}
			documentTypeID = newDocType.ID
			createdTypes = append(createdTypes, newDocType)
		}

		// Assign all templates to this document type (only if not already assigned)
		for _, t := range suggestion.Templates {
			internal.DB.Model(&models.Template{}).
				Where("id = ? AND (document_type_id IS NULL OR document_type_id = '')", t.ID).
				Updates(map[string]interface{}{
					"document_type_id": documentTypeID,
					"variant_name":     t.SuggestedVariant,
					"variant_order":    t.VariantOrder,
				})
		}
	}

	return createdTypes, nil
}

func generateUUID() string {
	return fmt.Sprintf("%s", newUUID())
}

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func getCategoryColor(category string) string {
	colors := map[string]string{
		"identification": "#3B82F6",
		"certificate":    "#10B981",
		"contract":       "#F59E0B",
		"application":    "#8B5CF6",
		"financial":      "#EF4444",
		"government":     "#6366F1",
		"education":      "#EC4899",
		"medical":        "#14B8A6",
		"other":          "#6B7280",
	}
	if c, ok := colors[category]; ok {
		return c
	}
	return "#6B7280"
}

// calculateMatchScore calculates how well a template matches a document type
func calculateMatchScore(normalizedTemplate, docTypeName, docTypeNameEN string) float64 {
	normalizedDocType := normalizeForMatching(docTypeName)
	normalizedDocTypeEN := normalizeForMatching(docTypeNameEN)

	// Exact match
	if normalizedTemplate == normalizedDocType || normalizedTemplate == normalizedDocTypeEN {
		return 1.0
	}

	// Contains match
	if strings.Contains(normalizedTemplate, normalizedDocType) ||
		strings.Contains(normalizedDocType, normalizedTemplate) {
		return 0.8
	}

	if normalizedDocTypeEN != "" {
		if strings.Contains(normalizedTemplate, normalizedDocTypeEN) ||
			strings.Contains(normalizedDocTypeEN, normalizedTemplate) {
			return 0.7
		}
	}

	// Partial match
	minLen := len(normalizedTemplate)
	if len(normalizedDocType) < minLen {
		minLen = len(normalizedDocType)
	}

	commonPrefix := 0
	for i := 0; i < minLen; i++ {
		if normalizedTemplate[i] == normalizedDocType[i] {
			commonPrefix++
		} else {
			break
		}
	}

	if commonPrefix > 3 {
		return float64(commonPrefix) / float64(minLen) * 0.6
	}

	return 0
}
