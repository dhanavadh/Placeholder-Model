package storage

import (
	"context"
	"fmt"
	"io"
	"time"
)

// StorageClient is the interface for file storage operations
// Both GCS and Local storage implementations must implement this interface
type StorageClient interface {
	UploadFile(ctx context.Context, reader io.Reader, objectName, contentType string) (*UploadResult, error)
	DeleteFile(ctx context.Context, objectName string) error
	ReadFile(ctx context.Context, objectName string) (io.ReadCloser, error)
	GetSignedURL(objectName string, expiry time.Duration) (string, error)
	Close() error
}

// UploadResult contains the result of an upload operation
type UploadResult struct {
	ObjectName string `json:"object_name"`
	PublicURL  string `json:"public_url"`
	Size       int64  `json:"size"`
}

// Helper functions for generating object names
func GenerateObjectName(templateID, filename string) string {
	timestamp := time.Now().Unix()
	return fmt.Sprintf("templates/%s/%d_%s", templateID, timestamp, filename)
}

func GenerateDocumentObjectName(documentID, filename string) string {
	timestamp := time.Now().Unix()
	return fmt.Sprintf("documents/%s/%d_%s", documentID, timestamp, filename)
}

func GenerateDocumentPDFObjectName(documentID, filename string) string {
	timestamp := time.Now().Unix()
	pdfFilename := filename[:len(filename)-5] + ".pdf" // Replace .docx with .pdf
	return fmt.Sprintf("documents/%s/%d_%s", documentID, timestamp, pdfFilename)
}
