package services

import (
	"fmt"
	"time"

	"DF-PLCH/internal"
	"DF-PLCH/internal/models"

	"github.com/google/uuid"
)

type StatisticsService struct{}

func NewStatisticsService() *StatisticsService {
	return &StatisticsService{}
}

// IncrementStat increments the count for a specific event type and optional template
// It uses upsert logic to either create a new record or increment existing one
func (s *StatisticsService) IncrementStat(eventType models.EventType, templateID string) error {
	today := time.Now().UTC().Truncate(24 * time.Hour)

	// Try to find existing record for today
	var stat models.Statistics
	query := internal.DB.Where("event_type = ? AND date = ?", eventType, today)

	if templateID != "" {
		query = query.Where("template_id = ?", templateID)
	} else {
		query = query.Where("template_id IS NULL OR template_id = ''")
	}

	result := query.First(&stat)

	if result.Error != nil {
		// Record doesn't exist, create new one
		stat = models.Statistics{
			ID:         uuid.New().String(),
			EventType:  eventType,
			TemplateID: templateID,
			Date:       today,
			Count:      1,
		}
		if err := internal.DB.Create(&stat).Error; err != nil {
			// Handle race condition - another request might have created it
			// Try to increment instead
			return s.incrementExisting(eventType, templateID, today)
		}
		return nil
	}

	// Increment existing record
	return internal.DB.Model(&stat).UpdateColumn("count", stat.Count+1).Error
}

// incrementExisting handles the case where a record was created by another request
func (s *StatisticsService) incrementExisting(eventType models.EventType, templateID string, date time.Time) error {
	query := internal.DB.Model(&models.Statistics{}).
		Where("event_type = ? AND date = ?", eventType, date)

	if templateID != "" {
		query = query.Where("template_id = ?", templateID)
	} else {
		query = query.Where("template_id IS NULL OR template_id = ''")
	}

	return query.UpdateColumn("count", internal.DB.Raw("count + 1")).Error
}

// RecordFormSubmit records a form submission event
func (s *StatisticsService) RecordFormSubmit(templateID string) error {
	// Record global stat (no template ID)
	if err := s.IncrementStat(models.EventFormSubmit, ""); err != nil {
		fmt.Printf("Warning: failed to record global form submit stat: %v\n", err)
	}

	// Record per-template stat
	if templateID != "" {
		if err := s.IncrementStat(models.EventFormSubmit, templateID); err != nil {
			return fmt.Errorf("failed to record template form submit stat: %w", err)
		}
	}

	return nil
}

// RecordExport records an export/download event
func (s *StatisticsService) RecordExport(templateID string) error {
	// Record global stat
	if err := s.IncrementStat(models.EventExport, ""); err != nil {
		fmt.Printf("Warning: failed to record global export stat: %v\n", err)
	}

	// Record per-template stat
	if templateID != "" {
		if err := s.IncrementStat(models.EventExport, templateID); err != nil {
			return fmt.Errorf("failed to record template export stat: %w", err)
		}
	}

	return nil
}

// RecordDownload records a download event (separate from export for granularity)
func (s *StatisticsService) RecordDownload(templateID string) error {
	// Record global stat
	if err := s.IncrementStat(models.EventDownload, ""); err != nil {
		fmt.Printf("Warning: failed to record global download stat: %v\n", err)
	}

	// Record per-template stat
	if templateID != "" {
		if err := s.IncrementStat(models.EventDownload, templateID); err != nil {
			return fmt.Errorf("failed to record template download stat: %w", err)
		}
	}

	return nil
}

// GetSummary returns total counts for all event types
// Combines real-time statistics with historical data from documents table
func (s *StatisticsService) GetSummary() (*models.StatisticsSummary, error) {
	summary := &models.StatisticsSummary{}

	// Get historical form submits from documents table (all past submissions)
	var historicalSubmits int64
	if err := internal.DB.Model(&models.Document{}).
		Where("deleted_at IS NULL").
		Count(&historicalSubmits).Error; err != nil {
		fmt.Printf("Warning: failed to get historical document count: %v\n", err)
	}

	// Get real-time form submits from statistics table
	var realtimeFormSubmits int64
	if err := internal.DB.Model(&models.Statistics{}).
		Where("event_type = ? AND (template_id IS NULL OR template_id = '')", models.EventFormSubmit).
		Select("COALESCE(SUM(count), 0)").
		Scan(&realtimeFormSubmits).Error; err != nil {
		return nil, fmt.Errorf("failed to get form submit count: %w", err)
	}

	// Use historical count as base, add real-time if tracking started after some documents existed
	// To avoid double counting, we use the maximum of historical or realtime
	if historicalSubmits > realtimeFormSubmits {
		summary.TotalFormSubmits = historicalSubmits
	} else {
		summary.TotalFormSubmits = realtimeFormSubmits
	}

	// Get total exports from statistics
	var exports int64
	if err := internal.DB.Model(&models.Statistics{}).
		Where("event_type = ? AND (template_id IS NULL OR template_id = '')", models.EventExport).
		Select("COALESCE(SUM(count), 0)").
		Scan(&exports).Error; err != nil {
		return nil, fmt.Errorf("failed to get export count: %w", err)
	}
	// For exports, we can estimate historical exports = historical documents (each doc was likely exported)
	if exports == 0 && historicalSubmits > 0 {
		exports = historicalSubmits // Assume each historical document was exported at least once
	}
	summary.TotalExports = exports

	// Get total downloads
	var downloads int64
	if err := internal.DB.Model(&models.Statistics{}).
		Where("event_type = ? AND (template_id IS NULL OR template_id = '')", models.EventDownload).
		Select("COALESCE(SUM(count), 0)").
		Scan(&downloads).Error; err != nil {
		return nil, fmt.Errorf("failed to get download count: %w", err)
	}
	summary.TotalDownloads = downloads

	return summary, nil
}

// GetTemplateStats returns statistics for all templates
// Combines real-time statistics with historical data from documents table
func (s *StatisticsService) GetTemplateStats() ([]models.TemplateStatistics, error) {
	// First, get all template IDs that have documents (historical data)
	var historicalTemplateIDs []string
	if err := internal.DB.Model(&models.Document{}).
		Where("deleted_at IS NULL AND template_id IS NOT NULL AND template_id != ''").
		Distinct("template_id").
		Pluck("template_id", &historicalTemplateIDs).Error; err != nil {
		fmt.Printf("Warning: failed to get historical template IDs: %v\n", err)
	}

	// Also get template IDs from statistics table
	var statsTemplateIDs []string
	if err := internal.DB.Model(&models.Statistics{}).
		Where("template_id IS NOT NULL AND template_id != ''").
		Distinct("template_id").
		Pluck("template_id", &statsTemplateIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to get template IDs from stats: %w", err)
	}

	// Merge unique template IDs
	templateIDSet := make(map[string]bool)
	for _, id := range historicalTemplateIDs {
		templateIDSet[id] = true
	}
	for _, id := range statsTemplateIDs {
		templateIDSet[id] = true
	}

	var stats []models.TemplateStatistics
	for templateID := range templateIDSet {
		templateStat := models.TemplateStatistics{
			TemplateID: templateID,
		}

		// Get template name - try display_name, then filename, then original_name
		var template models.Template
		if err := internal.DB.Select("display_name, filename, original_name").Where("id = ?", templateID).First(&template).Error; err == nil {
			if template.DisplayName != "" {
				templateStat.TemplateName = template.DisplayName
			} else if template.Filename != "" {
				templateStat.TemplateName = template.Filename
			} else if template.OriginalName != "" {
				templateStat.TemplateName = template.OriginalName
			} else {
				// Template exists but has no name fields set
				templateStat.TemplateName = fmt.Sprintf("Template %s", templateID[:8])
			}
		} else {
			// Template was deleted - mark it
			templateStat.TemplateName = "(deleted template)"
		}

		// Get historical document count for this template
		var historicalCount int64
		internal.DB.Model(&models.Document{}).
			Where("deleted_at IS NULL AND template_id = ?", templateID).
			Count(&historicalCount)

		// Get form submits from statistics
		var statsFormSubmits int64
		internal.DB.Model(&models.Statistics{}).
			Where("event_type = ? AND template_id = ?", models.EventFormSubmit, templateID).
			Select("COALESCE(SUM(count), 0)").
			Scan(&statsFormSubmits)

		// Use the higher of historical or stats count
		if historicalCount > statsFormSubmits {
			templateStat.FormSubmits = historicalCount
		} else {
			templateStat.FormSubmits = statsFormSubmits
		}

		// Get exports for this template
		var exports int64
		internal.DB.Model(&models.Statistics{}).
			Where("event_type = ? AND template_id = ?", models.EventExport, templateID).
			Select("COALESCE(SUM(count), 0)").
			Scan(&exports)
		// Estimate historical exports if none tracked
		if exports == 0 && historicalCount > 0 {
			exports = historicalCount
		}
		templateStat.Exports = exports

		// Get downloads for this template
		var downloads int64
		internal.DB.Model(&models.Statistics{}).
			Where("event_type = ? AND template_id = ?", models.EventDownload, templateID).
			Select("COALESCE(SUM(count), 0)").
			Scan(&downloads)
		templateStat.Downloads = downloads

		stats = append(stats, templateStat)
	}

	return stats, nil
}

// GetStatsByTemplate returns statistics for a specific template
// Combines real-time statistics with historical data from documents table
func (s *StatisticsService) GetStatsByTemplate(templateID string) (*models.TemplateStatistics, error) {
	templateStat := &models.TemplateStatistics{
		TemplateID: templateID,
	}

	// Get template name - try display_name, then filename, then original_name
	var template models.Template
	if err := internal.DB.Select("display_name, filename, original_name").Where("id = ?", templateID).First(&template).Error; err == nil {
		if template.DisplayName != "" {
			templateStat.TemplateName = template.DisplayName
		} else if template.Filename != "" {
			templateStat.TemplateName = template.Filename
		} else if template.OriginalName != "" {
			templateStat.TemplateName = template.OriginalName
		} else {
			templateStat.TemplateName = fmt.Sprintf("Template %s", templateID[:8])
		}
	} else {
		templateStat.TemplateName = "(deleted template)"
	}

	// Get historical document count for this template
	var historicalCount int64
	internal.DB.Model(&models.Document{}).
		Where("deleted_at IS NULL AND template_id = ?", templateID).
		Count(&historicalCount)

	// Get form submits from statistics
	var formSubmits int64
	if err := internal.DB.Model(&models.Statistics{}).
		Where("event_type = ? AND template_id = ?", models.EventFormSubmit, templateID).
		Select("COALESCE(SUM(count), 0)").
		Scan(&formSubmits).Error; err != nil {
		return nil, fmt.Errorf("failed to get form submit count: %w", err)
	}
	// Use higher of historical or stats
	if historicalCount > formSubmits {
		templateStat.FormSubmits = historicalCount
	} else {
		templateStat.FormSubmits = formSubmits
	}

	// Get exports
	var exports int64
	if err := internal.DB.Model(&models.Statistics{}).
		Where("event_type = ? AND template_id = ?", models.EventExport, templateID).
		Select("COALESCE(SUM(count), 0)").
		Scan(&exports).Error; err != nil {
		return nil, fmt.Errorf("failed to get export count: %w", err)
	}
	// Estimate historical exports if none tracked
	if exports == 0 && historicalCount > 0 {
		exports = historicalCount
	}
	templateStat.Exports = exports

	// Get downloads
	var downloads int64
	if err := internal.DB.Model(&models.Statistics{}).
		Where("event_type = ? AND template_id = ?", models.EventDownload, templateID).
		Select("COALESCE(SUM(count), 0)").
		Scan(&downloads).Error; err != nil {
		return nil, fmt.Errorf("failed to get download count: %w", err)
	}
	templateStat.Downloads = downloads

	return templateStat, nil
}

// GetTimeSeries returns time-based statistics for a specific event type
// Combines real-time statistics with historical data from documents table
// days is the number of days to look back
func (s *StatisticsService) GetTimeSeries(eventType models.EventType, days int, templateID string) (*models.TimeSeriesData, error) {
	startDate := time.Now().UTC().AddDate(0, 0, -days).Truncate(24 * time.Hour)

	// Map to store date -> count
	dateCountMap := make(map[string]int64)

	// For form_submit events, also get historical data from documents table
	if eventType == models.EventFormSubmit {
		var historicalResults []struct {
			Date  time.Time
			Count int64
		}

		docQuery := internal.DB.Model(&models.Document{}).
			Where("deleted_at IS NULL AND created_at >= ?", startDate)

		if templateID != "" {
			docQuery = docQuery.Where("template_id = ?", templateID)
		}

		if err := docQuery.
			Select("DATE(created_at) as date, COUNT(*) as count").
			Group("DATE(created_at)").
			Scan(&historicalResults).Error; err != nil {
			fmt.Printf("Warning: failed to get historical time series: %v\n", err)
		} else {
			for _, r := range historicalResults {
				dateStr := r.Date.Format("2006-01-02")
				dateCountMap[dateStr] = r.Count
			}
		}
	}

	// Get data from statistics table
	query := internal.DB.Model(&models.Statistics{}).
		Where("event_type = ? AND date >= ?", eventType, startDate)

	if templateID != "" {
		query = query.Where("template_id = ?", templateID)
	} else {
		// Global stats
		query = query.Where("template_id IS NULL OR template_id = ''")
	}

	var statsResults []struct {
		Date  time.Time
		Count int64
	}

	if err := query.
		Select("date, SUM(count) as count").
		Group("date").
		Order("date ASC").
		Scan(&statsResults).Error; err != nil {
		return nil, fmt.Errorf("failed to get time series data: %w", err)
	}

	// Merge stats data (use higher count to avoid double counting)
	for _, r := range statsResults {
		dateStr := r.Date.Format("2006-01-02")
		if existing, ok := dateCountMap[dateStr]; ok {
			// Use the higher value
			if r.Count > existing {
				dateCountMap[dateStr] = r.Count
			}
		} else {
			dateCountMap[dateStr] = r.Count
		}
	}

	// Convert to sorted slice
	data := &models.TimeSeriesData{
		EventType:  string(eventType),
		DataPoints: make([]models.TimeSeriesPoint, 0, len(dateCountMap)),
	}

	// Get sorted dates
	var dates []string
	for dateStr := range dateCountMap {
		dates = append(dates, dateStr)
	}
	// Sort dates
	for i := 0; i < len(dates); i++ {
		for j := i + 1; j < len(dates); j++ {
			if dates[i] > dates[j] {
				dates[i], dates[j] = dates[j], dates[i]
			}
		}
	}

	var total int64
	for _, dateStr := range dates {
		count := dateCountMap[dateStr]
		data.DataPoints = append(data.DataPoints, models.TimeSeriesPoint{
			Date:  dateStr,
			Count: count,
		})
		total += count
	}
	data.Total = total

	return data, nil
}

// GetTrends returns time-based statistics for all event types
func (s *StatisticsService) GetTrends(days int, templateID string) (map[string]*models.TimeSeriesData, error) {
	eventTypes := []models.EventType{models.EventFormSubmit, models.EventExport, models.EventDownload}
	trends := make(map[string]*models.TimeSeriesData)

	for _, et := range eventTypes {
		data, err := s.GetTimeSeries(et, days, templateID)
		if err != nil {
			return nil, err
		}
		trends[string(et)] = data
	}

	return trends, nil
}
