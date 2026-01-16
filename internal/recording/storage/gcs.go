package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

// GCSStorage implements Storage interface for Google Cloud Storage.
type GCSStorage struct {
	client    *storage.Client
	bucket    string
	projectID string
}

// GCSConfig contains configuration for GCS storage.
type GCSConfig struct {
	Bucket    string
	ProjectID string
}

// NewGCSStorage creates a new GCSStorage instance.
func NewGCSStorage(ctx context.Context, cfg GCSConfig) (*GCSStorage, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}

	return &GCSStorage{
		client:    client,
		bucket:    cfg.Bucket,
		projectID: cfg.ProjectID,
	}, nil
}

// NewGCSStorageWithClient creates a new GCSStorage with an existing client.
// This is useful for testing with a mock client.
func NewGCSStorageWithClient(client *storage.Client, bucket string, projectID string) *GCSStorage {
	return &GCSStorage{
		client:    client,
		bucket:    bucket,
		projectID: projectID,
	}
}

// Upload uploads a file to GCS.
func (s *GCSStorage) Upload(ctx context.Context, key string, reader io.Reader, metadata *FileMetadata) error {
	obj := s.client.Bucket(s.bucket).Object(key)
	writer := obj.NewWriter(ctx)

	if metadata != nil {
		if metadata.ContentType != "" {
			writer.ContentType = metadata.ContentType
		}
		if metadata.CustomMeta != nil {
			writer.Metadata = metadata.CustomMeta
		}
	}

	if _, err := io.Copy(writer, reader); err != nil {
		closeErr := writer.Close()
		if closeErr != nil {
			return fmt.Errorf("failed to copy data to GCS: %w (close error: %v)", err, closeErr)
		}
		return fmt.Errorf("failed to copy data to GCS: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close GCS writer: %w", err)
	}

	return nil
}

// Download downloads a file from GCS.
func (s *GCSStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	obj := s.client.Bucket(s.bucket).Object(key)
	reader, err := obj.NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to create GCS reader: %w", err)
	}
	return reader, nil
}

// Delete deletes a file from GCS.
func (s *GCSStorage) Delete(ctx context.Context, key string) error {
	obj := s.client.Bucket(s.bucket).Object(key)
	if err := obj.Delete(ctx); err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return ErrNotFound
		}
		return fmt.Errorf("failed to delete GCS object: %w", err)
	}
	return nil
}

// Exists checks if a file exists in GCS.
func (s *GCSStorage) Exists(ctx context.Context, key string) (bool, error) {
	obj := s.client.Bucket(s.bucket).Object(key)
	_, err := obj.Attrs(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get GCS object attrs: %w", err)
	}
	return true, nil
}

// GetMetadata retrieves metadata for a file.
func (s *GCSStorage) GetMetadata(ctx context.Context, key string) (*FileMetadata, error) {
	obj := s.client.Bucket(s.bucket).Object(key)
	attrs, err := obj.Attrs(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get GCS object attrs: %w", err)
	}

	return &FileMetadata{
		ContentType: attrs.ContentType,
		Size:        attrs.Size,
		Checksum:    fmt.Sprintf("%x", attrs.MD5),
		CustomMeta:  attrs.Metadata,
	}, nil
}

// List lists files with the given prefix.
func (s *GCSStorage) List(ctx context.Context, prefix string) ([]FileInfo, error) {
	var files []FileInfo

	it := s.client.Bucket(s.bucket).Objects(ctx, &storage.Query{
		Prefix: prefix,
	})

	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate GCS objects: %w", err)
		}

		files = append(files, FileInfo{
			Key:          attrs.Name,
			Size:         attrs.Size,
			LastModified: attrs.Updated,
			ContentType:  attrs.ContentType,
		})
	}

	return files, nil
}

// GetSignedURL generates a signed URL for temporary access to a file.
func (s *GCSStorage) GetSignedURL(ctx context.Context, key string, expiration time.Duration) (string, error) {
	url, err := s.client.Bucket(s.bucket).SignedURL(key, &storage.SignedURLOptions{
		Method:  "GET",
		Expires: time.Now().Add(expiration),
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate signed URL: %w", err)
	}
	return url, nil
}

// Close closes the GCS client.
func (s *GCSStorage) Close() error {
	return s.client.Close()
}

// UploadRecordingMetadata uploads recording metadata as a JSON file.
func (s *GCSStorage) UploadRecordingMetadata(ctx context.Context, basePath string, metadata *RecordingMetadata) error {
	key := basePath + "/metadata.json"

	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	obj := s.client.Bucket(s.bucket).Object(key)
	writer := obj.NewWriter(ctx)
	writer.ContentType = "application/json"

	if _, err := writer.Write(data); err != nil {
		closeErr := writer.Close()
		if closeErr != nil {
			return fmt.Errorf("failed to write metadata: %w (close error: %v)", err, closeErr)
		}
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close writer: %w", err)
	}

	return nil
}

// GetRecordingMetadata downloads and parses recording metadata.
func (s *GCSStorage) GetRecordingMetadata(ctx context.Context, basePath string) (*RecordingMetadata, error) {
	key := basePath + "/metadata.json"

	reader, err := s.Download(ctx, key)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = reader.Close()
	}()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	var metadata RecordingMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &metadata, nil
}
