package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"DF-PLCH/internal"
	"DF-PLCH/internal/config"
	"DF-PLCH/internal/handlers"
	"DF-PLCH/internal/services"
	"DF-PLCH/internal/storage"

	"github.com/gin-contrib/cors"
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

	// Initialize GCS client
	ctx := context.Background()
	gcsClient, err := storage.NewGCSClient(ctx, cfg.GCS.BucketName, cfg.GCS.ProjectID, cfg.GCS.CredentialsPath)
	if err != nil {
		log.Fatalf("Failed to initialize GCS client: %v", err)
	}
	defer gcsClient.Close()

	// Initialize services
	templateService := services.NewTemplateService(gcsClient)

	// Initialize PDF service with configurable timeout
	pdfService, err := services.NewPDFService(cfg.Gotenberg.URL, cfg.Gotenberg.Timeout)
	if err != nil {
		log.Printf("Warning: Failed to initialize PDF service: %v", err)
		pdfService = nil // Continue without PDF service
	} else {
		log.Printf("PDF service initialized with URL: %s, timeout: %s", cfg.Gotenberg.URL, cfg.Gotenberg.Timeout)
	}

	documentService := services.NewDocumentService(gcsClient, templateService, pdfService)
	activityLogService := services.NewActivityLogService()

	// Initialize handlers
	docxHandler := handlers.NewDocxHandler(templateService, documentService)
	logsHandler := handlers.NewLogsHandler(activityLogService)

	// Initialize Gin router
	r := gin.Default()

	// Activity logging middleware (add before other middlewares)
	r.Use(activityLogService.LoggingMiddleware())

	// CORS middleware
	corsConfig := cors.DefaultConfig()

	// Handle wildcard configuration
	if slices.Contains(cfg.Server.AllowOrigins, "*") {
		corsConfig.AllowAllOrigins = true
	} else {
		// Prepare allowed origins with development localhost support
		allowedOrigins := make([]string, 0, len(cfg.Server.AllowOrigins))

		// Add configured origins
		for _, origin := range cfg.Server.AllowOrigins {
			if origin != "" {
				allowedOrigins = append(allowedOrigins, origin)
			}
		}

		// Add localhost origins in development mode
		if cfg.Server.Environment == "development" {
			localhostOrigins := []string{
				"http://localhost:3001",
				"http://localhost:3001",
				"http://localhost:5173",
				"http://localhost:8080",
			}

			// Only add localhost origins that aren't already in the list
			for _, localhost := range localhostOrigins {
				if !slices.Contains(allowedOrigins, localhost) {
					allowedOrigins = append(allowedOrigins, localhost)
				}
			}
		}

		corsConfig.AllowOrigins = allowedOrigins
	}

	corsConfig.AllowCredentials = true
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Content-Type", "Authorization"}

	r.Use(cors.New(corsConfig))

	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"version":   "2.0.0-cloud",
		})
	})

	// API v1 routes
	v1 := r.Group("/api/v1")
	{
		// Template management
		v1.POST("/upload", docxHandler.UploadTemplate)
		v1.GET("/templates", docxHandler.GetAllTemplates)
		v1.GET("/templates/:templateId/placeholders", docxHandler.GetPlaceholders)
		v1.GET("/templates/:templateId/positions", docxHandler.GetPlaceholderPositions)
		v1.PUT("/templates/:templateId", docxHandler.UpdateTemplate)
		v1.DELETE("/templates/:templateId", docxHandler.DeleteTemplate)

		// Document processing and download
		v1.POST("/templates/:templateId/process", docxHandler.ProcessDocument)
		v1.GET("/documents/:documentId/download", docxHandler.DownloadDocument)

		// Activity logs
		v1.GET("/logs", logsHandler.GetAllLogs)
		v1.GET("/logs/stats", logsHandler.GetLogStats)
		v1.GET("/logs/process", logsHandler.GetProcessLogs)
		v1.GET("/logs/debug", logsHandler.GetAllLogsDebug)

		// Simple history endpoint
		v1.GET("/history", logsHandler.GetHistory)
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
