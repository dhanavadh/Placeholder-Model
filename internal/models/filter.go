package models

import (
	"time"

	"gorm.io/gorm"
)

// FilterCategory represents a filter category/group (e.g., "Tier", "Category", "Type")
type FilterCategory struct {
	ID          string         `json:"id" gorm:"primaryKey"`
	Code        string         `json:"code" gorm:"uniqueIndex;not null"`        // e.g., "tier", "category", "type"
	Name        string         `json:"name" gorm:"not null"`                    // Display name in Thai, e.g., "ระดับการใช้งาน"
	NameEN      string         `json:"name_en"`                                 // Display name in English, e.g., "Tier"
	Description string         `json:"description"`                             // Optional description
	FieldName   string         `json:"field_name" gorm:"not null"`              // Template field to filter on, e.g., "tier", "category", "type"
	SortOrder   int            `json:"sort_order" gorm:"default:0"`             // Order in filter list
	IsActive    bool           `json:"is_active" gorm:"default:true"`           // Whether this filter is visible
	IsSystem    bool           `json:"is_system" gorm:"default:false"`          // System filters cannot be deleted
	Options     []FilterOption `json:"options,omitempty" gorm:"foreignKey:FilterCategoryID;references:ID"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

// FilterOption represents an option within a filter category (e.g., "Free", "Premium" under "Tier")
type FilterOption struct {
	ID               string         `json:"id" gorm:"primaryKey"`
	FilterCategoryID string         `json:"filter_category_id" gorm:"index;not null"`
	Value            string         `json:"value" gorm:"not null"`                   // Actual value to filter, e.g., "free", "premium"
	Label            string         `json:"label" gorm:"not null"`                   // Display label in Thai, e.g., "ฟรี"
	LabelEN          string         `json:"label_en"`                                // Display label in English, e.g., "Free"
	Description      string         `json:"description"`                             // Optional description
	Color            string         `json:"color"`                                   // Optional color for badge
	Icon             string         `json:"icon"`                                    // Optional icon
	SortOrder        int            `json:"sort_order" gorm:"default:0"`             // Order in option list
	IsActive         bool           `json:"is_active" gorm:"default:true"`           // Whether this option is visible
	IsDefault        bool           `json:"is_default" gorm:"default:false"`         // Default selected option
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName specifies the table name for FilterCategory
func (FilterCategory) TableName() string {
	return "filter_categories"
}

// TableName specifies the table name for FilterOption
func (FilterOption) TableName() string {
	return "filter_options"
}
