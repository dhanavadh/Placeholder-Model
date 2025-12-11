package models

import (
	"time"

	"gorm.io/gorm"
)

// DocumentType represents a logical document type that groups related templates
// For example: "บัตรประชาชน" (Thai ID card) is 1 document type with 3 template variations
type DocumentType struct {
	ID          string         `gorm:"primaryKey" json:"id"`
	Code        string         `gorm:"uniqueIndex;not null" json:"code"`        // Unique code for the document type (e.g., "thai_id_card")
	Name        string         `gorm:"not null" json:"name"`                     // Display name (e.g., "บัตรประชาชน")
	NameEN      string         `json:"name_en"`                                  // English name (e.g., "Thai ID Card")
	Description    string         `json:"description"`                              // Description of the document type
	OriginalSource string         `json:"original_source"`                          // Source/origin of the document type
	Category       string         `gorm:"type:varchar(50)" json:"category"`         // Category (e.g., "identification", "certificate", "contract")
	Icon        string         `json:"icon"`                                     // Icon name/path for UI
	Color       string         `json:"color"`                                    // Color code for UI (e.g., "#FF5733")
	SortOrder   int            `gorm:"default:0" json:"sort_order"`              // Display order
	IsActive    bool           `gorm:"default:true" json:"is_active"`            // Whether this document type is active
	Metadata    string         `gorm:"type:json" json:"metadata"`                // Additional metadata as JSON
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	// Relations
	Templates []Template `gorm:"foreignKey:DocumentTypeID" json:"templates,omitempty"`
}

func (DocumentType) TableName() string {
	return "document_types"
}

// DocumentTypeCategory represents predefined categories for document types
type DocumentTypeCategory string

const (
	CategoryIdentification DocumentTypeCategory = "identification" // ID cards, passports
	CategoryCertificate    DocumentTypeCategory = "certificate"    // Birth, death, marriage certificates
	CategoryContract       DocumentTypeCategory = "contract"       // Legal contracts, agreements
	CategoryApplication    DocumentTypeCategory = "application"    // Application forms
	CategoryFinancial      DocumentTypeCategory = "financial"      // Financial documents
	CategoryGovernment     DocumentTypeCategory = "government"     // Government forms
	CategoryEducation      DocumentTypeCategory = "education"      // Educational documents
	CategoryMedical        DocumentTypeCategory = "medical"        // Medical documents
	CategoryOther          DocumentTypeCategory = "other"          // Other documents
)
