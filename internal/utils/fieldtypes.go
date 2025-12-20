package utils

import (
	"regexp"
	"strconv"
	"strings"
)

// DataType represents the type of data for a field
type DataType string

const (
	DataTypeText        DataType = "text"
	DataTypeIDNumber    DataType = "id_number"
	DataTypeDate        DataType = "date"
	DataTypeTime        DataType = "time"
	DataTypeNumber      DataType = "number"
	DataTypeAddress     DataType = "address"
	DataTypeProvince    DataType = "province"
	DataTypeDistrict    DataType = "district"
	DataTypeSubdistrict DataType = "subdistrict"
	DataTypeCountry     DataType = "country"
	DataTypeNamePrefix  DataType = "name_prefix"
	DataTypeName        DataType = "name"
	DataTypeWeekday     DataType = "weekday"
	DataTypePhone       DataType = "phone"
	DataTypeEmail       DataType = "email"
	DataTypeHouseCode   DataType = "house_code"
	DataTypeZodiac      DataType = "zodiac"
	DataTypeLunarMonth  DataType = "lunar_month"
	DataTypeOfficerName DataType = "officer_name"
)

// Entity represents the entity type for a field
type Entity string

const (
	EntityChild     Entity = "child"
	EntityMother    Entity = "mother"
	EntityFather    Entity = "father"
	EntityInformant Entity = "informant"
	EntityRegistrar Entity = "registrar"
	EntityGeneral   Entity = "general"
)

// InputType represents the HTML input type
type InputType string

const (
	InputTypeText     InputType = "text"
	InputTypeSelect   InputType = "select"
	InputTypeDate     InputType = "date"
	InputTypeTime     InputType = "time"
	InputTypeNumber   InputType = "number"
	InputTypeTextarea InputType = "textarea"
	InputTypeCheckbox InputType = "checkbox"
	InputTypeMerged   InputType = "merged"
	InputTypeRadio    InputType = "radio"
)

// FieldValidation contains validation rules for a field
type FieldValidation struct {
	Pattern   string   `json:"pattern,omitempty"`
	MinLength *int     `json:"minLength,omitempty"`
	MaxLength *int     `json:"maxLength,omitempty"`
	Min       *int     `json:"min,omitempty"`
	Max       *int     `json:"max,omitempty"`
	Options   []string `json:"options,omitempty"`
	Required  bool     `json:"required,omitempty"`
}

// RadioOption represents an option in a radio button group
type RadioOption struct {
	Placeholder string `json:"placeholder"` // The placeholder key (e.g., "$1", "$2")
	Label       string `json:"label"`       // Display label (e.g., "Male", "Female")
	Value       string `json:"value"`       // Value when selected (e.g., "/")
}

// FieldDefinition represents the complete definition of a field
type FieldDefinition struct {
	Placeholder  string           `json:"placeholder"`
	DataType     DataType         `json:"dataType"`
	Entity       Entity           `json:"entity"`
	InputType    InputType        `json:"inputType"`
	Validation   *FieldValidation `json:"validation,omitempty"`
	Label        string           `json:"label,omitempty"`
	Description  string           `json:"description,omitempty"`
	Group        string           `json:"group,omitempty"`        // Group name for related fields (e.g., "dollar_numbers", "4d_codes")
	GroupOrder   int              `json:"groupOrder,omitempty"`   // Order within the group
	Order        int              `json:"order"`                  // Global display order for the field (user can reorder)
	DefaultValue string           `json:"defaultValue,omitempty"` // Default value for the field (e.g., "/" for checkbox)
	// Merged field properties
	IsMerged     bool             `json:"isMerged,omitempty"`     // Whether this is a merged field
	MergedFields []string         `json:"mergedFields,omitempty"` // List of original placeholder keys that are merged
	Separator    string           `json:"separator,omitempty"`    // Separator to use when splitting the merged value
	MergePattern string           `json:"mergePattern,omitempty"` // Pattern used to detect merge (e.g., "$1-$13")
	// Radio group properties (for mutually exclusive checkbox groups like Male/Female)
	IsRadioGroup  bool          `json:"isRadioGroup,omitempty"`  // Whether this field is a radio group master
	RadioGroupId  string        `json:"radioGroupId,omitempty"`  // Unique identifier for the radio group
	RadioOptions  []RadioOption `json:"radioOptions,omitempty"`  // List of radio options with their placeholders
}

// Thai name prefixes
var NamePrefixOptions = []string{
	"นาย",
	"นาง",
	"นางสาว",
	"ด.ช.",
	"ด.ญ.",
	"เด็กชาย",
	"เด็กหญิง",
	"Mr.",
	"Mrs.",
	"Ms.",
	"Miss",
}

// Thai provinces
var ProvinceOptions = []string{
	"กรุงเทพมหานคร",
	"กระบี่",
	"กาญจนบุรี",
	"กาฬสินธุ์",
	"กำแพงเพชร",
	"ขอนแก่น",
	"จันทบุรี",
	"ฉะเชิงเทรา",
	"ชลบุรี",
	"ชัยนาท",
	"ชัยภูมิ",
	"ชุมพร",
	"เชียงราย",
	"เชียงใหม่",
	"ตรัง",
	"ตราด",
	"ตาก",
	"นครนายก",
	"นครปฐม",
	"นครพนม",
	"นครราชสีมา",
	"นครศรีธรรมราช",
	"นครสวรรค์",
	"นนทบุรี",
	"นราธิวาส",
	"น่าน",
	"บึงกาฬ",
	"บุรีรัมย์",
	"ปทุมธานี",
	"ประจวบคีรีขันธ์",
	"ปราจีนบุรี",
	"ปัตตานี",
	"พระนครศรีอยุธยา",
	"พะเยา",
	"พังงา",
	"พัทลุง",
	"พิจิตร",
	"พิษณุโลก",
	"เพชรบุรี",
	"เพชรบูรณ์",
	"แพร่",
	"ภูเก็ต",
	"มหาสารคาม",
	"มุกดาหาร",
	"แม่ฮ่องสอน",
	"ยโสธร",
	"ยะลา",
	"ร้อยเอ็ด",
	"ระนอง",
	"ระยอง",
	"ราชบุรี",
	"ลพบุรี",
	"ลำปาง",
	"ลำพูน",
	"เลย",
	"ศรีสะเกษ",
	"สกลนคร",
	"สงขลา",
	"สตูล",
	"สมุทรปราการ",
	"สมุทรสงคราม",
	"สมุทรสาคร",
	"สระแก้ว",
	"สระบุรี",
	"สิงห์บุรี",
	"สุโขทัย",
	"สุพรรณบุรี",
	"สุราษฎร์ธานี",
	"สุรินทร์",
	"หนองคาย",
	"หนองบัวลำภู",
	"อ่างทอง",
	"อำนาจเจริญ",
	"อุดรธานี",
	"อุตรดิตถ์",
	"อุทัยธานี",
	"อุบลราชธานี",
}

// Weekday options
var WeekdayOptions = []string{
	"วันจันทร์",
	"วันอังคาร",
	"วันพุธ",
	"วันพฤหัสบดี",
	"วันศุกร์",
	"วันเสาร์",
	"วันอาทิตย์",
}

// Chinese zodiac
var ZodiacOptions = []string{
	"ชวด (หนู)",
	"ฉลู (วัว)",
	"ขาล (เสือ)",
	"เถาะ (กระต่าย)",
	"มะโรง (งูใหญ่)",
	"มะเส็ง (งูเล็ก)",
	"มะเมีย (ม้า)",
	"มะแม (แพะ)",
	"วอก (ลิง)",
	"ระกา (ไก่)",
	"จอ (หมา)",
	"กุน (หมู)",
}

// Lunar months
var LunarMonthOptions = []string{
	"เดือนอ้าย",
	"เดือนยี่",
	"เดือนสาม",
	"เดือนสี่",
	"เดือนห้า",
	"เดือนหก",
	"เดือนเจ็ด",
	"เดือนแปด",
	"เดือนเก้า",
	"เดือนสิบ",
	"เดือนสิบเอ็ด",
	"เดือนสิบสอง",
}

// Helper function to create int pointer
func intPtr(v int) *int {
	return &v
}

// detectEntity detects the entity type from the placeholder key prefix
func detectEntity(key string) Entity {
	if strings.HasPrefix(key, "m_") {
		return EntityMother
	}
	if strings.HasPrefix(key, "f_") {
		return EntityFather
	}
	if strings.HasPrefix(key, "b_") {
		return EntityInformant
	}
	if strings.HasPrefix(key, "r_") {
		return EntityRegistrar
	}

	// Child/newborn fields (no prefix)
	childFields := []string{"first_name", "last_name", "name_prefix", "id_number", "dob", "place_of_birth"}
	for _, field := range childFields {
		if key == field {
			return EntityChild
		}
	}

	return EntityGeneral
}

// detectPrefixGroup detects if a key has a prefix pattern (e.g., p_something, abc_something)
// Returns the prefix group name and the remaining key, or empty string if no prefix
func detectPrefixGroup(key string) (group string, remainingKey string) {
	// Match pattern: one or more letters/numbers followed by underscore, then more content
	prefixPattern := regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9]*)_(.+)$`)
	matches := prefixPattern.FindStringSubmatch(key)

	if matches != nil {
		prefix := strings.ToLower(matches[1])
		remaining := matches[2]

		// Skip known entity prefixes (they're handled by detectEntity)
		knownEntityPrefixes := []string{"m", "f", "b", "r"}
		for _, ep := range knownEntityPrefixes {
			if prefix == ep {
				return "", key
			}
		}

		// Skip known pattern prefixes that have special handling
		knownPatternPrefixes := []string{"4d", "n"}
		for _, pp := range knownPatternPrefixes {
			if prefix == pp {
				return "", key
			}
		}

		return "prefix_" + prefix, remaining
	}

	return "", key
}

// DetectFieldType auto-detects field type from placeholder name
func DetectFieldType(placeholder string) FieldDefinition {
	// Remove {{ and }} from placeholder
	key := strings.ReplaceAll(placeholder, "{{", "")
	key = strings.ReplaceAll(key, "}}", "")
	lowerKey := strings.ToLower(key)

	// Detect entity
	entity := detectEntity(key)

	// Detect prefix group (e.g., p_name -> group "prefix_p")
	prefixGroup, _ := detectPrefixGroup(key)

	// Default definition
	definition := FieldDefinition{
		Placeholder: placeholder,
		DataType:    DataTypeText,
		Entity:      entity,
		InputType:   InputTypeText,
	}

	// Apply prefix group if detected
	if prefixGroup != "" {
		definition.Group = prefixGroup
	}

	// ID Number patterns
	if strings.Contains(lowerKey, "_id") || lowerKey == "id_number" || lowerKey == "id" {
		definition.DataType = DataTypeIDNumber
		definition.InputType = InputTypeText
		definition.Validation = &FieldValidation{
			Pattern:   `^\d{13}$`,
			MaxLength: intPtr(13),
			MinLength: intPtr(13),
		}
		definition.Description = "เลขบัตรประชาชน 13 หลัก"
		return definition
	}

	// Name prefix patterns
	if strings.Contains(lowerKey, "name_prefix") || strings.Contains(lowerKey, "_prefix") {
		definition.DataType = DataTypeNamePrefix
		definition.InputType = InputTypeSelect
		definition.Validation = &FieldValidation{Options: NamePrefixOptions}
		return definition
	}

	// Age patterns
	if strings.Contains(lowerKey, "_age") || lowerKey == "age" {
		definition.DataType = DataTypeNumber
		definition.InputType = InputTypeNumber
		definition.Validation = &FieldValidation{Min: intPtr(0), Max: intPtr(150)}
		return definition
	}

	// Date patterns
	if lowerKey == "dob" || strings.Contains(lowerKey, "date") || strings.Contains(lowerKey, "_date") {
		definition.DataType = DataTypeDate
		definition.InputType = InputTypeDate
		return definition
	}

	// Time patterns
	if lowerKey == "time" || strings.Contains(lowerKey, "_time") {
		definition.DataType = DataTypeTime
		definition.InputType = InputTypeTime
		return definition
	}

	// Weekday patterns
	if lowerKey == "weekday" || strings.Contains(lowerKey, "weekday") {
		definition.DataType = DataTypeWeekday
		definition.InputType = InputTypeSelect
		definition.Validation = &FieldValidation{Options: WeekdayOptions}
		return definition
	}

	// Province patterns
	if strings.Contains(lowerKey, "_prov") || strings.Contains(lowerKey, "province") {
		definition.DataType = DataTypeProvince
		definition.InputType = InputTypeSelect
		definition.Validation = &FieldValidation{Options: ProvinceOptions}
		return definition
	}

	// Subdistrict patterns (check BEFORE district to handle sub_district)
	if strings.Contains(lowerKey, "subdistrict") || strings.Contains(lowerKey, "sub_district") ||
		strings.Contains(lowerKey, "sub-district") || strings.Contains(lowerKey, "tambon") {
		definition.DataType = DataTypeSubdistrict
		definition.InputType = InputTypeText
		return definition
	}

	// District patterns (amphoe)
	if strings.Contains(lowerKey, "district") || strings.Contains(lowerKey, "amphoe") {
		definition.DataType = DataTypeDistrict
		definition.InputType = InputTypeText
		return definition
	}

	// Country patterns
	if strings.Contains(lowerKey, "_country") || strings.Contains(lowerKey, "country") {
		definition.DataType = DataTypeCountry
		definition.InputType = InputTypeText
		return definition
	}

	// Address patterns
	if strings.Contains(lowerKey, "_address") || strings.Contains(lowerKey, "address") {
		definition.DataType = DataTypeAddress
		definition.InputType = InputTypeTextarea
		return definition
	}

	// Name patterns (first_name, last_name, etc.)
	if strings.Contains(lowerKey, "first_name") || strings.Contains(lowerKey, "last_name") ||
		strings.Contains(lowerKey, "maiden_name") || strings.Contains(lowerKey, "_name") {
		definition.DataType = DataTypeName
		definition.InputType = InputTypeText
		return definition
	}

	// House code patterns
	if strings.Contains(lowerKey, "house_code") || strings.Contains(lowerKey, "house_no") {
		definition.DataType = DataTypeHouseCode
		definition.InputType = InputTypeText
		return definition
	}

	// Zodiac patterns
	if strings.Contains(lowerKey, "zodiac") || lowerKey == "cn_zodiac" {
		definition.DataType = DataTypeZodiac
		definition.InputType = InputTypeSelect
		definition.Validation = &FieldValidation{Options: ZodiacOptions}
		return definition
	}

	// Lunar month patterns
	if strings.Contains(lowerKey, "luna") || lowerKey == "luna_m" {
		definition.DataType = DataTypeLunarMonth
		definition.InputType = InputTypeSelect
		definition.Validation = &FieldValidation{Options: LunarMonthOptions}
		return definition
	}

	// Registration office
	if strings.Contains(lowerKey, "regis_office") || strings.Contains(lowerKey, "office") {
		definition.DataType = DataTypeText
		definition.InputType = InputTypeText
		return definition
	}

	// Place of birth
	if strings.Contains(lowerKey, "place_of_birth") {
		definition.DataType = DataTypeAddress
		definition.InputType = InputTypeText
		return definition
	}

	// Number patterns (4d_, n1, n2, $1, $2, etc.)
	fourDigitPattern := regexp.MustCompile(`^4d_(\d+)$`)
	nPattern := regexp.MustCompile(`^n(\d+)$`)
	dollarPattern := regexp.MustCompile(`^\$(\d+)$`)
	dollarSuffixPattern := regexp.MustCompile(`^\$(\d+)_([A-Za-z]+)$`) // e.g., $1_D, $1_M, $2_D

	// 4-digit code pattern (e.g., 4d_1, 4d_2)
	if matches := fourDigitPattern.FindStringSubmatch(key); matches != nil {
		definition.DataType = DataTypeNumber
		definition.InputType = InputTypeText
		definition.Group = "4d_codes"
		definition.Description = "รหัส 4 หลัก"
		definition.Validation = &FieldValidation{
			Pattern:   `^\d{4}$`,
			MaxLength: intPtr(4),
		}
		if num, err := strconv.Atoi(matches[1]); err == nil {
			definition.GroupOrder = num
		}
		return definition
	}

	// N-number pattern (e.g., n1, n2, n3)
	if matches := nPattern.FindStringSubmatch(key); matches != nil {
		definition.DataType = DataTypeNumber
		definition.InputType = InputTypeText
		definition.Group = "n_numbers"
		definition.Description = "ตัวเลข"
		if num, err := strconv.Atoi(matches[1]); err == nil {
			definition.GroupOrder = num
		}
		return definition
	}

	// Dollar pattern with suffix (e.g., $1_D, $2_D, $1_M)
	if matches := dollarSuffixPattern.FindStringSubmatch(key); matches != nil {
		definition.DataType = DataTypeNumber
		definition.InputType = InputTypeText
		suffix := strings.ToUpper(matches[2])
		definition.Group = "dollar_numbers_" + suffix
		definition.Description = "ตัวเลข (" + suffix + ")"
		if num, err := strconv.Atoi(matches[1]); err == nil {
			definition.GroupOrder = num
		}
		return definition
	}

	// Dollar pattern without suffix (e.g., $1, $2, $12)
	if matches := dollarPattern.FindStringSubmatch(key); matches != nil {
		definition.DataType = DataTypeNumber
		definition.InputType = InputTypeText
		definition.Group = "dollar_numbers"
		definition.Description = "ตัวเลข"
		if num, err := strconv.Atoi(matches[1]); err == nil {
			definition.GroupOrder = num
		}
		return definition
	}

	// Child number
	if lowerKey == "child_no" {
		definition.DataType = DataTypeNumber
		definition.InputType = InputTypeNumber
		definition.Validation = &FieldValidation{Min: intPtr(1), Max: intPtr(20)}
		return definition
	}

	return definition
}

// GenerateFieldDefinitions generates field definitions for all placeholders
func GenerateFieldDefinitions(placeholders []string) map[string]FieldDefinition {
	definitions := make(map[string]FieldDefinition)

	for _, placeholder := range placeholders {
		key := strings.ReplaceAll(placeholder, "{{", "")
		key = strings.ReplaceAll(key, "}}", "")
		definitions[key] = DetectFieldType(placeholder)
	}

	return definitions
}
