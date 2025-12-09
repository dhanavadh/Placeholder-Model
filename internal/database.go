package internal

import (
	"fmt"

	"DF-PLCH/internal/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB(cfg *config.Config) error {
	dsn := cfg.Database.DSN()

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Auto-migrate the schema
	if err := autoMigrate(); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	fmt.Println("Database connected and migrated successfully")
	return nil
}

func autoMigrate() error {
	// Create tables only if they don't exist (preserve existing data)
	fmt.Println("Ensuring document_templates table exists...")
	if DB.Migrator().HasTable("templates") && !DB.Migrator().HasTable("document_templates") {
		fmt.Println("Renaming legacy templates table to document_templates...")
		if err := DB.Exec("ALTER TABLE templates RENAME TO document_templates").Error; err != nil {
			return fmt.Errorf("failed to rename templates table: %w", err)
		}
	}

	result := DB.Exec(`
        CREATE TABLE IF NOT EXISTS document_templates (
            id varchar(191) PRIMARY KEY,
            filename text NOT NULL,
            original_name text,
            display_name text,
            description text,
            author text,
            gcs_path_docx text,
            file_size bigint,
            mime_type text,
            placeholders jsonb,
            created_at timestamp(3) NULL,
            updated_at timestamp(3) NULL,
            deleted_at timestamp(3) NULL
        )
    `);
	if result.Error != nil {
		return fmt.Errorf("failed to create document_templates table: %w", result.Error)
	}

	// Create index if not exists
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_document_templates_deleted_at ON document_templates(deleted_at)")

	// Handle legacy gcs_path migration for document_templates table
	var gcsPathExists bool
	err := DB.Raw("SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'document_templates' AND column_name = 'gcs_path'").Scan(&gcsPathExists).Error
	if err == nil && gcsPathExists {
		fmt.Println("Migrating legacy gcs_path column in document_templates...")

		// First ensure gcs_path_docx column exists
		if err := DB.Exec("ALTER TABLE document_templates ADD COLUMN gcs_path_docx longtext").Error; err != nil {
			// Column might already exist, that's OK
		}

		// Migrate data from gcs_path to gcs_path_docx
		if err := DB.Exec(`UPDATE document_templates SET gcs_path_docx = gcs_path WHERE gcs_path_docx IS NULL AND gcs_path IS NOT NULL`).Error; err != nil {
			return fmt.Errorf("failed to migrate gcs_path to gcs_path_docx in templates: %w", err)
		}

		// Drop the legacy column
		fmt.Println("Dropping legacy gcs_path column from document_templates...")
		if err := DB.Exec(`ALTER TABLE document_templates DROP COLUMN gcs_path`).Error; err != nil {
			fmt.Printf("Warning: failed to drop gcs_path column from templates: %v\n", err)
		}
	}

	// Drop positions column if it exists
	if DB.Migrator().HasColumn("document_templates", "positions") {
		fmt.Println("Dropping positions column from document_templates...")
		if err := DB.Exec("ALTER TABLE document_templates DROP COLUMN positions").Error; err != nil {
			fmt.Printf("Warning: failed to drop positions column: %v\n", err)
		}
	}

	ensureDocumentTemplateColumns := map[string]string{
		"filename":          "ALTER TABLE document_templates ADD COLUMN filename text",
		"original_name":     "ALTER TABLE document_templates ADD COLUMN original_name text",
		"display_name":      "ALTER TABLE document_templates ADD COLUMN display_name text",
		"name":              "ALTER TABLE document_templates ADD COLUMN name text",
		"description":       "ALTER TABLE document_templates ADD COLUMN description text",
		"author":            "ALTER TABLE document_templates ADD COLUMN author text",
		"category":          "ALTER TABLE document_templates ADD COLUMN category varchar(50)",
		"gcs_path_docx":     "ALTER TABLE document_templates ADD COLUMN gcs_path_docx text",
		"file_size":         "ALTER TABLE document_templates ADD COLUMN file_size bigint",
		"mime_type":         "ALTER TABLE document_templates ADD COLUMN mime_type text",
		"placeholders":      "ALTER TABLE document_templates ADD COLUMN placeholders jsonb",
		"aliases":           "ALTER TABLE document_templates ADD COLUMN aliases jsonb",
		"field_definitions": "ALTER TABLE document_templates ADD COLUMN field_definitions jsonb",
		"gcs_path_html":     "ALTER TABLE document_templates ADD COLUMN gcs_path_html text",
		"original_source":   "ALTER TABLE document_templates ADD COLUMN original_source text",
		"remarks":           "ALTER TABLE document_templates ADD COLUMN remarks text",
		"is_verified":       "ALTER TABLE document_templates ADD COLUMN is_verified boolean DEFAULT false",
		"is_ai_available":   "ALTER TABLE document_templates ADD COLUMN is_ai_available boolean DEFAULT false",
		"type":              "ALTER TABLE document_templates ADD COLUMN type varchar(20)",
		"tier":              "ALTER TABLE document_templates ADD COLUMN tier varchar(20)",
		"group":             "ALTER TABLE document_templates ADD COLUMN \"group\" text",
		"created_at":        "ALTER TABLE document_templates ADD COLUMN created_at timestamp(3) NULL",
		"updated_at":        "ALTER TABLE document_templates ADD COLUMN updated_at timestamp(3) NULL",
		"deleted_at":        "ALTER TABLE document_templates ADD COLUMN deleted_at timestamp(3) NULL",
	}

	for column, stmt := range ensureDocumentTemplateColumns {
		if err := ensureColumn("document_templates", column, stmt); err != nil {
			return err
		}
	}

	fmt.Println("Creating documents table if not exists...")
	result = DB.Exec(`
        CREATE TABLE IF NOT EXISTS documents (
            id varchar(191) PRIMARY KEY,
            template_id varchar(191) NOT NULL,
            filename text NOT NULL,
            gcs_path_docx text,
            gcs_path_pdf text,
            file_size bigint,
            mime_type text,
            data jsonb,
            status varchar(191) DEFAULT 'completed',
            created_at timestamp(3) NULL,
            updated_at timestamp(3) NULL,
            deleted_at timestamp(3) NULL
        )
    `)
	if result.Error != nil {
		return fmt.Errorf("failed to create documents table: %w", result.Error)
	}

	// Create indexes if not exists
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_documents_template_id ON documents(template_id)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_documents_deleted_at ON documents(deleted_at)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_documents_user_id ON documents(user_id)")

	// Handle legacy gcs_path column first
	if DB.Migrator().HasColumn("documents", "gcs_path") {
		fmt.Println("Found legacy gcs_path column, migrating...")

		// First ensure gcs_path_docx exists
		if !DB.Migrator().HasColumn("documents", "gcs_path_docx") {
			if err := DB.Exec("ALTER TABLE documents ADD COLUMN gcs_path_docx longtext").Error; err != nil {
				return fmt.Errorf("failed to add gcs_path_docx column: %w", err)
			}
		}

		// Migrate data from gcs_path to gcs_path_docx
		if err := DB.Exec(`UPDATE documents SET gcs_path_docx = gcs_path WHERE gcs_path_docx IS NULL AND gcs_path IS NOT NULL`).Error; err != nil {
			return fmt.Errorf("failed to migrate gcs_path to gcs_path_docx: %w", err)
		}

		// Drop the legacy column
		fmt.Println("Dropping legacy gcs_path column...")
		if err := DB.Exec(`ALTER TABLE documents DROP COLUMN gcs_path`).Error; err != nil {
			fmt.Printf("Warning: failed to drop gcs_path column: %v\n", err)
		}
	}

	ensureDocumentsColumns := map[string]string{
		"filename":      "ALTER TABLE documents ADD COLUMN filename text",
		"user_id":       "ALTER TABLE documents ADD COLUMN user_id varchar(191)",
		"gcs_path_docx": "ALTER TABLE documents ADD COLUMN gcs_path_docx text",
		"gcs_path_pdf":  "ALTER TABLE documents ADD COLUMN gcs_path_pdf text",
		"file_size":     "ALTER TABLE documents ADD COLUMN file_size bigint",
		"mime_type":     "ALTER TABLE documents ADD COLUMN mime_type text",
		"data":          "ALTER TABLE documents ADD COLUMN data jsonb",
		"status":        "ALTER TABLE documents ADD COLUMN status varchar(191) DEFAULT 'completed'",
		"created_at":    "ALTER TABLE documents ADD COLUMN created_at timestamp(3) NULL",
		"updated_at":    "ALTER TABLE documents ADD COLUMN updated_at timestamp(3) NULL",
		"deleted_at":    "ALTER TABLE documents ADD COLUMN deleted_at timestamp(3) NULL",
	}

	for column, stmt := range ensureDocumentsColumns {
		if err := ensureColumn("documents", column, stmt); err != nil {
			return err
		}
	}

	fmt.Println("Creating activity_logs table if not exists...")
	result = DB.Exec(`
        CREATE TABLE IF NOT EXISTS activity_logs (
            id varchar(191) PRIMARY KEY,
            method varchar(10) NOT NULL,
            path varchar(255) NOT NULL,
            user_agent text,
            ip_address varchar(45),
            request_body text,
            query_params text,
            status_code int NOT NULL,
            response_time bigint NOT NULL,
            user_id varchar(36),
            user_email varchar(255),
            created_at timestamp(3) NULL,
            updated_at timestamp(3) NULL,
            deleted_at timestamp(3) NULL
        )
    `)
	if result.Error != nil {
		return fmt.Errorf("failed to create activity_logs table: %w", result.Error)
	}

	// Create indexes if not exists
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_activity_logs_deleted_at ON activity_logs(deleted_at)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_activity_logs_method ON activity_logs(method)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_activity_logs_path ON activity_logs(path)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_activity_logs_created_at ON activity_logs(created_at)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_activity_logs_user_id ON activity_logs(user_id)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_activity_logs_user_email ON activity_logs(user_email)")

	// Ensure user columns exist in activity_logs for existing tables
	ensureActivityLogsColumns := map[string]string{
		"user_id":    "ALTER TABLE activity_logs ADD COLUMN user_id varchar(36)",
		"user_email": "ALTER TABLE activity_logs ADD COLUMN user_email varchar(255)",
	}

	for column, stmt := range ensureActivityLogsColumns {
		if err := ensureColumn("activity_logs", column, stmt); err != nil {
			return err
		}
	}

	// Indexes already created above, no need for duplicate code

	// Create field_rules table
	fmt.Println("Creating field_rules table if not exists...")
	result = DB.Exec(`
        CREATE TABLE IF NOT EXISTS field_rules (
            id varchar(191) PRIMARY KEY,
            name text NOT NULL,
            description text,
            pattern text NOT NULL,
            priority int DEFAULT 0,
            is_active boolean DEFAULT true,
            data_type varchar(50),
            input_type varchar(50),
            group_name text,
            validation jsonb,
            options jsonb,
            created_at timestamp(3) NULL,
            updated_at timestamp(3) NULL,
            deleted_at timestamp(3) NULL
        )
    `)
	if result.Error != nil {
		return fmt.Errorf("failed to create field_rules table: %w", result.Error)
	}

	// Create indexes for field_rules
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_field_rules_deleted_at ON field_rules(deleted_at)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_field_rules_priority ON field_rules(priority DESC)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_field_rules_is_active ON field_rules(is_active)")

	// Add entity column to field_rules if not exists
	ensureFieldRulesColumns := map[string]string{
		"entity": "ALTER TABLE field_rules ADD COLUMN entity varchar(50)",
	}

	for column, stmt := range ensureFieldRulesColumns {
		if err := ensureColumn("field_rules", column, stmt); err != nil {
			return err
		}
	}

	// Create entity_rules table
	fmt.Println("Creating entity_rules table if not exists...")
	result = DB.Exec(`
        CREATE TABLE IF NOT EXISTS entity_rules (
            id varchar(191) PRIMARY KEY,
            name text NOT NULL,
            code varchar(50) NOT NULL UNIQUE,
            description text,
            pattern text NOT NULL,
            priority int DEFAULT 0,
            is_active boolean DEFAULT true,
            color varchar(20),
            icon varchar(50),
            created_at timestamp(3) NULL,
            updated_at timestamp(3) NULL,
            deleted_at timestamp(3) NULL
        )
    `)
	if result.Error != nil {
		return fmt.Errorf("failed to create entity_rules table: %w", result.Error)
	}

	// Create indexes for entity_rules
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_entity_rules_deleted_at ON entity_rules(deleted_at)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_entity_rules_priority ON entity_rules(priority DESC)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_entity_rules_is_active ON entity_rules(is_active)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_entity_rules_code ON entity_rules(code)")

	// Create data_types table for configurable data types
	fmt.Println("Creating data_types table if not exists...")
	result = DB.Exec(`
        CREATE TABLE IF NOT EXISTS data_types (
            id varchar(191) PRIMARY KEY,
            code varchar(50) NOT NULL UNIQUE,
            name text NOT NULL,
            description text,
            pattern text,
            input_type varchar(50) DEFAULT 'text',
            validation jsonb,
            options jsonb,
            priority int DEFAULT 0,
            is_active boolean DEFAULT true,
            created_at timestamp(3) NULL,
            updated_at timestamp(3) NULL,
            deleted_at timestamp(3) NULL
        )
    `)
	if result.Error != nil {
		return fmt.Errorf("failed to create data_types table: %w", result.Error)
	}

	// Add pattern column if it doesn't exist (migration for existing tables)
	DB.Exec("ALTER TABLE data_types ADD COLUMN IF NOT EXISTS pattern text")
	// Add default_value column if it doesn't exist (migration for existing tables)
	DB.Exec("ALTER TABLE data_types ADD COLUMN IF NOT EXISTS default_value text")

	// Create indexes for data_types
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_data_types_deleted_at ON data_types(deleted_at)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_data_types_code ON data_types(code)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_data_types_is_active ON data_types(is_active)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_data_types_priority ON data_types(priority DESC)")

	// Create input_types table for configurable input types
	fmt.Println("Creating input_types table if not exists...")
	result = DB.Exec(`
        CREATE TABLE IF NOT EXISTS input_types (
            id varchar(191) PRIMARY KEY,
            code varchar(50) NOT NULL UNIQUE,
            name text NOT NULL,
            description text,
            priority int DEFAULT 0,
            is_active boolean DEFAULT true,
            created_at timestamp(3) NULL,
            updated_at timestamp(3) NULL,
            deleted_at timestamp(3) NULL
        )
    `)
	if result.Error != nil {
		return fmt.Errorf("failed to create input_types table: %w", result.Error)
	}

	// Create indexes for input_types
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_input_types_deleted_at ON input_types(deleted_at)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_input_types_code ON input_types(code)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_input_types_is_active ON input_types(is_active)")

	// Create statistics table for tracking form submissions, exports, etc.
	fmt.Println("Creating statistics table if not exists...")
	result = DB.Exec(`
        CREATE TABLE IF NOT EXISTS statistics (
            id varchar(36) PRIMARY KEY,
            event_type varchar(50) NOT NULL,
            template_id varchar(191),
            date date NOT NULL,
            count bigint NOT NULL DEFAULT 0,
            created_at timestamp(3) NULL,
            updated_at timestamp(3) NULL,
            deleted_at timestamp(3) NULL
        )
    `)
	if result.Error != nil {
		return fmt.Errorf("failed to create statistics table: %w", result.Error)
	}

	// Create indexes for statistics
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_statistics_deleted_at ON statistics(deleted_at)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_statistics_event_type ON statistics(event_type)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_statistics_template_id ON statistics(template_id)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_statistics_date ON statistics(date)")
	// Composite index for efficient lookups
	DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_statistics_unique ON statistics(event_type, template_id, date) WHERE deleted_at IS NULL")

	// Create document_types table for grouping related templates
	fmt.Println("Creating document_types table if not exists...")
	result = DB.Exec(`
        CREATE TABLE IF NOT EXISTS document_types (
            id varchar(191) PRIMARY KEY,
            code varchar(50) NOT NULL UNIQUE,
            name text NOT NULL,
            name_en text,
            description text,
            category varchar(50),
            icon text,
            color varchar(20),
            sort_order int DEFAULT 0,
            is_active boolean DEFAULT true,
            metadata jsonb,
            created_at timestamp(3) NULL,
            updated_at timestamp(3) NULL,
            deleted_at timestamp(3) NULL
        )
    `)
	if result.Error != nil {
		return fmt.Errorf("failed to create document_types table: %w", result.Error)
	}

	// Create indexes for document_types
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_document_types_deleted_at ON document_types(deleted_at)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_document_types_code ON document_types(code)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_document_types_category ON document_types(category)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_document_types_is_active ON document_types(is_active)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_document_types_sort_order ON document_types(sort_order)")

	// Add document_type_id and variant columns to document_templates if not exists
	ensureDocumentTemplateNewColumns := map[string]string{
		"document_type_id": "ALTER TABLE document_templates ADD COLUMN document_type_id varchar(191)",
		"variant_name":     "ALTER TABLE document_templates ADD COLUMN variant_name text",
		"variant_order":    "ALTER TABLE document_templates ADD COLUMN variant_order int DEFAULT 0",
	}

	for column, stmt := range ensureDocumentTemplateNewColumns {
		if err := ensureColumn("document_templates", column, stmt); err != nil {
			return err
		}
	}

	// Create index for document_type_id in document_templates
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_document_templates_document_type_id ON document_templates(document_type_id)")

	// Create filter_categories table for configurable filter groups
	fmt.Println("Creating filter_categories table if not exists...")
	result = DB.Exec(`
        CREATE TABLE IF NOT EXISTS filter_categories (
            id varchar(191) PRIMARY KEY,
            code varchar(50) NOT NULL UNIQUE,
            name text NOT NULL,
            name_en text,
            description text,
            field_name varchar(50) NOT NULL,
            sort_order int DEFAULT 0,
            is_active boolean DEFAULT true,
            is_system boolean DEFAULT false,
            created_at timestamp(3) NULL,
            updated_at timestamp(3) NULL,
            deleted_at timestamp(3) NULL
        )
    `)
	if result.Error != nil {
		return fmt.Errorf("failed to create filter_categories table: %w", result.Error)
	}

	// Create indexes for filter_categories
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_filter_categories_deleted_at ON filter_categories(deleted_at)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_filter_categories_code ON filter_categories(code)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_filter_categories_is_active ON filter_categories(is_active)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_filter_categories_sort_order ON filter_categories(sort_order)")

	// Create filter_options table for options within filter categories
	fmt.Println("Creating filter_options table if not exists...")
	result = DB.Exec(`
        CREATE TABLE IF NOT EXISTS filter_options (
            id varchar(191) PRIMARY KEY,
            filter_category_id varchar(191) NOT NULL,
            value varchar(100) NOT NULL,
            label text NOT NULL,
            label_en text,
            description text,
            color varchar(20),
            icon varchar(50),
            sort_order int DEFAULT 0,
            is_active boolean DEFAULT true,
            is_default boolean DEFAULT false,
            created_at timestamp(3) NULL,
            updated_at timestamp(3) NULL,
            deleted_at timestamp(3) NULL
        )
    `)
	if result.Error != nil {
		return fmt.Errorf("failed to create filter_options table: %w", result.Error)
	}

	// Create indexes for filter_options
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_filter_options_deleted_at ON filter_options(deleted_at)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_filter_options_filter_category_id ON filter_options(filter_category_id)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_filter_options_value ON filter_options(value)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_filter_options_is_active ON filter_options(is_active)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_filter_options_sort_order ON filter_options(sort_order)")

	fmt.Println("Tables created/verified successfully")
	return nil
}

func ensureColumn(table, column, statement string) error {
	// Check if column exists (handle quoted column names like "group")
	cleanColumn := column
	if column == "group" {
		// Special check for reserved keyword columns
		var exists bool
		err := DB.Raw("SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?)", table, column).Scan(&exists).Error
		if err != nil {
			return fmt.Errorf("failed to check column %s.%s: %w", table, column, err)
		}
		if exists {
			return nil
		}
	} else if DB.Migrator().HasColumn(table, cleanColumn) {
		return nil
	}

	fmt.Printf("Adding missing column %s.%s...\n", table, column)
	if err := DB.Exec(statement).Error; err != nil {
		return fmt.Errorf("failed to add column %s.%s: %w", table, column, err)
	}

	return nil
}

func CloseDB() error {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}
