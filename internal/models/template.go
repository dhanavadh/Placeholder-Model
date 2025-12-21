package models

import (
	"time"

	"gorm.io/gorm"
)

// Category represents the template category
type Category string

const (
	CategoryFrequentlyUsed Category = "frequently_used" // ใช้งานบ่อย
	// Add more categories as needed
)

// TemplateType represents the template ownership/visibility type
type TemplateType string

const (
	TypeOfficial  TemplateType = "official"  // Official templates
	TypePrivate   TemplateType = "private"   // Office/Private templates
	TypeCommunity TemplateType = "community" // Community templates
)

// Tier represents the user tier required to access the template
type Tier string

const (
	TierFree       Tier = "free"
	TierBasic      Tier = "basic"
	TierPremium    Tier = "premium"
	TierEnterprise Tier = "enterprise"
	// Add more tiers as needed
)

// PageOrientation represents the document page orientation
type PageOrientation string

const (
	OrientationPortrait  PageOrientation = "portrait"
	OrientationLandscape PageOrientation = "landscape"
)

type Template struct {
	ID           string         `gorm:"primaryKey" json:"id"`
	Filename     string         `gorm:"not null" json:"filename"`
	OriginalName string         `json:"original_name"`
	DisplayName  string         `json:"display_name"`
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Author       string         `json:"author"`
	Category     Category       `gorm:"type:varchar(50)" json:"category"`
	GCSPath          string         `gorm:"column:gcs_path_docx" json:"gcs_path"`
	GCSPathHTML      string         `gorm:"column:gcs_path_html" json:"gcs_path_html"`           // Path to HTML preview file (auto-generated from DOCX)
	GCSPathPDF       string         `gorm:"column:gcs_path_pdf" json:"gcs_path_pdf"`             // Path to PDF preview file (auto-generated from DOCX)
	GCSPathThumbnail string         `gorm:"column:gcs_path_thumbnail" json:"gcs_path_thumbnail"` // Path to thumbnail image (PNG) for gallery preview
	FileSize     int64          `json:"file_size"`
	MimeType     string         `json:"mime_type"`
	Placeholders string         `gorm:"type:json" json:"placeholders"` // JSON array of placeholder strings
	Aliases      string         `gorm:"type:json" json:"aliases"`      // JSON object mapping placeholders to aliases
	FieldDefinitions string     `gorm:"type:json" json:"field_definitions"` // JSON object of field type definitions
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	// New fields
	OriginalSource string       `json:"original_source"`           // Source of the template
	Remarks        string       `json:"remarks"`                   // Additional remarks
	IsVerified     bool         `gorm:"default:false" json:"is_verified"`
	IsAIAvailable  bool         `gorm:"default:false" json:"is_ai_available"`
	Type           TemplateType `gorm:"type:varchar(20)" json:"type"` // official, private, community
	Tier           Tier         `gorm:"type:varchar(20)" json:"tier"` // Required tier to access
	Group          string       `gorm:"column:\"group\"" json:"group"` // Group for variations (e.g., ID card variations)

	// Document Type grouping - links template to a logical document type
	DocumentTypeID string        `gorm:"index" json:"document_type_id"` // FK to document_types table
	VariantName    string        `json:"variant_name"`                   // Name of this variant (e.g., "ด้านหน้า", "ด้านหลัง")
	VariantOrder   int           `gorm:"default:0" json:"variant_order"` // Display order within document type

	// Page orientation (detected from DOCX)
	PageOrientation PageOrientation `gorm:"type:varchar(20);default:'portrait'" json:"page_orientation"`

	// Relations
	DocumentType *DocumentType `gorm:"foreignKey:DocumentTypeID" json:"document_type,omitempty"`
	Documents    []Document    `gorm:"foreignKey:TemplateID" json:"documents,omitempty"`
}

func (Template) TableName() string {
	return "document_templates"
}

type Document struct {
	ID          string         `gorm:"primaryKey" json:"id"`
	TemplateID  string         `gorm:"not null;index" json:"template_id"`
	UserID      string         `gorm:"index" json:"user_id"` // User who created the document
	Filename    string         `gorm:"not null" json:"filename"`
	GCSPathDocx string         `json:"gcs_path_docx"`
	GCSPathPdf  string         `json:"gcs_path_pdf,omitempty"`
	TempPDFPath string         `gorm:"-" json:"-"`           // Temp PDF file path (not stored in DB)
	PDFReady    bool           `gorm:"-" json:"pdf_ready"`   // PDF availability flag (not stored in DB)
	FileSize    int64          `json:"file_size"`
	MimeType    string         `json:"mime_type"`
	Data        string         `gorm:"type:json" json:"data"` // JSON object of placeholder data used
	Status      string         `gorm:"default:'completed'" json:"status"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	Template Template `gorm:"foreignKey:TemplateID" json:"template,omitempty"`
}
