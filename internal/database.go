package internal

import (
	"fmt"

	"DF-PLCH/internal/config"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB(cfg *config.Config) error {
	dsn := cfg.Database.DSN()

	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
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
		if err := DB.Exec("RENAME TABLE templates TO document_templates").Error; err != nil {
			return fmt.Errorf("failed to rename templates table: %w", err)
		}
	}

	result := DB.Exec(`
        CREATE TABLE IF NOT EXISTS document_templates (
            id varchar(191) PRIMARY KEY,
            filename longtext NOT NULL,
            original_name longtext,
            display_name longtext,
            description longtext,
            author longtext,
            gcs_path_docx longtext,
            file_size bigint,
            mime_type longtext,
            placeholders json,
            positions json,
            created_at datetime(3) NULL,
            updated_at datetime(3) NULL,
            deleted_at datetime(3) NULL,
            INDEX idx_document_templates_deleted_at (deleted_at)
        )
    `)
	if result.Error != nil {
		return fmt.Errorf("failed to create document_templates table: %w", result.Error)
	}

	// Handle legacy gcs_path migration for document_templates table
	var gcsPathExists bool
	err := DB.Raw("SELECT count(*) FROM INFORMATION_SCHEMA.columns WHERE table_schema = DATABASE() AND table_name = 'document_templates' AND column_name = 'gcs_path'").Scan(&gcsPathExists).Error
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

	ensureDocumentTemplateColumns := map[string]string{
		"filename":      "ALTER TABLE document_templates ADD COLUMN filename longtext",
		"original_name": "ALTER TABLE document_templates ADD COLUMN original_name longtext",
		"display_name":  "ALTER TABLE document_templates ADD COLUMN display_name longtext",
		"description":   "ALTER TABLE document_templates ADD COLUMN description longtext",
		"author":        "ALTER TABLE document_templates ADD COLUMN author longtext",
		"gcs_path_docx": "ALTER TABLE document_templates ADD COLUMN gcs_path_docx longtext",
		"file_size":     "ALTER TABLE document_templates ADD COLUMN file_size bigint",
		"mime_type":     "ALTER TABLE document_templates ADD COLUMN mime_type longtext",
		"placeholders":  "ALTER TABLE document_templates ADD COLUMN placeholders json",
		"positions":     "ALTER TABLE document_templates ADD COLUMN positions json",
		"aliases":       "ALTER TABLE document_templates ADD COLUMN aliases json",
		"gcs_path_html": "ALTER TABLE document_templates ADD COLUMN gcs_path_html longtext",
		"created_at":    "ALTER TABLE document_templates ADD COLUMN created_at datetime(3) NULL",
		"updated_at":    "ALTER TABLE document_templates ADD COLUMN updated_at datetime(3) NULL",
		"deleted_at":    "ALTER TABLE document_templates ADD COLUMN deleted_at datetime(3) NULL",
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
            filename longtext NOT NULL,
            gcs_path_docx longtext,
            gcs_path_pdf longtext,
            file_size bigint,
            mime_type longtext,
            data json,
            status varchar(191) DEFAULT 'completed',
            created_at datetime(3) NULL,
            updated_at datetime(3) NULL,
            deleted_at datetime(3) NULL,
            INDEX idx_documents_template_id (template_id),
            INDEX idx_documents_deleted_at (deleted_at)
        )
    `)
	if result.Error != nil {
		return fmt.Errorf("failed to create documents table: %w", result.Error)
	}

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
		"filename":      "ALTER TABLE documents ADD COLUMN filename longtext",
		"gcs_path_docx": "ALTER TABLE documents ADD COLUMN gcs_path_docx longtext",
		"gcs_path_pdf":  "ALTER TABLE documents ADD COLUMN gcs_path_pdf longtext",
		"file_size":     "ALTER TABLE documents ADD COLUMN file_size bigint",
		"mime_type":     "ALTER TABLE documents ADD COLUMN mime_type longtext",
		"data":          "ALTER TABLE documents ADD COLUMN data json",
		"status":        "ALTER TABLE documents ADD COLUMN status varchar(191) DEFAULT 'completed'",
		"created_at":    "ALTER TABLE documents ADD COLUMN created_at datetime(3) NULL",
		"updated_at":    "ALTER TABLE documents ADD COLUMN updated_at datetime(3) NULL",
		"deleted_at":    "ALTER TABLE documents ADD COLUMN deleted_at datetime(3) NULL",
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
            created_at datetime(3) NULL,
            updated_at datetime(3) NULL,
            deleted_at datetime(3) NULL,
            INDEX idx_activity_logs_deleted_at (deleted_at),
            INDEX idx_activity_logs_method (method),
            INDEX idx_activity_logs_path (path),
            INDEX idx_activity_logs_created_at (created_at)
        )
    `)
	if result.Error != nil {
		return fmt.Errorf("failed to create activity_logs table: %w", result.Error)
	}

	fmt.Println("Tables created/verified successfully")
	return nil
}

func ensureColumn(table, column, statement string) error {
	if DB.Migrator().HasColumn(table, column) {
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
