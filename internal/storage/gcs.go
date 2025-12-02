package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

// GCSClient implements StorageClient interface for Google Cloud Storage
type GCSClient struct {
	client     *storage.Client
	bucketName string
}

func NewGCSClient(ctx context.Context, bucketName, projectID, credentialsPath string) (*GCSClient, error) {
	var client *storage.Client
	var err error

	if credentialsPath != "" {
		client, err = storage.NewClient(ctx, option.WithCredentialsFile(credentialsPath))
	} else {
		client, err = storage.NewClient(ctx)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}

	return &GCSClient{
		client:     client,
		bucketName: bucketName,
	}, nil
}

func (g *GCSClient) UploadFile(ctx context.Context, reader io.Reader, objectName, contentType string) (*UploadResult, error) {
	obj := g.client.Bucket(g.bucketName).Object(objectName)
	writer := obj.NewWriter(ctx)

	if contentType != "" {
		writer.ContentType = contentType
	}

	size, err := io.Copy(writer, reader)
	if err != nil {
		writer.Close()
		return nil, fmt.Errorf("failed to copy data to GCS: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close GCS writer: %w", err)
	}

	return &UploadResult{
		ObjectName: objectName,
		PublicURL:  fmt.Sprintf("https://storage.googleapis.com/%s/%s", g.bucketName, objectName),
		Size:       size,
	}, nil
}

func (g *GCSClient) DeleteFile(ctx context.Context, objectName string) error {
	obj := g.client.Bucket(g.bucketName).Object(objectName)
	return obj.Delete(ctx)
}

func (g *GCSClient) ReadFile(ctx context.Context, objectName string) (io.ReadCloser, error) {
	obj := g.client.Bucket(g.bucketName).Object(objectName)
	return obj.NewReader(ctx)
}

func (g *GCSClient) GetSignedURL(objectName string, expiry time.Duration) (string, error) {
	opts := &storage.SignedURLOptions{
		Scheme:  storage.SigningSchemeV4,
		Method:  "GET",
		Expires: time.Now().Add(expiry),
	}

	return g.client.Bucket(g.bucketName).SignedURL(objectName, opts)
}

func (g *GCSClient) Close() error {
	return g.client.Close()
}

// Ensure GCSClient implements StorageClient interface
var _ StorageClient = (*GCSClient)(nil)