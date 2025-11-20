package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Server    ServerConfig    `json:"server"`
	Database  DatabaseConfig  `json:"database"`
	GCS       GCSConfig       `json:"gcs"`
	Gotenberg GotenbergConfig `json:"gotenberg"`
}

type ServerConfig struct {
	Port         string   `json:"port"`
	Environment  string   `json:"environment"`
	BaseURL      string   `json:"base_url"`
	AllowOrigins []string `json:"allow_origins"`
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

	config := &Config{
		Server: ServerConfig{
			Port:         getEnv("SERVER_PORT", "8081"),
			Environment:  getEnv("ENVIRONMENT", "development"),
			BaseURL:      getEnv("BASE_URL", ""),
			AllowOrigins: parseAllowOrigins(),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", ""),
			DBName:   getEnv("DB_NAME", "df_plch"),
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
	}

	return config, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseAllowOrigins() []string {
	// First try to get from ALLOW_ORIGINS (comma-separated)
	if origins := os.Getenv("ALLOW_ORIGINS"); origins != "" {
		// Split by comma and trim whitespace
		var allowOrigins []string
		for _, origin := range strings.Split(origins, ",") {
			if trimmed := strings.TrimSpace(origin); trimmed != "" {
				allowOrigins = append(allowOrigins, trimmed)
			}
		}
		return allowOrigins
	}

	// Fallback to individual FRONTEND_URL_* variables for backward compatibility
	var allowOrigins []string

	if url1 := getEnv("FRONTEND_URL_1", ""); url1 != "" {
		allowOrigins = append(allowOrigins, url1)
	}

	if url2 := getEnv("FRONTEND_URL_2", ""); url2 != "" {
		allowOrigins = append(allowOrigins, url2)
	}

	// Default origins if none specified
	if len(allowOrigins) == 0 {
		allowOrigins = []string{
			"http://localhost:3000",
			"http://localhost:3002",
		}
	}

	return allowOrigins
}
