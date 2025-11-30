package services

import (
	"DF-PLCH/internal"
	"DF-PLCH/internal/models"
	"fmt"

	"github.com/google/uuid"
)

type DataTypeService struct{}

func NewDataTypeService() *DataTypeService {
	return &DataTypeService{}
}

// GetAllDataTypes returns all data types ordered by priority
func (s *DataTypeService) GetAllDataTypes(activeOnly bool) ([]models.DataType, error) {
	var dataTypes []models.DataType
	query := internal.DB.Order("priority ASC")

	if activeOnly {
		query = query.Where("is_active = ?", true)
	}

	if err := query.Find(&dataTypes).Error; err != nil {
		return nil, fmt.Errorf("failed to get data types: %w", err)
	}

	return dataTypes, nil
}

// GetDataTypeByID returns a data type by ID
func (s *DataTypeService) GetDataTypeByID(id string) (*models.DataType, error) {
	var dataType models.DataType
	if err := internal.DB.First(&dataType, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("data type not found: %w", err)
	}
	return &dataType, nil
}

// GetDataTypeByCode returns a data type by code
func (s *DataTypeService) GetDataTypeByCode(code string) (*models.DataType, error) {
	var dataType models.DataType
	if err := internal.DB.First(&dataType, "code = ?", code).Error; err != nil {
		return nil, fmt.Errorf("data type not found: %w", err)
	}
	return &dataType, nil
}

// CreateDataType creates a new data type
func (s *DataTypeService) CreateDataType(dataType *models.DataType) error {
	// Check if data type with this code already exists (including soft-deleted)
	var existing models.DataType
	err := internal.DB.Unscoped().Where("code = ?", dataType.Code).First(&existing).Error

	if err == nil {
		// Record exists
		if existing.DeletedAt.Valid {
			// It was soft-deleted, hard delete it first
			if err := internal.DB.Unscoped().Delete(&existing).Error; err != nil {
				return fmt.Errorf("failed to remove old data type: %w", err)
			}
		} else {
			// Active record with same code exists
			return fmt.Errorf("data type with code '%s' already exists", dataType.Code)
		}
	}

	dataType.ID = uuid.New().String()

	// Ensure Validation and Options are valid JSON
	if dataType.Validation == "" {
		dataType.Validation = "{}"
	}
	if dataType.Options == "" {
		dataType.Options = "{}"
	}

	if err := internal.DB.Create(dataType).Error; err != nil {
		return fmt.Errorf("failed to create data type: %w", err)
	}

	return nil
}

// UpdateDataType updates an existing data type
func (s *DataTypeService) UpdateDataType(id string, updates map[string]interface{}) error {
	result := internal.DB.Model(&models.DataType{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update data type: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("data type not found")
	}
	return nil
}

// DeleteDataType deletes a data type by ID
func (s *DataTypeService) DeleteDataType(id string) error {
	result := internal.DB.Delete(&models.DataType{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete data type: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("data type not found")
	}
	return nil
}

// InitializeDefaultDataTypes creates or updates default data types
func (s *DataTypeService) InitializeDefaultDataTypes() error {
	defaults := models.DefaultDataTypes()
	created := 0
	updated := 0

	for _, dt := range defaults {
		// Check if data type with this code already exists (including soft-deleted)
		var existing models.DataType
		err := internal.DB.Unscoped().Where("code = ?", dt.Code).First(&existing).Error

		if err != nil {
			// Doesn't exist at all, create new
			dt.ID = uuid.New().String()
			dt.IsActive = true
			if err := internal.DB.Create(&dt).Error; err != nil {
				return fmt.Errorf("failed to create default data type %s: %w", dt.Code, err)
			}
			created++
		} else if existing.DeletedAt.Valid {
			// Soft-deleted, hard delete first then create new
			if err := internal.DB.Unscoped().Delete(&existing).Error; err != nil {
				return fmt.Errorf("failed to remove soft-deleted data type %s: %w", dt.Code, err)
			}
			dt.ID = uuid.New().String()
			dt.IsActive = true
			if err := internal.DB.Create(&dt).Error; err != nil {
				return fmt.Errorf("failed to create default data type %s: %w", dt.Code, err)
			}
			created++
		} else {
			// Exists and active, update with defaults (pattern, options, etc.)
			updates := map[string]interface{}{
				"pattern":     dt.Pattern,
				"options":     dt.Options,
				"input_type":  dt.InputType,
				"description": dt.Description,
				"priority":    dt.Priority,
				"validation":  dt.Validation,
			}
			if err := internal.DB.Model(&models.DataType{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
				return fmt.Errorf("failed to update default data type %s: %w", dt.Code, err)
			}
			updated++
		}
	}

	fmt.Printf("Initialized default data types: %d created, %d updated\n", created, updated)
	return nil
}

// InputType Service

type InputTypeService struct{}

func NewInputTypeService() *InputTypeService {
	return &InputTypeService{}
}

// GetAllInputTypes returns all input types ordered by priority
func (s *InputTypeService) GetAllInputTypes(activeOnly bool) ([]models.InputType, error) {
	var inputTypes []models.InputType
	query := internal.DB.Order("priority ASC")

	if activeOnly {
		query = query.Where("is_active = ?", true)
	}

	if err := query.Find(&inputTypes).Error; err != nil {
		return nil, fmt.Errorf("failed to get input types: %w", err)
	}

	return inputTypes, nil
}

// GetInputTypeByID returns an input type by ID
func (s *InputTypeService) GetInputTypeByID(id string) (*models.InputType, error) {
	var inputType models.InputType
	if err := internal.DB.First(&inputType, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("input type not found: %w", err)
	}
	return &inputType, nil
}

// CreateInputType creates a new input type
func (s *InputTypeService) CreateInputType(inputType *models.InputType) error {
	// Check if input type with this code already exists (including soft-deleted)
	var existing models.InputType
	err := internal.DB.Unscoped().Where("code = ?", inputType.Code).First(&existing).Error

	if err == nil {
		// Record exists
		if existing.DeletedAt.Valid {
			// It was soft-deleted, hard delete it first
			if err := internal.DB.Unscoped().Delete(&existing).Error; err != nil {
				return fmt.Errorf("failed to remove old input type: %w", err)
			}
		} else {
			// Active record with same code exists
			return fmt.Errorf("input type with code '%s' already exists", inputType.Code)
		}
	}

	inputType.ID = uuid.New().String()

	if err := internal.DB.Create(inputType).Error; err != nil {
		return fmt.Errorf("failed to create input type: %w", err)
	}

	return nil
}

// UpdateInputType updates an existing input type
func (s *InputTypeService) UpdateInputType(id string, updates map[string]interface{}) error {
	result := internal.DB.Model(&models.InputType{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update input type: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("input type not found")
	}
	return nil
}

// DeleteInputType deletes an input type by ID
func (s *InputTypeService) DeleteInputType(id string) error {
	result := internal.DB.Delete(&models.InputType{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete input type: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("input type not found")
	}
	return nil
}

// InitializeDefaultInputTypes creates or updates default input types
func (s *InputTypeService) InitializeDefaultInputTypes() error {
	defaults := models.DefaultInputTypes()
	created := 0
	updated := 0

	for _, it := range defaults {
		// Check if input type with this code already exists (including soft-deleted)
		var existing models.InputType
		err := internal.DB.Unscoped().Where("code = ?", it.Code).First(&existing).Error

		if err != nil {
			// Doesn't exist at all, create new
			it.ID = uuid.New().String()
			it.IsActive = true
			if err := internal.DB.Create(&it).Error; err != nil {
				return fmt.Errorf("failed to create default input type %s: %w", it.Code, err)
			}
			created++
		} else if existing.DeletedAt.Valid {
			// Soft-deleted, hard delete first then create new
			if err := internal.DB.Unscoped().Delete(&existing).Error; err != nil {
				return fmt.Errorf("failed to remove soft-deleted input type %s: %w", it.Code, err)
			}
			it.ID = uuid.New().String()
			it.IsActive = true
			if err := internal.DB.Create(&it).Error; err != nil {
				return fmt.Errorf("failed to create default input type %s: %w", it.Code, err)
			}
			created++
		} else {
			// Exists and active, update with defaults
			updates := map[string]interface{}{
				"name":        it.Name,
				"description": it.Description,
				"priority":    it.Priority,
			}
			if err := internal.DB.Model(&models.InputType{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
				return fmt.Errorf("failed to update default input type %s: %w", it.Code, err)
			}
			updated++
		}
	}

	fmt.Printf("Initialized default input types: %d created, %d updated\n", created, updated)
	return nil
}
