package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LocalStorage implements Storage interface for local filesystem storage.
// This is primarily intended for development and testing purposes.
type LocalStorage struct {
	basePath string
}

// NewLocalStorage creates a new LocalStorage instance.
func NewLocalStorage(basePath string) (*LocalStorage, error) {
	// Ensure base path exists
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}
	return &LocalStorage{basePath: basePath}, nil
}

// Upload uploads a file to local storage.
func (s *LocalStorage) Upload(ctx context.Context, key string, reader io.Reader, metadata *FileMetadata) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	fullPath := filepath.Join(s.basePath, key)

	// Create directory structure
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create file
	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	// Copy data
	if _, err := io.Copy(file, reader); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	// Save metadata if provided
	if metadata != nil {
		if err := s.saveMetadata(fullPath, metadata); err != nil {
			return fmt.Errorf("failed to save metadata: %w", err)
		}
	}

	return nil
}

// Download downloads a file from local storage.
func (s *LocalStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	fullPath := filepath.Join(s.basePath, key)
	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	return file, nil
}

// Delete deletes a file from local storage.
func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	fullPath := filepath.Join(s.basePath, key)

	// Delete metadata file if exists
	metaPath := fullPath + ".meta.json"
	_ = os.Remove(metaPath)

	// Delete main file
	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return fmt.Errorf("failed to delete file: %w", err)
	}
	return nil
}

// Exists checks if a file exists in local storage.
func (s *LocalStorage) Exists(ctx context.Context, key string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	fullPath := filepath.Join(s.basePath, key)
	_, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat file: %w", err)
	}
	return true, nil
}

// GetMetadata retrieves metadata for a file.
func (s *LocalStorage) GetMetadata(ctx context.Context, key string) (*FileMetadata, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	fullPath := filepath.Join(s.basePath, key)
	return s.loadMetadata(fullPath)
}

// List lists files with the given prefix.
func (s *LocalStorage) List(ctx context.Context, prefix string) ([]FileInfo, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	searchPath := filepath.Join(s.basePath, prefix)
	var files []FileInfo

	err := filepath.Walk(s.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Skip metadata files
		if strings.HasSuffix(path, ".meta.json") {
			return nil
		}

		// Check if path matches prefix
		relPath, err := filepath.Rel(s.basePath, path)
		if err != nil {
			return err
		}

		if strings.HasPrefix(relPath, prefix) || strings.HasPrefix(path, searchPath) {
			files = append(files, FileInfo{
				Key:          relPath,
				Size:         info.Size(),
				LastModified: info.ModTime(),
			})
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	return files, nil
}

// GetSignedURL returns a file:// URL for local storage.
// In production, this should be replaced with a proper signed URL implementation.
func (s *LocalStorage) GetSignedURL(ctx context.Context, key string, _ time.Duration) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	fullPath := filepath.Join(s.basePath, key)
	if _, err := os.Stat(fullPath); err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("failed to stat file: %w", err)
	}

	// Return file:// URL for local development
	return "file://" + fullPath, nil
}

// saveMetadata saves file metadata to a sidecar file.
func (s *LocalStorage) saveMetadata(filePath string, metadata *FileMetadata) error {
	metaPath := filePath + ".meta.json"
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, data, 0644)
}

// loadMetadata loads file metadata from a sidecar file.
func (s *LocalStorage) loadMetadata(filePath string) (*FileMetadata, error) {
	metaPath := filePath + ".meta.json"
	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return basic metadata from file info if metadata file doesn't exist
			info, statErr := os.Stat(filePath)
			if statErr != nil {
				if os.IsNotExist(statErr) {
					return nil, ErrNotFound
				}
				return nil, statErr
			}
			return &FileMetadata{
				Size: info.Size(),
			}, nil
		}
		return nil, err
	}

	var metadata FileMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}
	return &metadata, nil
}

// ErrNotFound is returned when a file is not found.
var ErrNotFound = errors.New("file not found")
