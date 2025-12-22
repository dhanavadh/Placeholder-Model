package models

import (
	"time"

	"gorm.io/gorm"
)

// DataType represents a configurable data type for field detection
type DataType struct {
	ID           string         `gorm:"primaryKey" json:"id"`
	Code         string         `gorm:"not null;unique" json:"code"`
	Name         string         `gorm:"not null" json:"name"`
	Description  string         `json:"description"`
	Pattern      string         `json:"pattern"`                       // Regex pattern to match placeholder names
	InputType    string         `gorm:"default:text" json:"input_type"`
	Validation   string         `gorm:"type:jsonb" json:"validation"`
	Options      string         `gorm:"type:jsonb" json:"options"`
	DefaultValue string         `json:"default_value"`                 // Default input value
	Priority     int            `gorm:"default:0" json:"priority"`
	IsActive     bool           `gorm:"default:true" json:"is_active"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

func (DataType) TableName() string {
	return "data_types"
}

// InputType represents a configurable input type for form fields
type InputType struct {
	ID          string         `gorm:"primaryKey" json:"id"`
	Code        string         `gorm:"not null;unique" json:"code"`
	Name        string         `gorm:"not null" json:"name"`
	Description string         `json:"description"`
	Priority    int            `gorm:"default:0" json:"priority"`
	IsActive    bool           `gorm:"default:true" json:"is_active"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (InputType) TableName() string {
	return "input_types"
}

// DefaultDataTypes returns the default data types to initialize the database
func DefaultDataTypes() []DataType {
	// Pre-defined options for select types
	namePrefixOptions := `["นาย","นาง","นางสาว","เด็กชาย","เด็กหญิง","ด.ช.","ด.ญ.","Mr.","Mrs.","Miss","Ms."]`
	weekdayOptions := `["วันจันทร์","วันอังคาร","วันพุธ","วันพฤหัสบดี","วันศุกร์","วันเสาร์","วันอาทิตย์"]`
	zodiacOptions := `["ชวด","ฉลู","ขาล","เถาะ","มะโรง","มะเส็ง","มะเมีย","มะแม","วอก","ระกา","จอ","กุน"]`
	lunarMonthOptions := `["เดือนอ้าย","เดือนยี่","เดือนสาม","เดือนสี่","เดือนห้า","เดือนหก","เดือนเจ็ด","เดือนแปด","เดือนเก้า","เดือนสิบ","เดือนสิบเอ็ด","เดือนสิบสอง"]`
	countryOptions := `["ไทย","Thailand","ลาว","กัมพูชา","เวียดนาม","เมียนมา","มาเลเซีย","สิงคโปร์","อินโดนีเซีย","ฟิลิปปินส์","จีน","ญี่ปุ่น","เกาหลีใต้","อินเดีย","สหรัฐอเมริกา","อังกฤษ","ฝรั่งเศส","เยอรมนี","ออสเตรเลีย","อื่นๆ"]`

	return []DataType{
		// Default fallback - lowest priority, matches everything
		{Code: "text", Name: "ข้อความ", Description: "ข้อความทั่วไป", Pattern: ".*", InputType: "text", Priority: 0, Validation: "{}", Options: "{}"},
		// ID Number - matches _id suffix or id_number or id
		{Code: "id_number", Name: "เลขบัตรประชาชน", Description: "เลขบัตรประชาชน 13 หลัก", Pattern: `(?i)(_id$|^id_number$|^id$)`, InputType: "text", Priority: 100, Validation: `{"pattern":"^\\d{13}$","minLength":13,"maxLength":13}`, Options: "{}"},
		// Date - matches dob, date, _date
		{Code: "date", Name: "วันที่", Description: "วันที่ เช่น วันเกิด", Pattern: `(?i)(^dob$|date|_date$)`, InputType: "date", Priority: 80, Validation: "{}", Options: "{}"},
		// Time - matches time, _time
		{Code: "time", Name: "เวลา", Description: "เวลา เช่น 08:00", Pattern: `(?i)(^time$|_time$)`, InputType: "time", Priority: 75, Validation: "{}", Options: "{}"},
		// Number/Age - matches _age, age, weight, child_no
		{Code: "number", Name: "ตัวเลข", Description: "ตัวเลขทั่วไป เช่น อายุ น้ำหนัก", Pattern: `(?i)(_age$|^age$|^weight$|^child_no$)`, InputType: "number", Priority: 85, Validation: `{"min":0,"max":999}`, Options: "{}"},
		// Address - matches _address, address
		{Code: "address", Name: "ที่อยู่", Description: "ที่อยู่พร้อม autocomplete", Pattern: `(?i)(_address|address)`, InputType: "text", Priority: 65, Validation: "{}", Options: "{}"},
		// Province - matches _prov, province
		{Code: "province", Name: "จังหวัด", Description: "จังหวัด", Pattern: `(?i)(_prov|province)`, InputType: "text", Priority: 70, Validation: "{}", Options: "{}"},
		// Subdistrict - matches subdistrict, sub_district, sub-district, tambon (check BEFORE district)
		{Code: "subdistrict", Name: "ตำบล/แขวง", Description: "ตำบล/แขวง", Pattern: `(?i)(subdistrict|sub_district|sub-district|tambon)`, InputType: "text", Priority: 72, Validation: "{}", Options: "{}"},
		// District - matches district, amphoe
		{Code: "district", Name: "อำเภอ/เขต", Description: "อำเภอ/เขต", Pattern: `(?i)(district|amphoe)`, InputType: "text", Priority: 71, Validation: "{}", Options: "{}"},
		// Country - matches _country, country, _nation, nation
		{Code: "country", Name: "ประเทศ", Description: "ชื่อประเทศ", Pattern: `(?i)(_country|country|_nation|nation)`, InputType: "select", Priority: 70, Validation: "{}", Options: countryOptions},
		// Name prefix - matches name_prefix, _prefix
		{Code: "name_prefix", Name: "คำนำหน้าชื่อ", Description: "นาย นาง นางสาว", Pattern: `(?i)(name_prefix|_prefix$)`, InputType: "select", Priority: 90, Validation: "{}", Options: namePrefixOptions},
		// Name - matches first_name, last_name, _name
		{Code: "name", Name: "ชื่อ", Description: "ชื่อบุคคล", Pattern: `(?i)(first_name|last_name|maiden_name|_name$)`, InputType: "text", Priority: 60, Validation: "{}", Options: "{}"},
		// Weekday - matches weekday
		{Code: "weekday", Name: "วันในสัปดาห์", Description: "วันจันทร์ - วันอาทิตย์", Pattern: `(?i)(^weekday$|weekday)`, InputType: "select", Priority: 35, Validation: "{}", Options: weekdayOptions},
		// Phone - matches phone, _phone, _tel
		{Code: "phone", Name: "เบอร์โทรศัพท์", Description: "เบอร์โทรศัพท์", Pattern: `(?i)(phone|_phone|_tel)`, InputType: "text", Priority: 50, Validation: "{}", Options: "{}"},
		// Email - matches email, _email
		{Code: "email", Name: "อีเมล", Description: "อีเมล", Pattern: `(?i)(email|_email)`, InputType: "text", Priority: 50, Validation: "{}", Options: "{}"},
		// House code - matches house_code, house_no
		{Code: "house_code", Name: "รหัสบ้าน", Description: "รหัสบ้าน 11 หลัก", Pattern: `(?i)(house_code|house_no)`, InputType: "text", Priority: 45, Validation: "{}", Options: "{}"},
		// Zodiac - matches zodiac, cn_zodiac
		{Code: "zodiac", Name: "ปีนักษัตร", Description: "ปีชวด ฉลู ขาล...", Pattern: `(?i)(zodiac|^cn_zodiac$)`, InputType: "select", Priority: 30, Validation: "{}", Options: zodiacOptions},
		// Lunar month - matches luna, luna_m
		{Code: "lunar_month", Name: "เดือนจันทรคติ", Description: "เดือนอ้าย ยี่ สาม...", Pattern: `(?i)(luna|^luna_m$)`, InputType: "select", Priority: 25, Validation: "{}", Options: lunarMonthOptions},
		// Sex/Gender - matches sex, gender
		{Code: "sex", Name: "เพศ", Description: "เพศ ชาย/หญิง", Pattern: `(?i)(^sex$|^gender$)`, InputType: "select", Priority: 40, Validation: "{}", Options: `["ชาย","หญิง"]`},
		// Officer name - matches officer, registrar_name, issuer
		{Code: "officer_name", Name: "ชื่อเจ้าหน้าที่", Description: "ชื่อเจ้าหน้าที่/นายทะเบียน", Pattern: `(?i)(officer|registrar_name|issuer|issued_by)`, InputType: "select", Priority: 55, Validation: "{}", Options: `[]`},
	}
}

// DefaultInputTypes returns the default input types to initialize the database
func DefaultInputTypes() []InputType {
	return []InputType{
		{Code: "text", Name: "Text Input", Description: "ช่องกรอกข้อความ", Priority: 0},
		{Code: "select", Name: "Dropdown Select", Description: "ตัวเลือกแบบ dropdown", Priority: 10},
		{Code: "date", Name: "Date Picker", Description: "ตัวเลือกวันที่", Priority: 20},
		{Code: "time", Name: "Time Picker", Description: "ตัวเลือกเวลา", Priority: 30},
		{Code: "number", Name: "Number Input", Description: "ช่องกรอกตัวเลข", Priority: 40},
		{Code: "textarea", Name: "Text Area", Description: "ช่องกรอกข้อความหลายบรรทัด", Priority: 50},
		{Code: "checkbox", Name: "Checkbox", Description: "ช่องติ๊กถูก/ผิด", Priority: 55},
		{Code: "merged", Name: "Merged Fields", Description: "ช่องรวมหลายค่า", Priority: 60},
		{Code: "location", Name: "Location", Description: "Thai administrative boundary selection (ตำบล/อำเภอ/จังหวัด)", Priority: 65},
		{Code: "radio", Name: "Radio Button", Description: "ตัวเลือกแบบ radio button", Priority: 70},
		{Code: "digit", Name: "Digit Blocks", Description: "Digit block input with separators (OTP, License Plate)", Priority: 75},
	}
}
