package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

type Config struct {
	Server      ServerConfig      `json:"server"`
	Database    DatabaseConfig    `json:"database"`
	Storage     StorageConfig     `json:"storage"`
	GCS         GCSConfig         `json:"gcs"`
	Gotenberg   GotenbergConfig   `json:"gotenberg"`
	LibreOffice LibreOfficeConfig `json:"libreoffice"`
}

type LibreOfficeConfig struct {
	Enabled bool   `json:"enabled"` // Enable LibreOffice-based DOCX processing for better format preservation
	Path    string `json:"path"`    // Path to LibreOffice executable (auto-detected if empty)
}

type StorageConfig struct {
	Type      string `json:"type"`       // "gcs" or "local"
	LocalPath string `json:"local_path"` // Path for local storage (e.g., "./storage")
	LocalURL  string `json:"local_url"`  // Base URL for local storage (e.g., "http://localhost:8081/files")
	SecretKey string `json:"secret_key"` // Secret key for signing local URLs
}

type ServerConfig struct {
	Port        string `json:"port"`
	Environment string `json:"environment"`
	BaseURL     string `json:"base_url"`
}

type DatabaseConfig struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	DBName   string `json:"db_name"`
}

type GCSConfig struct {
	BucketName      string `json:"bucket_name"`
	ProjectID       string `json:"project_id"`
	CredentialsPath string `json:"credentials_path"`
}

type GotenbergConfig struct {
	URL     string `json:"url"`
	Timeout string `json:"timeout"`
}

func (d *DatabaseConfig) DSN() string {
	// Cloud SQL Unix socket support
	if len(d.Host) > 0 && d.Host[0] == '/' {
		return fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=disable",
			d.Host, d.User, d.Password, d.DBName)
	}
	// Standard TCP connection
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		d.Host, d.Port, d.User, d.Password, d.DBName)
}

// findProjectRoot finds the project root by looking for go.mod file
func findProjectRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func Load() (*Config, error) {
	// Try to find and load .env from project root
	envPaths := []string{}

	if projectRoot := findProjectRoot(); projectRoot != "" {
		envPaths = append(envPaths, filepath.Join(projectRoot, ".env"))
	}

	// Fallback paths
	envPaths = append(envPaths, "../../.env", ".env")

	loaded := false
	for _, envPath := range envPaths {
		if err := godotenv.Load(envPath); err == nil {
			loaded = true
			break
		}
	}

	if !loaded {
		fmt.Printf("Failed to load .env file from any location, using system environment variables\n")
	}

	// Get storage type (default to "gcs" for backward compatibility)
	storageType := getEnv("STORAGE_TYPE", "gcs")

	config := &Config{
		Server: ServerConfig{
			Port:        getEnv("SERVER_PORT", "8081"),
			Environment: getEnv("ENVIRONMENT", "development"),
			BaseURL:     getEnv("BASE_URL", ""),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", ""),
			DBName:   getEnv("DB_NAME", "df_plch"),
		},
		Storage: StorageConfig{
			Type:      storageType,
			LocalPath: getEnv("STORAGE_LOCAL_PATH", "./storage"),
			LocalURL:  getEnv("STORAGE_LOCAL_URL", "http://localhost:8081/files"),
			SecretKey: getEnv("STORAGE_SECRET_KEY", ""),
		},
		GCS: GCSConfig{
			BucketName:      getEnv("GCS_BUCKET_NAME", ""),
			ProjectID:       getEnv("GOOGLE_CLOUD_PROJECT", ""),
			CredentialsPath: getEnv("GCS_CREDENTIALS_PATH", ""),
		},
		Gotenberg: GotenbergConfig{
			URL:     getEnv("GOTENBERG_URL", "http://localhost:3000"),
			Timeout: getEnv("GOTENBERG_TIMEOUT", "30s"), // Faster timeout for optimized Gotenberg
		},
		LibreOffice: LibreOfficeConfig{
			Enabled: getEnv("LIBREOFFICE_ENABLED", "false") == "true",
			Path:    getEnv("LIBREOFFICE_PATH", ""), // Auto-detected if empty
		},
	}

	return config, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
