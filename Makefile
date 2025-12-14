# DF-PLCH Makefile
# DOCX Placeholder Processing Service

.PHONY: help start build clean test lint deps watch status endpoints

# Default target
help:
	@echo "DF-PLCH - DOCX Placeholder Processing Service"
	@echo ""
	@echo "Local Development:"
	@echo "  make start      - Start server locally"
	@echo "  make build      - Build the application"
	@echo "  make test       - Run tests"
	@echo "  make lint       - Run linter"
	@echo "  make clean      - Clean build artifacts"
	@echo "  make deps       - Install dependencies"
	@echo "  make watch      - Start with hot reload (requires air)"
	@echo ""
	@echo "Note: For Docker deployment, use parent Makefile (cd .. && make help)"

# Start server
start:
	@echo "Starting DF-PLCH server..."
	@echo "Server will be available at http://localhost:8081"
	@echo ""
	cd cmd/server && go run .

# Build the application
build:
	@echo "Building DF-PLCH..."
	cd cmd/server && go build -o ../../bin/df-plch .
	@echo "Build complete: bin/df-plch"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	go clean ./...

# Run tests
test:
	@echo "Running tests..."
	go test ./...

# Run linter
lint:
	@echo "Running linter..."
	go fmt ./...
	go vet ./...

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Development server with hot reload (requires air)
watch:
	@echo "Starting server with hot reload..."
	@echo "Note: Requires 'air' to be installed (go install github.com/cosmtrek/air@latest)"
	cd cmd/server && air

# Quick status check
status:
	@echo "DF-PLCH Service Status"
	@echo "======================"
	@curl -s http://localhost:8081/health | jq . 2>/dev/null || echo "Server not running or health endpoint unavailable"

# Show server endpoints
endpoints:
	@echo "DF-PLCH API Endpoints"
	@echo "===================="
	@echo "Health Check:"
	@echo "  GET  /health"
	@echo ""
	@echo "Template Management:"
	@echo "  POST /api/v1/upload"
	@echo "  GET  /api/v1/templates/{templateId}/placeholders"
	@echo ""
	@echo "Document Processing:"
	@echo "  POST /api/v1/templates/{templateId}/process"
	@echo "  GET  /api/v1/documents/{documentId}/download"
