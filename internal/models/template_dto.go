package models

import (
	"encoding/json"
	"time"
)

// TemplateResponse represents a clean template response for public API
type TemplateResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Author      string   `json:"author"`
	Category    Category `json:"category"`

	// Classification
	Type          TemplateType `json:"type"`
	Tier          Tier         `json:"tier"`
	IsVerified    bool         `json:"is_verified"`
	IsAIAvailable bool         `json:"is_ai_available"`

	// Parsed template data (as proper objects, not JSON strings)
	Placeholders     []string               `json:"placeholders"`
	Aliases          map[string]string      `json:"aliases"`
	FieldDefinitions map[string]interface{} `json:"field_definitions"`

	// Additional metadata (for editing/display)
	OriginalSource string `json:"original_source,omitempty"`
	Remarks        string `json:"remarks,omitempty"`
	Group          string `json:"group,omitempty"`
	FileSize       int64  `json:"file_size,omitempty"`

	// Document type grouping
	DocumentTypeID string                `json:"document_type_id,omitempty"`
	VariantName    string                `json:"variant_name,omitempty"`
	VariantOrder   int                   `json:"variant_order,omitempty"`
	DocumentType   *DocumentTypeResponse `json:"document_type,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DocumentTypeResponse is a clean document type for API response
type DocumentTypeResponse struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	NameEn      string             `json:"name_en,omitempty"`
	Code        string             `json:"code,omitempty"`
	Category    string             `json:"category,omitempty"`
	Description string             `json:"description,omitempty"`
	Templates   []TemplateResponse `json:"templates,omitempty"`
}

// TemplateListResponse is the response for GET /api/v1/templates
type TemplateListResponse struct {
	Templates []TemplateResponse `json:"templates"`
}

// GroupedTemplatesResponse is the response for GET /api/v1/templates?grouped=true
type GroupedTemplatesResponse struct {
	DocumentTypes   []DocumentTypeResponse `json:"document_types"`
	OrphanTemplates []TemplateResponse     `json:"orphan_templates"`
}

// ToResponse converts a Template model to a clean TemplateResponse
func (t *Template) ToResponse() TemplateResponse {
	resp := TemplateResponse{
		ID:             t.ID,
		Name:           t.getDisplayName(),
		Description:    t.Description,
		Author:         t.Author,
		Category:       t.Category,
		Type:           t.Type,
		Tier:           t.Tier,
		IsVerified:     t.IsVerified,
		IsAIAvailable:  t.IsAIAvailable,
		OriginalSource: t.OriginalSource,
		Remarks:        t.Remarks,
		Group:          t.Group,
		FileSize:       t.FileSize,
		DocumentTypeID: t.DocumentTypeID,
		VariantName:    t.VariantName,
		VariantOrder:   t.VariantOrder,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
	}

	// Parse placeholders from JSON string to array
	if t.Placeholders != "" {
		var placeholders []string
		if err := json.Unmarshal([]byte(t.Placeholders), &placeholders); err == nil {
			resp.Placeholders = placeholders
		}
	}
	if resp.Placeholders == nil {
		resp.Placeholders = []string{}
	}

	// Parse aliases from JSON string to map
	if t.Aliases != "" {
		var aliases map[string]string
		if err := json.Unmarshal([]byte(t.Aliases), &aliases); err == nil {
			resp.Aliases = aliases
		}
	}
	if resp.Aliases == nil {
		resp.Aliases = map[string]string{}
	}

	// Parse field definitions from JSON string to map
	if t.FieldDefinitions != "" {
		var fieldDefs map[string]interface{}
		if err := json.Unmarshal([]byte(t.FieldDefinitions), &fieldDefs); err == nil {
			resp.FieldDefinitions = fieldDefs
		}
	}
	if resp.FieldDefinitions == nil {
		resp.FieldDefinitions = map[string]interface{}{}
	}

	// Add document type if loaded
	if t.DocumentType != nil {
		resp.DocumentType = t.DocumentType.ToResponse()
	}

	return resp
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

// ToResponseList converts a slice of Templates to TemplateResponses
func ToResponseList(templates []Template) []TemplateResponse {
	responses := make([]TemplateResponse, len(templates))
	for i := range templates {
		responses[i] = templates[i].ToResponse()
	}
	return responses
}

// ToResponse converts a DocumentType model to a clean DocumentTypeResponse
func (dt *DocumentType) ToResponse() *DocumentTypeResponse {
	if dt == nil {
		return nil
	}
	resp := &DocumentTypeResponse{
		ID:          dt.ID,
		Name:        dt.Name,
		NameEn:      dt.NameEN,
		Code:        dt.Code,
		Category:    dt.Category,
		Description: dt.Description,
	}

	// Convert templates if present
	if dt.Templates != nil {
		resp.Templates = ToResponseList(dt.Templates)
	}

	return resp
}

// ToResponseListDocTypes converts a slice of DocumentTypes to DocumentTypeResponses
func ToResponseListDocTypes(docTypes []DocumentType) []DocumentTypeResponse {
	responses := make([]DocumentTypeResponse, len(docTypes))
	for i := range docTypes {
		resp := docTypes[i].ToResponse()
		if resp != nil {
			responses[i] = *resp
		}
	}
	return responses
}
