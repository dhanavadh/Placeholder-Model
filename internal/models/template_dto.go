package models

import (
	"encoding/json"
	"time"
)

// TemplateListItem represents a clean, public-facing template for listing
// This DTO hides internal implementation details and provides a cleaner API response
type TemplateListItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	// Classification
	Type       TemplateType `json:"type"`
	Tier       Tier         `json:"tier"`
	IsVerified bool         `json:"is_verified"`

	// URLs for accessing resources (not internal paths)
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	PreviewURL   string `json:"preview_url"`

	// Document type info (simplified)
	DocumentType *DocumentTypeInfo `json:"document_type,omitempty"`

	// Variant info for multi-part documents
	Variant *VariantInfo `json:"variant,omitempty"`

	// Summary stats
	PlaceholderCount int `json:"placeholder_count"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DocumentTypeInfo is a simplified document type for listing
type DocumentTypeInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category,omitempty"`
}

// VariantInfo represents variant information for templates
type VariantInfo struct {
	Name  string `json:"name"`
	Order int    `json:"order"`
}

// TemplateListResponse is the response for GET /api/v1/templates
type TemplateListResponse struct {
	Templates []TemplateListItem `json:"templates"`
	Total     int                `json:"total"`
}

// TemplateDetailItem represents a detailed template for single template view
// Includes parsed placeholders and field definitions as proper objects
type TemplateDetailItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	// Classification
	Type          TemplateType `json:"type"`
	Tier          Tier         `json:"tier"`
	IsVerified    bool         `json:"is_verified"`
	IsAIAvailable bool         `json:"is_ai_available"`

	// URLs for accessing resources
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	PreviewURL   string `json:"preview_url"`
	DownloadURL  string `json:"download_url,omitempty"`

	// Document type info
	DocumentType *DocumentTypeInfo `json:"document_type,omitempty"`

	// Variant info
	Variant *VariantInfo `json:"variant,omitempty"`

	// Parsed template data (as proper objects, not JSON strings)
	Placeholders     []string                   `json:"placeholders"`
	Aliases          map[string]string          `json:"aliases,omitempty"`
	FieldDefinitions map[string]FieldDefinition `json:"field_definitions,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FieldDefinition represents a single field definition (for DTO)
type FieldDefinition struct {
	Placeholder  string            `json:"placeholder"`
	DataType     string            `json:"dataType"`
	Entity       string            `json:"entity"`
	InputType    string            `json:"inputType"`
	Label        string            `json:"label,omitempty"`
	Description  string            `json:"description,omitempty"`
	Order        int               `json:"order"`
	Group        string            `json:"group,omitempty"`
	GroupOrder   int               `json:"groupOrder,omitempty"`
	DefaultValue string            `json:"defaultValue,omitempty"`
	Validation   *FieldValidation  `json:"validation,omitempty"`
	IsMerged     bool              `json:"isMerged,omitempty"`
	MergedFields []string          `json:"mergedFields,omitempty"`
}

// FieldValidation for field validation rules
type FieldValidation struct {
	Pattern   string   `json:"pattern,omitempty"`
	MinLength *int     `json:"minLength,omitempty"`
	MaxLength *int     `json:"maxLength,omitempty"`
	Min       *int     `json:"min,omitempty"`
	Max       *int     `json:"max,omitempty"`
	Options   []string `json:"options,omitempty"`
	Required  bool     `json:"required,omitempty"`
}

// ToListItem converts a Template model to a clean TemplateListItem DTO
func (t *Template) ToListItem(baseURL string) TemplateListItem {
	item := TemplateListItem{
		ID:          t.ID,
		Name:        t.getDisplayName(),
		Description: t.Description,
		Type:        t.Type,
		Tier:        t.Tier,
		IsVerified:  t.IsVerified,
		PreviewURL:  baseURL + "/api/v1/templates/" + t.ID + "/preview",
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}

	// Set thumbnail URL if available
	if t.GCSPathThumbnail != "" {
		item.ThumbnailURL = baseURL + "/api/v1/templates/" + t.ID + "/thumbnail"
	}

	// Parse placeholders to get count
	if t.Placeholders != "" {
		var placeholders []string
		if err := json.Unmarshal([]byte(t.Placeholders), &placeholders); err == nil {
			item.PlaceholderCount = len(placeholders)
		}
	}

	// Add document type info if available
	if t.DocumentType != nil {
		item.DocumentType = &DocumentTypeInfo{
			ID:       t.DocumentType.ID,
			Name:     t.DocumentType.Name,
			Category: t.DocumentType.Category,
		}
	}

	// Add variant info if this is a variant
	if t.VariantName != "" {
		item.Variant = &VariantInfo{
			Name:  t.VariantName,
			Order: t.VariantOrder,
		}
	}

	return item
}

// ToDetailItem converts a Template model to a detailed TemplateDetailItem DTO
func (t *Template) ToDetailItem(baseURL string) TemplateDetailItem {
	item := TemplateDetailItem{
		ID:            t.ID,
		Name:          t.getDisplayName(),
		Description:   t.Description,
		Type:          t.Type,
		Tier:          t.Tier,
		IsVerified:    t.IsVerified,
		IsAIAvailable: t.IsAIAvailable,
		PreviewURL:    baseURL + "/api/v1/templates/" + t.ID + "/preview",
		CreatedAt:     t.CreatedAt,
		UpdatedAt:     t.UpdatedAt,
	}

	// Set thumbnail URL if available
	if t.GCSPathThumbnail != "" {
		item.ThumbnailURL = baseURL + "/api/v1/templates/" + t.ID + "/thumbnail"
	}

	// Parse placeholders from JSON string to array
	if t.Placeholders != "" {
		var placeholders []string
		if err := json.Unmarshal([]byte(t.Placeholders), &placeholders); err == nil {
			item.Placeholders = placeholders
		}
	}

	// Parse aliases from JSON string to map
	if t.Aliases != "" {
		var aliases map[string]string
		if err := json.Unmarshal([]byte(t.Aliases), &aliases); err == nil {
			item.Aliases = aliases
		}
	}

	// Parse field definitions from JSON string to map
	if t.FieldDefinitions != "" {
		var fieldDefs map[string]FieldDefinition
		if err := json.Unmarshal([]byte(t.FieldDefinitions), &fieldDefs); err == nil {
			item.FieldDefinitions = fieldDefs
		}
	}

	// Add document type info if available
	if t.DocumentType != nil {
		item.DocumentType = &DocumentTypeInfo{
			ID:       t.DocumentType.ID,
			Name:     t.DocumentType.Name,
			Category: t.DocumentType.Category,
		}
	}

	// Add variant info if this is a variant
	if t.VariantName != "" {
		item.Variant = &VariantInfo{
			Name:  t.VariantName,
			Order: t.VariantOrder,
		}
	}

	return item
}

// getDisplayName returns the best available name for display
func (t *Template) getDisplayName() string {
	if t.DisplayName != "" {
		return t.DisplayName
	}
	if t.Name != "" {
		return t.Name
	}
	return t.OriginalName
}

// ToListItems converts a slice of Templates to TemplateListItems
func ToListItems(templates []Template, baseURL string) []TemplateListItem {
	items := make([]TemplateListItem, len(templates))
	for i, t := range templates {
		items[i] = t.ToListItem(baseURL)
	}
	return items
}
