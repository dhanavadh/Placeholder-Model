package models

import (
	"time"

	"gorm.io/gorm"
)

// EventType represents the type of statistical event
type EventType string

const (
	EventFormSubmit EventType = "form_submit"
	EventExport     EventType = "export"
	EventDownload   EventType = "download"
)

// Statistics represents a single statistics record
// Tracks counts per template per day for detailed analytics
type Statistics struct {
	ID         string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	EventType  EventType      `gorm:"type:varchar(50);not null;index" json:"event_type"`
	TemplateID string         `gorm:"type:varchar(191);index" json:"template_id,omitempty"` // Optional: for per-template stats
	Date       time.Time      `gorm:"type:date;not null;index" json:"date"`                 // Day-level granularity
	Count      int64          `gorm:"not null;default:0" json:"count"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Statistics) TableName() string {
	return "statistics"
}

// StatisticsSummary represents aggregated statistics
type StatisticsSummary struct {
	TotalFormSubmits int64 `json:"total_form_submits"`
	TotalExports     int64 `json:"total_exports"`
	TotalDownloads   int64 `json:"total_downloads"`
}

// TemplateStatistics represents statistics for a specific template
type TemplateStatistics struct {
	TemplateID   string `json:"template_id"`
	TemplateName string `json:"template_name,omitempty"`
	FormSubmits  int64  `json:"form_submits"`
	Exports      int64  `json:"exports"`
	Downloads    int64  `json:"downloads"`
}

// TimeSeriesPoint represents a single point in time-based statistics
type TimeSeriesPoint struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// TimeSeriesData represents time-based statistics for a specific event type
type TimeSeriesData struct {
	EventType  string            `json:"event_type"`
	DataPoints []TimeSeriesPoint `json:"data_points"`
	Total      int64             `json:"total"`
}
