package models

import (
	"time"

	"gorm.io/gorm"
)

type Template struct {
	ID           string         `gorm:"primaryKey" json:"id"`
	Filename     string         `gorm:"not null" json:"filename"`
	OriginalName string         `json:"original_name"`
	DisplayName  string         `json:"display_name"`
	Description  string         `json:"description"`
	Author       string         `json:"author"`
	GCSPath      string         `gorm:"column:gcs_path_docx" json:"gcs_path"`
	GCSPathHTML  string         `gorm:"column:gcs_path_html" json:"gcs_path_html"`    // Path to HTML preview file
	FileSize     int64          `json:"file_size"`
	MimeType     string         `json:"mime_type"`
	Placeholders string         `gorm:"type:json" json:"placeholders"` // JSON array of placeholder strings
	Positions    string         `gorm:"type:json" json:"positions"`    // JSON array of placeholder positions
	Aliases      string         `gorm:"type:json" json:"aliases"`      // JSON object mapping placeholders to aliases
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	Documents []Document `gorm:"foreignKey:TemplateID" json:"documents,omitempty"`
}

func (Template) TableName() string {
	return "document_templates"
}

type Document struct {
	ID          string         `gorm:"primaryKey" json:"id"`
	TemplateID  string         `gorm:"not null;index" json:"template_id"`
	Filename    string         `gorm:"not null" json:"filename"`
	GCSPathDocx string         `json:"gcs_path_docx"`
	GCSPathPdf  string         `json:"gcs_path_pdf,omitempty"`
	TempPDFPath string         `gorm:"-" json:"-"` // Temp PDF file path (not stored in DB)
	PDFReady    bool           `gorm:"-" json:"pdf_ready"` // PDF availability flag (not stored in DB)
	FileSize    int64          `json:"file_size"`
	MimeType    string         `json:"mime_type"`
	Data        string         `gorm:"type:json" json:"data"` // JSON object of placeholder data used
	Status      string         `gorm:"default:'completed'" json:"status"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	Template Template `gorm:"foreignKey:TemplateID" json:"template,omitempty"`
}
