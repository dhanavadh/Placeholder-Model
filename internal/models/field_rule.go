package models

import (
	"time"

	"gorm.io/gorm"
)

// FieldRule represents a configurable rule for detecting field types
type FieldRule struct {
	ID          string         `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"not null" json:"name"`                        // Display name for the rule
	Description string         `json:"description"`                                 // Description of what this rule does
	Pattern     string         `gorm:"not null" json:"pattern"`                     // Regex pattern to match placeholder names
	Priority    int            `gorm:"default:0" json:"priority"`                   // Higher priority rules are checked first
	IsActive    bool           `gorm:"default:true" json:"is_active"`               // Whether the rule is active
	DataType    string         `json:"data_type"`                                   // The data type to assign (text, number, date, etc.)
	InputType   string         `json:"input_type"`                                  // The input type (text, select, date, number, textarea)
	Entity      string         `json:"entity"`                                      // The entity to assign (child, mother, father, etc.)
	GroupName   string         `json:"group_name"`                                  // Group name to assign
	Validation  string         `gorm:"type:json" json:"validation"`                 // JSON validation rules
	Options     string         `gorm:"type:json" json:"options"`                    // JSON array of options for select inputs
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// EntityRule represents a configurable rule for detecting entities
type EntityRule struct {
	ID          string         `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"not null" json:"name"`           // Display name (Thai label)
	Code        string         `gorm:"not null;unique" json:"code"`    // Entity code (e.g., "mother", "father")
	Description string         `json:"description"`                    // Description
	Pattern     string         `gorm:"not null" json:"pattern"`        // Regex pattern to match placeholder names
	Priority    int            `gorm:"default:0" json:"priority"`      // Higher priority rules are checked first
	IsActive    bool           `gorm:"default:true" json:"is_active"`  // Whether the rule is active
	Color       string         `json:"color"`                          // Color for UI display (hex)
	Icon        string         `json:"icon"`                           // Icon name for UI
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (EntityRule) TableName() string {
	return "entity_rules"
}

func (FieldRule) TableName() string {
	return "field_rules"
}

// DefaultEntityRules returns the default set of entity detection rules
func DefaultEntityRules() []EntityRule {
	return []EntityRule{
		{
			ID:          "entity_mother",
			Name:        "มารดา",
			Code:        "mother",
			Description: "ช่องข้อมูลของมารดา (เริ่มด้วย m_)",
			Pattern:     `^m_`,
			Priority:    100,
			IsActive:    true,
			Color:       "#ec4899", // pink
		},
		{
			ID:          "entity_father",
			Name:        "บิดา",
			Code:        "father",
			Description: "ช่องข้อมูลของบิดา (เริ่มด้วย f_)",
			Pattern:     `^f_`,
			Priority:    100,
			IsActive:    true,
			Color:       "#3b82f6", // blue
		},
		{
			ID:          "entity_child",
			Name:        "เด็ก/ผู้เกิด",
			Code:        "child",
			Description: "ช่องข้อมูลของเด็ก (เริ่มด้วย c_ หรือ child_)",
			Pattern:     `^(c_|child_)`,
			Priority:    100,
			IsActive:    true,
			Color:       "#10b981", // green
		},
		{
			ID:          "entity_informant",
			Name:        "ผู้แจ้งเกิด",
			Code:        "informant",
			Description: "ช่องข้อมูลของผู้แจ้งเกิด (เริ่มด้วย b_ หรือ i_)",
			Pattern:     `^(b_|i_|informant_)`,
			Priority:    90,
			IsActive:    true,
			Color:       "#f59e0b", // amber
		},
		{
			ID:          "entity_registrar",
			Name:        "นายทะเบียน",
			Code:        "registrar",
			Description: "ช่องข้อมูลของนายทะเบียน (เริ่มด้วย r_)",
			Pattern:     `^r_`,
			Priority:    90,
			IsActive:    true,
			Color:       "#8b5cf6", // purple
		},
		{
			ID:          "entity_witness",
			Name:        "พยาน",
			Code:        "witness",
			Description: "ช่องข้อมูลของพยาน (เริ่มด้วย w_)",
			Pattern:     `^w_`,
			Priority:    80,
			IsActive:    true,
			Color:       "#6366f1", // indigo
		},
		{
			ID:          "entity_general",
			Name:        "ทั่วไป",
			Code:        "general",
			Description: "ช่องข้อมูลทั่วไป (ไม่ตรงกับ pattern อื่น)",
			Pattern:     `.*`,
			Priority:    0, // Lowest priority - fallback
			IsActive:    true,
			Color:       "#6b7280", // gray
		},
	}
}

// DefaultFieldRules returns the default set of field detection rules
func DefaultFieldRules() []FieldRule {
	return []FieldRule{
		// ID Number patterns
		{
			ID:          "rule_id_number",
			Name:        "เลขบัตรประชาชน",
			Description: "ตรวจจับช่องเลขบัตรประชาชน 13 หลัก",
			Pattern:     `(?i)(_id$|^id_number$|^id$)`,
			Priority:    100,
			IsActive:    true,
			DataType:    "id_number",
			InputType:   "text",
			Validation:  `{"pattern":"^\\d{13}$","minLength":13,"maxLength":13}`,
		},
		// Name prefix patterns
		{
			ID:          "rule_name_prefix",
			Name:        "คำนำหน้าชื่อ",
			Description: "ตรวจจับช่องคำนำหน้าชื่อ",
			Pattern:     `(?i)(name_prefix|_prefix$)`,
			Priority:    90,
			IsActive:    true,
			DataType:    "name_prefix",
			InputType:   "select",
		},
		// Age patterns
		{
			ID:          "rule_age",
			Name:        "อายุ",
			Description: "ตรวจจับช่องอายุ",
			Pattern:     `(?i)(_age$|^age$)`,
			Priority:    85,
			IsActive:    true,
			DataType:    "number",
			InputType:   "number",
			Validation:  `{"min":0,"max":150}`,
		},
		// Date patterns
		{
			ID:          "rule_date",
			Name:        "วันที่",
			Description: "ตรวจจับช่องวันที่",
			Pattern:     `(?i)(^dob$|date|_date$)`,
			Priority:    80,
			IsActive:    true,
			DataType:    "date",
			InputType:   "date",
		},
		// Time patterns
		{
			ID:          "rule_time",
			Name:        "เวลา",
			Description: "ตรวจจับช่องเวลา",
			Pattern:     `(?i)(^time$|_time$)`,
			Priority:    75,
			IsActive:    true,
			DataType:    "time",
			InputType:   "time",
		},
		// Province patterns
		{
			ID:          "rule_province",
			Name:        "จังหวัด",
			Description: "ตรวจจับช่องจังหวัด",
			Pattern:     `(?i)(_prov|province)`,
			Priority:    70,
			IsActive:    true,
			DataType:    "province",
			InputType:   "select",
		},
		// Address patterns
		{
			ID:          "rule_address",
			Name:        "ที่อยู่",
			Description: "ตรวจจับช่องที่อยู่",
			Pattern:     `(?i)(_address|address)`,
			Priority:    65,
			IsActive:    true,
			DataType:    "address",
			InputType:   "textarea",
		},
		// Name patterns
		{
			ID:          "rule_name",
			Name:        "ชื่อ",
			Description: "ตรวจจับช่องชื่อ",
			Pattern:     `(?i)(first_name|last_name|maiden_name|_name$)`,
			Priority:    60,
			IsActive:    true,
			DataType:    "name",
			InputType:   "text",
		},
		// Dollar number patterns ($1, $2, $1_D, etc.)
		{
			ID:          "rule_dollar_number",
			Name:        "ตัวเลข ($)",
			Description: "ตรวจจับช่องตัวเลขแบบ $1, $2",
			Pattern:     `^\$(\d+)$`,
			Priority:    50,
			IsActive:    true,
			DataType:    "number",
			InputType:   "text",
			GroupName:   "dollar_numbers",
		},
		// Dollar number with suffix patterns ($1_D, $2_M, etc.)
		{
			ID:          "rule_dollar_suffix",
			Name:        "ตัวเลข ($ + suffix)",
			Description: "ตรวจจับช่องตัวเลขแบบ $1_D, $2_M",
			Pattern:     `^\$(\d+)_([A-Za-z]+)$`,
			Priority:    55,
			IsActive:    true,
			DataType:    "number",
			InputType:   "text",
			GroupName:   "dollar_numbers_{suffix}",
		},
		// 4-digit code patterns
		{
			ID:          "rule_4d_code",
			Name:        "รหัส 4 หลัก",
			Description: "ตรวจจับช่องรหัส 4 หลัก (4d_1, 4d_2)",
			Pattern:     `^4d_(\d+)$`,
			Priority:    45,
			IsActive:    true,
			DataType:    "number",
			InputType:   "text",
			GroupName:   "4d_codes",
			Validation:  `{"pattern":"^\\d{4}$","maxLength":4}`,
		},
		// N-number patterns (n1, n2, etc.)
		{
			ID:          "rule_n_number",
			Name:        "ตัวเลข (n)",
			Description: "ตรวจจับช่องตัวเลขแบบ n1, n2",
			Pattern:     `^n(\d+)$`,
			Priority:    40,
			IsActive:    true,
			DataType:    "number",
			InputType:   "text",
			GroupName:   "n_numbers",
		},
		// Weekday patterns
		{
			ID:          "rule_weekday",
			Name:        "วันในสัปดาห์",
			Description: "ตรวจจับช่องวันในสัปดาห์",
			Pattern:     `(?i)(^weekday$|weekday)`,
			Priority:    35,
			IsActive:    true,
			DataType:    "weekday",
			InputType:   "select",
		},
		// Zodiac patterns
		{
			ID:          "rule_zodiac",
			Name:        "ปีนักษัตร",
			Description: "ตรวจจับช่องปีนักษัตร",
			Pattern:     `(?i)(zodiac|^cn_zodiac$)`,
			Priority:    30,
			IsActive:    true,
			DataType:    "zodiac",
			InputType:   "select",
		},
		// Lunar month patterns
		{
			ID:          "rule_lunar_month",
			Name:        "เดือนจันทรคติ",
			Description: "ตรวจจับช่องเดือนจันทรคติ",
			Pattern:     `(?i)(luna|^luna_m$)`,
			Priority:    25,
			IsActive:    true,
			DataType:    "lunar_month",
			InputType:   "select",
		},
		// Generic prefix grouping (p_*, x_*, etc.)
		{
			ID:          "rule_prefix_group",
			Name:        "กลุ่มตาม prefix",
			Description: "จัดกลุ่มตาม prefix เช่น p_name, x_value",
			Pattern:     `^([a-zA-Z][a-zA-Z0-9]*)_(.+)$`,
			Priority:    10,
			IsActive:    true,
			DataType:    "text",
			InputType:   "text",
			GroupName:   "prefix_{prefix}",
		},
	}
}
