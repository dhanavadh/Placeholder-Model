package handlers

import (
	"net/http"
	"strconv"

	"DF-PLCH/internal/models"
	"DF-PLCH/internal/services"

	"github.com/gin-gonic/gin"
)

type StatisticsHandler struct {
	statisticsService *services.StatisticsService
}

func NewStatisticsHandler(statisticsService *services.StatisticsService) *StatisticsHandler {
	return &StatisticsHandler{
		statisticsService: statisticsService,
	}
}

// GetSummary returns total counts for all event types
// GET /api/v1/stats/summary
func (h *StatisticsHandler) GetSummary(c *gin.Context) {
	summary, err := h.statisticsService.GetSummary()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"summary": summary,
	})
}

// GetTemplateStats returns statistics for all templates
// GET /api/v1/stats/templates
func (h *StatisticsHandler) GetTemplateStats(c *gin.Context) {
	stats, err := h.statisticsService.GetTemplateStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"templates": stats,
	})
}

// GetStatsByTemplate returns statistics for a specific template
// GET /api/v1/stats/templates/:templateId
func (h *StatisticsHandler) GetStatsByTemplate(c *gin.Context) {
	templateID := c.Param("templateId")
	if templateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "template_id is required"})
		return
	}

	stats, err := h.statisticsService.GetStatsByTemplate(templateID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetTrends returns time-based statistics
// GET /api/v1/stats/trends?days=30&template_id=xxx
func (h *StatisticsHandler) GetTrends(c *gin.Context) {
	// Parse days parameter (default 30)
	days := 30
	if d := c.Query("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 && parsed <= 365 {
			days = parsed
		}
	}

	// Optional template_id filter
	templateID := c.Query("template_id")

	trends, err := h.statisticsService.GetTrends(days, templateID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"days":   days,
		"trends": trends,
	})
}

// GetTimeSeries returns time-based statistics for a specific event type
// GET /api/v1/stats/trends/:eventType?days=30&template_id=xxx
func (h *StatisticsHandler) GetTimeSeries(c *gin.Context) {
	eventTypeStr := c.Param("eventType")
	if eventTypeStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "event_type is required"})
		return
	}

	// Validate event type
	var eventType models.EventType
	switch eventTypeStr {
	case "form_submit":
		eventType = models.EventFormSubmit
	case "export":
		eventType = models.EventExport
	case "download":
		eventType = models.EventDownload
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"error":       "invalid event_type",
			"valid_types": []string{"form_submit", "export", "download"},
		})
		return
	}

	// Parse days parameter (default 30)
	days := 30
	if d := c.Query("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 && parsed <= 365 {
			days = parsed
		}
	}

	// Optional template_id filter
	templateID := c.Query("template_id")

	data, err := h.statisticsService.GetTimeSeries(eventType, days, templateID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"days": days,
		"data": data,
	})
}

// GetAll returns a comprehensive statistics overview
// GET /api/v1/stats
func (h *StatisticsHandler) GetAll(c *gin.Context) {
	// Get summary
	summary, err := h.statisticsService.GetSummary()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get template stats
	templateStats, err := h.statisticsService.GetTemplateStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get 30-day trends
	trends, err := h.statisticsService.GetTrends(30, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"summary":   summary,
		"templates": templateStats,
		"trends":    trends,
	})
}
