package handlers

import (
	"DF-PLCH/internal/models"
	"DF-PLCH/internal/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

type DataTypeHandler struct {
	dataTypeService  *services.DataTypeService
	inputTypeService *services.InputTypeService
}

func NewDataTypeHandler(dataTypeService *services.DataTypeService, inputTypeService *services.InputTypeService) *DataTypeHandler {
	return &DataTypeHandler{
		dataTypeService:  dataTypeService,
		inputTypeService: inputTypeService,
	}
}

// GetAllDataTypes returns all data types
func (h *DataTypeHandler) GetAllDataTypes(c *gin.Context) {
	activeOnly := c.Query("active") == "true"

	dataTypes, err := h.dataTypeService.GetAllDataTypes(activeOnly)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data_types": dataTypes,
		"total":      len(dataTypes),
	})
}

// GetDataType returns a single data type by ID
func (h *DataTypeHandler) GetDataType(c *gin.Context) {
	id := c.Param("id")

	dataType, err := h.dataTypeService.GetDataTypeByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, dataType)
}

// CreateDataType creates a new data type
func (h *DataTypeHandler) CreateDataType(c *gin.Context) {
	var req struct {
		Code         string `json:"code" binding:"required"`
		Name         string `json:"name" binding:"required"`
		Description  string `json:"description"`
		Pattern      string `json:"pattern"`
		InputType    string `json:"input_type"`
		Validation   string `json:"validation"`
		Options      string `json:"options"`
		DefaultValue string `json:"default_value"`
		Priority     int    `json:"priority"`
		IsActive     bool   `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Ensure validation and options are valid JSON or empty
	validation := req.Validation
	if validation == "" {
		validation = "{}"
	}
	options := req.Options
	if options == "" {
		options = "[]"
	}

	dataType := &models.DataType{
		Code:         req.Code,
		Name:         req.Name,
		Description:  req.Description,
		Pattern:      req.Pattern,
		InputType:    req.InputType,
		Validation:   validation,
		Options:      options,
		DefaultValue: req.DefaultValue,
		Priority:     req.Priority,
		IsActive:     req.IsActive,
	}

	if dataType.InputType == "" {
		dataType.InputType = "text"
	}

	if err := h.dataTypeService.CreateDataType(dataType); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, dataType)
}

// UpdateDataType updates an existing data type
func (h *DataTypeHandler) UpdateDataType(c *gin.Context) {
	id := c.Param("id")

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.dataTypeService.UpdateDataType(id, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	dataType, _ := h.dataTypeService.GetDataTypeByID(id)
	c.JSON(http.StatusOK, dataType)
}

// DeleteDataType deletes a data type
func (h *DataTypeHandler) DeleteDataType(c *gin.Context) {
	id := c.Param("id")

	if err := h.dataTypeService.DeleteDataType(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "data type deleted successfully"})
}

// InitializeDefaultDataTypes initializes the default data types
func (h *DataTypeHandler) InitializeDefaultDataTypes(c *gin.Context) {
	if err := h.dataTypeService.InitializeDefaultDataTypes(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "default data types initialized successfully"})
}

// Input Types Handlers

// GetAllInputTypes returns all input types
func (h *DataTypeHandler) GetAllInputTypes(c *gin.Context) {
	activeOnly := c.Query("active") == "true"

	inputTypes, err := h.inputTypeService.GetAllInputTypes(activeOnly)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"input_types": inputTypes,
		"total":       len(inputTypes),
	})
}

// GetInputType returns a single input type by ID
func (h *DataTypeHandler) GetInputType(c *gin.Context) {
	id := c.Param("id")

	inputType, err := h.inputTypeService.GetInputTypeByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, inputType)
}

// CreateInputType creates a new input type
func (h *DataTypeHandler) CreateInputType(c *gin.Context) {
	var req struct {
		Code        string `json:"code" binding:"required"`
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		Priority    int    `json:"priority"`
		IsActive    bool   `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	inputType := &models.InputType{
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
		Priority:    req.Priority,
		IsActive:    req.IsActive,
	}

	if err := h.inputTypeService.CreateInputType(inputType); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, inputType)
}

// UpdateInputType updates an existing input type
func (h *DataTypeHandler) UpdateInputType(c *gin.Context) {
	id := c.Param("id")

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.inputTypeService.UpdateInputType(id, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	inputType, _ := h.inputTypeService.GetInputTypeByID(id)
	c.JSON(http.StatusOK, inputType)
}

// DeleteInputType deletes an input type
func (h *DataTypeHandler) DeleteInputType(c *gin.Context) {
	id := c.Param("id")

	if err := h.inputTypeService.DeleteInputType(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "input type deleted successfully"})
}

// InitializeDefaultInputTypes initializes the default input types
func (h *DataTypeHandler) InitializeDefaultInputTypes(c *gin.Context) {
	if err := h.inputTypeService.InitializeDefaultInputTypes(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "default input types initialized successfully"})
}
