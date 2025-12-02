package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"DF-PLCH/internal"
	"DF-PLCH/internal/config"
	"DF-PLCH/internal/handlers"
	"DF-PLCH/internal/services"
	"DF-PLCH/internal/storage"

	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Set Gin mode based on environment
	if cfg.Server.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize database
	if err := internal.InitDB(cfg); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Initialize storage client based on configuration
	ctx := context.Background()
	var storageClient storage.StorageClient
	var localStorageClient *storage.LocalStorageClient

	switch cfg.Storage.Type {
	case "local":
		log.Printf("Initializing local storage at: %s", cfg.Storage.LocalPath)
		client, err := storage.NewLocalStorageClient(cfg.Storage.LocalPath, cfg.Storage.LocalURL, cfg.Storage.SecretKey)
		if err != nil {
			log.Fatalf("Failed to initialize local storage client: %v", err)
		}
		storageClient = client
		localStorageClient = client
		log.Printf("Local storage initialized with base URL: %s", cfg.Storage.LocalURL)
	case "gcs":
		fallthrough
	default:
		log.Printf("Initializing GCS storage with bucket: %s", cfg.GCS.BucketName)
		client, err := storage.NewGCSClient(ctx, cfg.GCS.BucketName, cfg.GCS.ProjectID, cfg.GCS.CredentialsPath)
		if err != nil {
			log.Fatalf("Failed to initialize GCS client: %v", err)
		}
		storageClient = client
		log.Printf("GCS storage initialized")
	}
	defer storageClient.Close()

	// Initialize services
	templateService := services.NewTemplateService(storageClient)

	// Initialize PDF service with configurable timeout
	pdfService, err := services.NewPDFService(cfg.Gotenberg.URL, cfg.Gotenberg.Timeout)
	if err != nil {
		log.Printf("Warning: Failed to initialize PDF service: %v", err)
		pdfService = nil // Continue without PDF service
	} else {
		log.Printf("PDF service initialized with URL: %s, timeout: %s", cfg.Gotenberg.URL, cfg.Gotenberg.Timeout)
	}

	documentService := services.NewDocumentService(storageClient, templateService, pdfService)
	activityLogService := services.NewActivityLogService()
	fieldRuleService := services.NewFieldRuleService()
	entityRuleService := services.NewEntityRuleService()
	dataTypeService := services.NewDataTypeService()
	inputTypeService := services.NewInputTypeService()

	// Initialize default field rules if none exist
	if err := fieldRuleService.InitializeDefaultRules(); err != nil {
		log.Printf("Warning: Failed to initialize default field rules: %v", err)
	}

	// Initialize default entity rules if none exist
	if err := entityRuleService.InitializeDefaultRules(); err != nil {
		log.Printf("Warning: Failed to initialize default entity rules: %v", err)
	}

	// Initialize default data types if none exist
	if err := dataTypeService.InitializeDefaultDataTypes(); err != nil {
		log.Printf("Warning: Failed to initialize default data types: %v", err)
	}

	// Initialize default input types if none exist
	if err := inputTypeService.InitializeDefaultInputTypes(); err != nil {
		log.Printf("Warning: Failed to initialize default input types: %v", err)
	}

	// Initialize handlers
	docxHandler := handlers.NewDocxHandler(templateService, documentService)
	// Set storage info based on storage type
	if cfg.Storage.Type == "local" {
		docxHandler.SetStorageInfo(cfg.Storage.LocalPath)
	} else {
		docxHandler.SetStorageInfo(cfg.GCS.BucketName)
	}
	logsHandler := handlers.NewLogsHandler(activityLogService)
	fieldRuleHandler := handlers.NewFieldRuleHandler(fieldRuleService)
	entityRuleHandler := handlers.NewEntityRuleHandler(entityRuleService)
	dataTypeHandler := handlers.NewDataTypeHandler(dataTypeService, inputTypeService)
	ocrHandler := handlers.NewOCRHandler()

	// Initialize Gin router
	r := gin.Default()

	// Activity logging middleware
	r.Use(activityLogService.LoggingMiddleware())

	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"version":   "2.0.0-cloud",
			"storage":   cfg.Storage.Type,
		})
	})

	// Local file server endpoint (only for local storage with public URL configured)
	// For internal-only deployments, files are streamed through /documents/:id/download endpoint
	if localStorageClient != nil && cfg.Storage.LocalURL != "" && cfg.Storage.LocalURL != "internal://storage" {
		r.GET("/files/*filepath", func(c *gin.Context) {
			filePath := c.Param("filepath")
			if filePath == "" || filePath == "/" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "file path required"})
				return
			}

			// Remove leading slash
			if filePath[0] == '/' {
				filePath = filePath[1:]
			}

			// Security: Reject path traversal attempts
			cleanPath := filepath.Clean(filePath)
			if strings.Contains(cleanPath, "..") || strings.HasPrefix(cleanPath, "/") || strings.HasPrefix(cleanPath, "\\") {
				c.JSON(http.StatusForbidden, gin.H{"error": "invalid file path"})
				return
			}

			// Security: Always require signed URLs for file access
			expiresStr := c.Query("expires")
			signature := c.Query("signature")

			if signature == "" || expiresStr == "" {
				c.JSON(http.StatusForbidden, gin.H{"error": "signed URL required"})
				return
			}

			var expiresAt int64
			if _, err := fmt.Sscanf(expiresStr, "%d", &expiresAt); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid expires parameter"})
				return
			}

			if !localStorageClient.VerifySignedURL(cleanPath, expiresAt, signature) {
				c.JSON(http.StatusForbidden, gin.H{"error": "invalid or expired signature"})
				return
			}

			// Security: Verify the resolved path is within storage directory
			fullPath := localStorageClient.GetFilePath(cleanPath)
			absPath, err := filepath.Abs(fullPath)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve path"})
				return
			}
			basePath, err := filepath.Abs(localStorageClient.GetBasePath())
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve base path"})
				return
			}
			if !strings.HasPrefix(absPath, basePath+string(filepath.Separator)) {
				c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
				return
			}

			c.File(fullPath)
		})
		log.Printf("Local file server enabled at /files/*")
	} else if localStorageClient != nil {
		log.Printf("Local storage in internal-only mode - files served via /documents/:id/download")
	}

	// API v1 routes
	v1 := r.Group("/api/v1")
	{
		// Template management
		v1.POST("/upload", docxHandler.UploadTemplate)
		v1.GET("/templates", docxHandler.GetAllTemplates)
		v1.GET("/templates/:templateId/placeholders", docxHandler.GetPlaceholders)
		v1.GET("/templates/:templateId/preview", docxHandler.GetHTMLPreview)
		v1.PUT("/templates/:templateId", docxHandler.UpdateTemplate)
		v1.DELETE("/templates/:templateId", docxHandler.DeleteTemplate)
		v1.POST("/templates/:templateId/files", docxHandler.ReplaceTemplateFiles)

		// Field definitions (auto-detected from placeholders)
		v1.GET("/templates/:templateId/field-definitions", docxHandler.GetFieldDefinitions)
		v1.PUT("/templates/:templateId/field-definitions", docxHandler.UpdateFieldDefinitions)
		v1.POST("/templates/:templateId/field-definitions/regenerate", docxHandler.RegenerateFieldDefinitions)

		// Document processing and download
		v1.POST("/templates/:templateId/process", docxHandler.ProcessDocument)
		v1.GET("/documents/:documentId/download", docxHandler.DownloadDocument)

		// User document history
		v1.GET("/documents/history", docxHandler.GetUserDocumentHistory)

		// Regenerate document from history
		v1.POST("/documents/:documentId/regenerate", docxHandler.RegenerateDocument)

		// Activity logs
		v1.GET("/logs", logsHandler.GetAllLogs)
		v1.GET("/logs/stats", logsHandler.GetLogStats)
		v1.GET("/logs/process", logsHandler.GetProcessLogs)
		v1.GET("/logs/debug", logsHandler.GetAllLogsDebug)

		// Simple history endpoint
		v1.GET("/history", logsHandler.GetHistory)

		// Field detection rules
		v1.GET("/field-rules", fieldRuleHandler.GetAllRules)
		v1.GET("/field-rules/:ruleId", fieldRuleHandler.GetRule)
		v1.POST("/field-rules", fieldRuleHandler.CreateRule)
		v1.PUT("/field-rules/:ruleId", fieldRuleHandler.UpdateRule)
		v1.DELETE("/field-rules/:ruleId", fieldRuleHandler.DeleteRule)
		v1.POST("/field-rules/test", fieldRuleHandler.TestRule)
		v1.POST("/field-rules/initialize", fieldRuleHandler.InitializeDefaultRules)
		v1.GET("/field-rules/data-types", fieldRuleHandler.GetDataTypes)
		v1.GET("/field-rules/input-types", fieldRuleHandler.GetInputTypes)

		// Entity detection rules
		v1.GET("/entity-rules", entityRuleHandler.GetAllRules)
		v1.GET("/entity-rules/:ruleId", entityRuleHandler.GetRule)
		v1.POST("/entity-rules", entityRuleHandler.CreateRule)
		v1.PUT("/entity-rules/:ruleId", entityRuleHandler.UpdateRule)
		v1.DELETE("/entity-rules/:ruleId", entityRuleHandler.DeleteRule)
		v1.POST("/entity-rules/initialize", entityRuleHandler.InitializeDefaultRules)
		v1.GET("/entity-rules/labels", entityRuleHandler.GetEntityLabels)
		v1.GET("/entity-rules/colors", entityRuleHandler.GetEntityColors)

		// Data types management
		v1.GET("/data-types", dataTypeHandler.GetAllDataTypes)
		v1.GET("/data-types/:id", dataTypeHandler.GetDataType)
		v1.POST("/data-types", dataTypeHandler.CreateDataType)
		v1.PUT("/data-types/:id", dataTypeHandler.UpdateDataType)
		v1.DELETE("/data-types/:id", dataTypeHandler.DeleteDataType)
		v1.POST("/data-types/initialize", dataTypeHandler.InitializeDefaultDataTypes)

		// Input types management
		v1.GET("/input-types", dataTypeHandler.GetAllInputTypes)
		v1.GET("/input-types/:id", dataTypeHandler.GetInputType)
		v1.POST("/input-types", dataTypeHandler.CreateInputType)
		v1.PUT("/input-types/:id", dataTypeHandler.UpdateInputType)
		v1.DELETE("/input-types/:id", dataTypeHandler.DeleteInputType)
		v1.POST("/input-types/initialize", dataTypeHandler.InitializeDefaultInputTypes)

		// OCR endpoints
		v1.POST("/ocr/extract", ocrHandler.ExtractText)
		v1.POST("/templates/:templateId/ocr", ocrHandler.ExtractForTemplate)
	}

	// Create HTTP server with increased timeouts for document processing
	srv := &http.Server{
		Addr:         fmt.Sprintf("0.0.0.0:%s", cfg.Server.Port), // Listen on all interfaces for Cloud Run
		Handler:      r,
		ReadTimeout:  60 * time.Second,  // Increased from 30s
		WriteTimeout: 150 * time.Second, // Increased from 30s to handle PDF conversion
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Starting server on port %s (environment: %s)", cfg.Server.Port, cfg.Server.Environment)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Graceful shutdown handling
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Give outstanding requests a deadline for completion
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown server
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	// Close database connection
	if err := internal.CloseDB(); err != nil {
		log.Printf("Error closing database: %v", err)
	}

	// Close PDF service
	if pdfService != nil {
		if err := pdfService.Close(); err != nil {
			log.Printf("Error closing PDF service: %v", err)
		}
	}

	log.Println("Server exited")
}
