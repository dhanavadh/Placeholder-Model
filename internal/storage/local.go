package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// LocalStorageClient implements StorageClient interface for local filesystem storage
type LocalStorageClient struct {
	basePath  string // Base directory for file storage
	baseURL   string // Base URL for serving files (optional, for internal reference only)
	secretKey string // Secret key for signing URLs (optional)
}

// NewLocalStorageClient creates a new local storage client
// For internal-only deployments, baseURL can be empty as files are streamed through API
func NewLocalStorageClient(basePath, baseURL, secretKey string) (*LocalStorageClient, error) {
	// Create base directory if it doesn't exist
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Use a default secret key if not provided
	if secretKey == "" {
		secretKey = "default-local-storage-key"
	}

	// Default baseURL to internal reference if not provided
	if baseURL == "" {
		baseURL = "internal://storage"
	}

	return &LocalStorageClient{
		basePath:  basePath,
		baseURL:   baseURL,
		secretKey: secretKey,
	}, nil
}

// UploadFile uploads a file to the local filesystem
func (l *LocalStorageClient) UploadFile(ctx context.Context, reader io.Reader, objectName, contentType string) (*UploadResult, error) {
	// Create full path for the file
	fullPath := filepath.Join(l.basePath, objectName)

	// Create directory structure if it doesn't exist
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Create the file
	file, err := os.Create(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file %s: %w", fullPath, err)
	}
	defer file.Close()

	// Copy data to file
	size, err := io.Copy(file, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to write data to file: %w", err)
	}

	// Generate public URL
	publicURL := fmt.Sprintf("%s/%s", l.baseURL, objectName)

	return &UploadResult{
		ObjectName: objectName,
		PublicURL:  publicURL,
		Size:       size,
	}, nil
}

// DeleteFile deletes a file from the local filesystem
func (l *LocalStorageClient) DeleteFile(ctx context.Context, objectName string) error {
	fullPath := filepath.Join(l.basePath, objectName)

	// Check if file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		// File doesn't exist, return nil (no error)
		return nil
	}

	// Delete the file
	if err := os.Remove(fullPath); err != nil {
		return fmt.Errorf("failed to delete file %s: %w", fullPath, err)
	}

	// Try to clean up empty parent directories
	l.cleanEmptyDirs(filepath.Dir(fullPath))

	return nil
}

// cleanEmptyDirs removes empty parent directories up to basePath
func (l *LocalStorageClient) cleanEmptyDirs(dir string) {
	for dir != l.basePath && dir != "" {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		os.Remove(dir)
		dir = filepath.Dir(dir)
	}
}

// ReadFile reads a file from the local filesystem
func (l *LocalStorageClient) ReadFile(ctx context.Context, objectName string) (io.ReadCloser, error) {
	fullPath := filepath.Join(l.basePath, objectName)

	file, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", fullPath, err)
	}

	return file, nil
}

// GetSignedURL generates a signed URL for temporary access
// For local storage, we generate a URL with an expiration timestamp and signature
func (l *LocalStorageClient) GetSignedURL(objectName string, expiry time.Duration) (string, error) {
	// Calculate expiration timestamp
	expiresAt := time.Now().Add(expiry).Unix()

	// Create signature
	message := fmt.Sprintf("%s:%d", objectName, expiresAt)
	signature := l.sign(message)

	// Generate signed URL
	signedURL := fmt.Sprintf("%s/%s?expires=%d&signature=%s",
		l.baseURL, objectName, expiresAt, signature)

	return signedURL, nil
}

// sign creates an HMAC signature for the given message
func (l *LocalStorageClient) sign(message string) string {
	h := hmac.New(sha256.New, []byte(l.secretKey))
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifySignedURL verifies that a signed URL is valid and not expired
func (l *LocalStorageClient) VerifySignedURL(objectName string, expiresAt int64, signature string) bool {
	// Check if URL is expired
	if time.Now().Unix() > expiresAt {
		return false
	}

	// Verify signature
	message := fmt.Sprintf("%s:%d", objectName, expiresAt)
	expectedSignature := l.sign(message)

	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

// GetFilePath returns the full filesystem path for an object
func (l *LocalStorageClient) GetFilePath(objectName string) string {
	return filepath.Join(l.basePath, objectName)
}

// GetBasePath returns the base storage directory path
func (l *LocalStorageClient) GetBasePath() string {
	return l.basePath
}

// Close closes the local storage client (no-op for local storage)
func (l *LocalStorageClient) Close() error {
	return nil
}

// Ensure LocalStorageClient implements StorageClient interface
var _ StorageClient = (*LocalStorageClient)(nil)
