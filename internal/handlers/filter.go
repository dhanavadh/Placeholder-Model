package handlers

import (
	"net/http"

	"DF-PLCH/internal/services"

	"github.com/gin-gonic/gin"
)

type FilterHandler struct {
	filterService *services.FilterService
}

func NewFilterHandler(filterService *services.FilterService) *FilterHandler {
	return &FilterHandler{
		filterService: filterService,
	}
}

// ========== Filter Category Endpoints ==========

// GetAllFilters returns all filter categories with options and counts
// GET /api/v1/filters
func (h *FilterHandler) GetAllFilters(c *gin.Context) {
	filters, err := h.filterService.GetFiltersWithCounts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"filters": filters})
}

// GetAllCategories returns all filter categories
// GET /api/v1/filters/categories
func (h *FilterHandler) GetAllCategories(c *gin.Context) {
	activeOnly := c.Query("active_only") == "true"

	categories, err := h.filterService.GetAllCategories(activeOnly)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"categories": categories})
}

// GetCategory returns a single filter category
// GET /api/v1/filters/categories/:id
func (h *FilterHandler) GetCategory(c *gin.Context) {
	id := c.Param("id")

	category, err := h.filterService.GetCategoryByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, category)
}

// CreateCategory creates a new filter category
// POST /api/v1/filters/categories
func (h *FilterHandler) CreateCategory(c *gin.Context) {
	var req services.CreateFilterCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Code == "" || req.Name == "" || req.FieldName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code, name, and field_name are required"})
		return
	}

	category, err := h.filterService.CreateCategory(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, category)
}

// UpdateCategory updates a filter category
// PUT /api/v1/filters/categories/:id
func (h *FilterHandler) UpdateCategory(c *gin.Context) {
	id := c.Param("id")

	var req services.UpdateFilterCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	category, err := h.filterService.UpdateCategory(id, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, category)
}

// DeleteCategory deletes a filter category
// DELETE /api/v1/filters/categories/:id
func (h *FilterHandler) DeleteCategory(c *gin.Context) {
	id := c.Param("id")

	if err := h.filterService.DeleteCategory(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Filter category deleted successfully"})
}

// ========== Filter Option Endpoints ==========

// GetOptions returns all options for a category
// GET /api/v1/filters/categories/:id/options
func (h *FilterHandler) GetOptions(c *gin.Context) {
	categoryID := c.Param("id")
	activeOnly := c.Query("active_only") == "true"

	options, err := h.filterService.GetOptionsByCategory(categoryID, activeOnly)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"options": options})
}

// GetOption returns a single filter option
// GET /api/v1/filters/options/:id
func (h *FilterHandler) GetOption(c *gin.Context) {
	id := c.Param("id")

	option, err := h.filterService.GetOptionByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, option)
}

// CreateOption creates a new filter option
// POST /api/v1/filters/options
func (h *FilterHandler) CreateOption(c *gin.Context) {
	var req services.CreateFilterOptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.FilterCategoryID == "" || req.Value == "" || req.Label == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "filter_category_id, value, and label are required"})
		return
	}

	option, err := h.filterService.CreateOption(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, option)
}

// UpdateOption updates a filter option
// PUT /api/v1/filters/options/:id
func (h *FilterHandler) UpdateOption(c *gin.Context) {
	id := c.Param("id")

	var req services.UpdateFilterOptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	option, err := h.filterService.UpdateOption(id, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, option)
}

// DeleteOption deletes a filter option
// DELETE /api/v1/filters/options/:id
func (h *FilterHandler) DeleteOption(c *gin.Context) {
	id := c.Param("id")

	if err := h.filterService.DeleteOption(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Filter option deleted successfully"})
}

// InitializeDefaultFilters initializes default filter categories and options
// POST /api/v1/filters/initialize
func (h *FilterHandler) InitializeDefaultFilters(c *gin.Context) {
	if err := h.filterService.InitializeDefaultFilters(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Default filters initialized successfully"})
}
