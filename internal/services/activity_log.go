package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"DF-PLCH/internal"
	"DF-PLCH/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// sanitizeUTF8 ensures the string is valid UTF-8, replacing invalid bytes
func sanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	// Replace invalid UTF-8 sequences with replacement character
	return strings.ToValidUTF8(s, "�")
}

type ActivityLogService struct{}

func NewActivityLogService() *ActivityLogService {
	return &ActivityLogService{}
}

type LogEntry struct {
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	UserAgent   string            `json:"user_agent"`
	IPAddress   string            `json:"ip_address"`
	RequestBody interface{}       `json:"request_body,omitempty"`
	QueryParams map[string]string `json:"query_params,omitempty"`
	StatusCode  int               `json:"status_code"`
	ResponseTime int64            `json:"response_time"`
	Timestamp   time.Time         `json:"timestamp"`
}

func (s *ActivityLogService) LogRequest(c *gin.Context, statusCode int, responseTime time.Duration) {
	// Get client IP
	clientIP := c.ClientIP()
	if clientIP == "" {
		clientIP = c.Request.RemoteAddr
	}

	// Get user agent
	userAgent := c.Request.UserAgent()

	// Get query parameters
	queryParams := make(map[string]string)
	for key, values := range c.Request.URL.Query() {
		if len(values) > 0 {
			queryParams[key] = values[0]
		}
	}

	// Get request body from context if it was stored by middleware
	var requestBody string
	if body, exists := c.Get("request_body"); exists {
		if bodyStr, ok := body.(string); ok {
			requestBody = bodyStr
		}
	}

	// Convert query params to JSON string
	queryParamsJSON, _ := json.Marshal(queryParams)

	// Get user information from JWT token (set by gateway)
	var userID, userEmail string
	if uid, exists := c.Get("user_id"); exists {
		if uidStr, ok := uid.(string); ok {
			userID = uidStr
		}
	}
	if email, exists := c.Get("user_email"); exists {
		if emailStr, ok := email.(string); ok {
			userEmail = emailStr
		}
	}

	// Create activity log entry
	// Sanitize request body to ensure valid UTF-8 for PostgreSQL
	sanitizedRequestBody := sanitizeUTF8(requestBody)

	activityLog := &models.ActivityLog{
		ID:           uuid.New().String(),
		Method:       c.Request.Method,
		Path:         c.Request.URL.Path,
		UserAgent:    userAgent,
		IPAddress:    clientIP,
		RequestBody:  sanitizedRequestBody,
		QueryParams:  string(queryParamsJSON),
		StatusCode:   statusCode,
		ResponseTime: responseTime.Milliseconds(),
		UserID:       userID,
		UserEmail:    userEmail,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Save to database (don't block the request if this fails)
	go func() {
		if err := internal.DB.Create(activityLog).Error; err != nil {
			fmt.Printf("Failed to save activity log: %v\n", err)
		}
	}()
}

func (s *ActivityLogService) GetAllLogs(limit int, offset int) ([]models.ActivityLog, int64, error) {
	var logs []models.ActivityLog
	var total int64

	// Get total count
	if err := internal.DB.Model(&models.ActivityLog{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count logs: %w", err)
	}

	// Get logs with pagination, ordered by most recent first
	query := internal.DB.Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Find(&logs).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch logs: %w", err)
	}

	return logs, total, nil
}

func (s *ActivityLogService) GetLogsByMethod(method string, limit int, offset int) ([]models.ActivityLog, int64, error) {
	var logs []models.ActivityLog
	var total int64

	query := internal.DB.Where("method = ?", strings.ToUpper(method))

	// Get total count
	if err := query.Model(&models.ActivityLog{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count logs: %w", err)
	}

	// Get logs with pagination
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Order("created_at DESC").Find(&logs).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch logs: %w", err)
	}

	return logs, total, nil
}

func (s *ActivityLogService) GetLogsByPath(path string, limit int, offset int) ([]models.ActivityLog, int64, error) {
	var logs []models.ActivityLog
	var total int64

	query := internal.DB.Where("path LIKE ?", "%"+path+"%")

	// Get total count
	if err := query.Model(&models.ActivityLog{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count logs: %w", err)
	}

	// Get logs with pagination
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Order("created_at DESC").Find(&logs).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch logs: %w", err)
	}

	return logs, total, nil
}

// Middleware function to automatically log all requests
func (s *ActivityLogService) LoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Extract user information from headers (set by gateway)
		if userID := c.GetHeader("X-User-ID"); userID != "" {
			c.Set("user_id", userID)
		}
		if userEmail := c.GetHeader("X-User-Email"); userEmail != "" {
			c.Set("user_email", userEmail)
		}

		// Capture request body for POST requests
		if c.Request.Method == "POST" && c.Request.Body != nil {
			bodyBytes, err := io.ReadAll(c.Request.Body)
			if err == nil {
				// Restore the body for other handlers
				c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

				// Store body in context for logging
				if len(bodyBytes) > 0 {
					if len(bodyBytes) > 10000 { // 10KB limit
						c.Set("request_body", fmt.Sprintf("[Large body: %d bytes] %s...", len(bodyBytes), string(bodyBytes[:100])))
					} else {
						c.Set("request_body", string(bodyBytes))
					}
				}
			}
		}

		// Process the request
		c.Next()

		// Log after the request is processed
		duration := time.Since(start)
		s.LogRequest(c, c.Writer.Status(), duration)
	}
}