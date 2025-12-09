package services

import (
	"fmt"

	"DF-PLCH/internal"
	"DF-PLCH/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type FilterService struct{}

func NewFilterService() *FilterService {
	return &FilterService{}
}

// CreateFilterCategoryRequest contains fields for creating a filter category
type CreateFilterCategoryRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	NameEN      string `json:"name_en"`
	Description string `json:"description"`
	FieldName   string `json:"field_name"`
	SortOrder   int    `json:"sort_order"`
}

// UpdateFilterCategoryRequest contains fields for updating a filter category
type UpdateFilterCategoryRequest struct {
	Name        string `json:"name"`
	NameEN      string `json:"name_en"`
	Description string `json:"description"`
	FieldName   string `json:"field_name"`
	SortOrder   *int   `json:"sort_order"`
	IsActive    *bool  `json:"is_active"`
}

// CreateFilterOptionRequest contains fields for creating a filter option
type CreateFilterOptionRequest struct {
	FilterCategoryID string `json:"filter_category_id"`
	Value            string `json:"value"`
	Label            string `json:"label"`
	LabelEN          string `json:"label_en"`
	Description      string `json:"description"`
	Color            string `json:"color"`
	Icon             string `json:"icon"`
	SortOrder        int    `json:"sort_order"`
	IsDefault        bool   `json:"is_default"`
}

// UpdateFilterOptionRequest contains fields for updating a filter option
type UpdateFilterOptionRequest struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	LabelEN     string `json:"label_en"`
	Description string `json:"description"`
	Color       string `json:"color"`
	Icon        string `json:"icon"`
	SortOrder   *int   `json:"sort_order"`
	IsActive    *bool  `json:"is_active"`
	IsDefault   *bool  `json:"is_default"`
}

// ========== Filter Category Methods ==========

// CreateCategory creates a new filter category
func (s *FilterService) CreateCategory(req *CreateFilterCategoryRequest) (*models.FilterCategory, error) {
	// Check if code already exists
	var existing models.FilterCategory
	if err := internal.DB.Where("code = ?", req.Code).First(&existing).Error; err == nil {
		return nil, fmt.Errorf("filter category with code '%s' already exists", req.Code)
	}

	category := &models.FilterCategory{
		ID:          uuid.New().String(),
		Code:        req.Code,
		Name:        req.Name,
		NameEN:      req.NameEN,
		Description: req.Description,
		FieldName:   req.FieldName,
		SortOrder:   req.SortOrder,
		IsActive:    true,
		IsSystem:    false,
	}

	if err := internal.DB.Create(category).Error; err != nil {
		return nil, fmt.Errorf("failed to create filter category: %w", err)
	}

	return category, nil
}

// GetCategoryByID retrieves a filter category by ID
func (s *FilterService) GetCategoryByID(id string) (*models.FilterCategory, error) {
	var category models.FilterCategory
	if err := internal.DB.First(&category, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("filter category not found: %w", err)
	}
	return &category, nil
}

// GetCategoryByCode retrieves a filter category by code
func (s *FilterService) GetCategoryByCode(code string) (*models.FilterCategory, error) {
	var category models.FilterCategory
	if err := internal.DB.First(&category, "code = ?", code).Error; err != nil {
		return nil, fmt.Errorf("filter category not found: %w", err)
	}
	return &category, nil
}

// GetAllCategories retrieves all filter categories with their options
func (s *FilterService) GetAllCategories(activeOnly bool) ([]models.FilterCategory, error) {
	var categories []models.FilterCategory
	query := internal.DB.Preload("Options", func(db *gorm.DB) *gorm.DB {
		q := db.Order("sort_order ASC, label ASC")
		if activeOnly {
			q = q.Where("is_active = ?", true)
		}
		return q
	}).Order("sort_order ASC, name ASC")

	if activeOnly {
		query = query.Where("is_active = ?", true)
	}

	if err := query.Find(&categories).Error; err != nil {
		return nil, fmt.Errorf("failed to get filter categories: %w", err)
	}

	return categories, nil
}

// UpdateCategory updates an existing filter category
func (s *FilterService) UpdateCategory(id string, req *UpdateFilterCategoryRequest) (*models.FilterCategory, error) {
	category, err := s.GetCategoryByID(id)
	if err != nil {
		return nil, err
	}

	// Update fields
	if req.Name != "" {
		category.Name = req.Name
	}
	if req.NameEN != "" {
		category.NameEN = req.NameEN
	}
	if req.Description != "" {
		category.Description = req.Description
	}
	if req.FieldName != "" {
		category.FieldName = req.FieldName
	}
	if req.SortOrder != nil {
		category.SortOrder = *req.SortOrder
	}
	if req.IsActive != nil {
		category.IsActive = *req.IsActive
	}

	if err := internal.DB.Save(category).Error; err != nil {
		return nil, fmt.Errorf("failed to update filter category: %w", err)
	}

	return category, nil
}

// DeleteCategory deletes a filter category (only if not a system filter)
func (s *FilterService) DeleteCategory(id string) error {
	category, err := s.GetCategoryByID(id)
	if err != nil {
		return err
	}

	if category.IsSystem {
		return fmt.Errorf("cannot delete system filter category")
	}

	// Delete all options first
	if err := internal.DB.Where("filter_category_id = ?", id).Delete(&models.FilterOption{}).Error; err != nil {
		return fmt.Errorf("failed to delete filter options: %w", err)
	}

	if err := internal.DB.Delete(category).Error; err != nil {
		return fmt.Errorf("failed to delete filter category: %w", err)
	}

	return nil
}

// ========== Filter Option Methods ==========

// CreateOption creates a new filter option
func (s *FilterService) CreateOption(req *CreateFilterOptionRequest) (*models.FilterOption, error) {
	// Verify category exists
	if _, err := s.GetCategoryByID(req.FilterCategoryID); err != nil {
		return nil, err
	}

	// Check if value already exists in this category
	var existing models.FilterOption
	if err := internal.DB.Where("filter_category_id = ? AND value = ?", req.FilterCategoryID, req.Value).First(&existing).Error; err == nil {
		return nil, fmt.Errorf("filter option with value '%s' already exists in this category", req.Value)
	}

	option := &models.FilterOption{
		ID:               uuid.New().String(),
		FilterCategoryID: req.FilterCategoryID,
		Value:            req.Value,
		Label:            req.Label,
		LabelEN:          req.LabelEN,
		Description:      req.Description,
		Color:            req.Color,
		Icon:             req.Icon,
		SortOrder:        req.SortOrder,
		IsActive:         true,
		IsDefault:        req.IsDefault,
	}

	if err := internal.DB.Create(option).Error; err != nil {
		return nil, fmt.Errorf("failed to create filter option: %w", err)
	}

	return option, nil
}

// GetOptionByID retrieves a filter option by ID
func (s *FilterService) GetOptionByID(id string) (*models.FilterOption, error) {
	var option models.FilterOption
	if err := internal.DB.First(&option, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("filter option not found: %w", err)
	}
	return &option, nil
}

// GetOptionsByCategory retrieves all options for a category
func (s *FilterService) GetOptionsByCategory(categoryID string, activeOnly bool) ([]models.FilterOption, error) {
	var options []models.FilterOption
	query := internal.DB.Where("filter_category_id = ?", categoryID).Order("sort_order ASC, label ASC")

	if activeOnly {
		query = query.Where("is_active = ?", true)
	}

	if err := query.Find(&options).Error; err != nil {
		return nil, fmt.Errorf("failed to get filter options: %w", err)
	}

	return options, nil
}

// UpdateOption updates an existing filter option
func (s *FilterService) UpdateOption(id string, req *UpdateFilterOptionRequest) (*models.FilterOption, error) {
	option, err := s.GetOptionByID(id)
	if err != nil {
		return nil, err
	}

	// Update fields
	if req.Value != "" {
		option.Value = req.Value
	}
	if req.Label != "" {
		option.Label = req.Label
	}
	if req.LabelEN != "" {
		option.LabelEN = req.LabelEN
	}
	if req.Description != "" {
		option.Description = req.Description
	}
	if req.Color != "" {
		option.Color = req.Color
	}
	if req.Icon != "" {
		option.Icon = req.Icon
	}
	if req.SortOrder != nil {
		option.SortOrder = *req.SortOrder
	}
	if req.IsActive != nil {
		option.IsActive = *req.IsActive
	}
	if req.IsDefault != nil {
		option.IsDefault = *req.IsDefault
	}

	if err := internal.DB.Save(option).Error; err != nil {
		return nil, fmt.Errorf("failed to update filter option: %w", err)
	}

	return option, nil
}

// DeleteOption deletes a filter option
func (s *FilterService) DeleteOption(id string) error {
	option, err := s.GetOptionByID(id)
	if err != nil {
		return err
	}

	if err := internal.DB.Delete(option).Error; err != nil {
		return fmt.Errorf("failed to delete filter option: %w", err)
	}

	return nil
}

// ========== Initialization ==========

// InitializeDefaultFilters creates default filter categories and options
func (s *FilterService) InitializeDefaultFilters() error {
	defaultCategories := []struct {
		Category models.FilterCategory
		Options  []models.FilterOption
	}{
		{
			Category: models.FilterCategory{
				ID:        "filter_tier",
				Code:      "tier",
				Name:      "ระดับการใช้งาน",
				NameEN:    "Tier",
				FieldName: "tier",
				SortOrder: 1,
				IsActive:  true,
				IsSystem:  true,
			},
			Options: []models.FilterOption{
				{ID: "opt_tier_free", FilterCategoryID: "filter_tier", Value: "free", Label: "Free", LabelEN: "Free", SortOrder: 1, IsActive: true},
				{ID: "opt_tier_basic", FilterCategoryID: "filter_tier", Value: "basic", Label: "Basic", LabelEN: "Basic", SortOrder: 2, IsActive: true},
				{ID: "opt_tier_premium", FilterCategoryID: "filter_tier", Value: "premium", Label: "Premium", LabelEN: "Premium", SortOrder: 3, IsActive: true},
				{ID: "opt_tier_enterprise", FilterCategoryID: "filter_tier", Value: "enterprise", Label: "Enterprise", LabelEN: "Enterprise", SortOrder: 4, IsActive: true},
			},
		},
		{
			Category: models.FilterCategory{
				ID:        "filter_category",
				Code:      "category",
				Name:      "หมวดหมู่",
				NameEN:    "Category",
				FieldName: "category",
				SortOrder: 2,
				IsActive:  true,
				IsSystem:  true,
			},
			Options: []models.FilterOption{
				{ID: "opt_cat_legal", FilterCategoryID: "filter_category", Value: "legal", Label: "กฎหมาย", LabelEN: "Legal", SortOrder: 1, IsActive: true},
				{ID: "opt_cat_finance", FilterCategoryID: "filter_category", Value: "finance", Label: "การเงิน", LabelEN: "Finance", SortOrder: 2, IsActive: true},
				{ID: "opt_cat_hr", FilterCategoryID: "filter_category", Value: "hr", Label: "ทรัพยากรบุคคล", LabelEN: "Human Resources", SortOrder: 3, IsActive: true},
				{ID: "opt_cat_education", FilterCategoryID: "filter_category", Value: "education", Label: "การศึกษา", LabelEN: "Education", SortOrder: 4, IsActive: true},
				{ID: "opt_cat_government", FilterCategoryID: "filter_category", Value: "government", Label: "ราชการ", LabelEN: "Government", SortOrder: 5, IsActive: true},
				{ID: "opt_cat_business", FilterCategoryID: "filter_category", Value: "business", Label: "ธุรกิจ", LabelEN: "Business", SortOrder: 6, IsActive: true},
				{ID: "opt_cat_other", FilterCategoryID: "filter_category", Value: "other", Label: "อื่นๆ", LabelEN: "Other", SortOrder: 99, IsActive: true},
			},
		},
		{
			Category: models.FilterCategory{
				ID:        "filter_type",
				Code:      "type",
				Name:      "ประเภท",
				NameEN:    "Type",
				FieldName: "type",
				SortOrder: 3,
				IsActive:  true,
				IsSystem:  true,
			},
			Options: []models.FilterOption{
				{ID: "opt_type_official", FilterCategoryID: "filter_type", Value: "official", Label: "Official", LabelEN: "Official", SortOrder: 1, IsActive: true},
				{ID: "opt_type_community", FilterCategoryID: "filter_type", Value: "community", Label: "Community", LabelEN: "Community", SortOrder: 2, IsActive: true},
				{ID: "opt_type_private", FilterCategoryID: "filter_type", Value: "private", Label: "Private", LabelEN: "Private", SortOrder: 3, IsActive: true},
			},
		},
	}

	for _, item := range defaultCategories {
		// Check if category exists
		var existing models.FilterCategory
		if err := internal.DB.Where("id = ?", item.Category.ID).First(&existing).Error; err != nil {
			// Create category
			if err := internal.DB.Create(&item.Category).Error; err != nil {
				return fmt.Errorf("failed to create default filter category %s: %w", item.Category.Code, err)
			}
		}

		// Create options
		for _, opt := range item.Options {
			var existingOpt models.FilterOption
			if err := internal.DB.Where("id = ?", opt.ID).First(&existingOpt).Error; err != nil {
				if err := internal.DB.Create(&opt).Error; err != nil {
					return fmt.Errorf("failed to create default filter option %s: %w", opt.Value, err)
				}
			}
		}
	}

	return nil
}

// GetFiltersWithCounts retrieves all active filter categories with option counts from templates
func (s *FilterService) GetFiltersWithCounts() ([]map[string]interface{}, error) {
	categories, err := s.GetAllCategories(true)
	if err != nil {
		return nil, err
	}

	result := make([]map[string]interface{}, 0)

	for _, cat := range categories {
		options := make([]map[string]interface{}, 0)

		for _, opt := range cat.Options {
			if !opt.IsActive {
				continue
			}

			// Count templates with this option value
			var count int64
			internal.DB.Model(&models.Template{}).Where(cat.FieldName+" = ?", opt.Value).Count(&count)

			options = append(options, map[string]interface{}{
				"id":          opt.ID,
				"value":       opt.Value,
				"label":       opt.Label,
				"label_en":    opt.LabelEN,
				"color":       opt.Color,
				"icon":        opt.Icon,
				"sort_order":  opt.SortOrder,
				"count":       count,
				"is_default":  opt.IsDefault,
				"is_active":   opt.IsActive,
			})
		}

		result = append(result, map[string]interface{}{
			"id":          cat.ID,
			"code":        cat.Code,
			"name":        cat.Name,
			"name_en":     cat.NameEN,
			"field_name":  cat.FieldName,
			"sort_order":  cat.SortOrder,
			"is_system":   cat.IsSystem,
			"is_active":   cat.IsActive,
			"options":     options,
		})
	}

	return result, nil
}
