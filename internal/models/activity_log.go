package models

import (
	"time"
	"gorm.io/gorm"
)

type ActivityLog struct {
	ID           string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	Method       string         `gorm:"type:varchar(10);not null" json:"method"`
	Path         string         `gorm:"type:varchar(255);not null" json:"path"`
	UserAgent    string         `gorm:"type:text" json:"user_agent"`
	IPAddress    string         `gorm:"type:varchar(45)" json:"ip_address"`
	RequestBody  string         `gorm:"type:text" json:"request_body"`
	QueryParams  string         `gorm:"type:text" json:"query_params"`
	StatusCode   int            `gorm:"not null" json:"status_code"`
	ResponseTime int64          `gorm:"not null" json:"response_time"` // in milliseconds
	// User information from JWT token
	UserID       string         `gorm:"type:varchar(36);index" json:"user_id"`
	UserEmail    string         `gorm:"type:varchar(255);index" json:"user_email"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

func (ActivityLog) TableName() string {
	return "activity_logs"
}