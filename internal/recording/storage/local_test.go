package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalStorage_Upload(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewLocalStorage(tempDir)
	require.NoError(t, err)

	ctx := context.Background()
	key := "test/file.txt"
	content := []byte("hello world")

	err = storage.Upload(ctx, key, bytes.NewReader(content), &FileMetadata{
		ContentType: "text/plain",
		Size:        int64(len(content)),
	})
	require.NoError(t, err)

	// Verify file exists
	filePath := filepath.Join(tempDir, key)
	_, err = os.Stat(filePath)
	require.NoError(t, err)

	// Read and verify content
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestLocalStorage_Download(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewLocalStorage(tempDir)
	require.NoError(t, err)

	ctx := context.Background()
	key := "test/file.txt"
	content := []byte("download test content")

	// Upload first
	err = storage.Upload(ctx, key, bytes.NewReader(content), nil)
	require.NoError(t, err)

	// Download and verify
	reader, err := storage.Download(ctx, key)
	require.NoError(t, err)
	defer reader.Close()

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestLocalStorage_Download_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewLocalStorage(tempDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = storage.Download(ctx, "nonexistent/file.txt")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestLocalStorage_Delete(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewLocalStorage(tempDir)
	require.NoError(t, err)

	ctx := context.Background()
	key := "test/delete.txt"
	content := []byte("delete me")

	// Upload first
	err = storage.Upload(ctx, key, bytes.NewReader(content), nil)
	require.NoError(t, err)

	// Verify exists
	exists, err := storage.Exists(ctx, key)
	require.NoError(t, err)
	assert.True(t, exists)

	// Delete
	err = storage.Delete(ctx, key)
	require.NoError(t, err)

	// Verify deleted
	exists, err = storage.Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestLocalStorage_Delete_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewLocalStorage(tempDir)
	require.NoError(t, err)

	ctx := context.Background()
	err = storage.Delete(ctx, "nonexistent/file.txt")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestLocalStorage_Exists(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewLocalStorage(tempDir)
	require.NoError(t, err)

	ctx := context.Background()
	key := "test/exists.txt"

	// Should not exist initially
	exists, err := storage.Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, exists)

	// Upload
	err = storage.Upload(ctx, key, bytes.NewReader([]byte("content")), nil)
	require.NoError(t, err)

	// Should exist now
	exists, err = storage.Exists(ctx, key)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestLocalStorage_GetMetadata(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewLocalStorage(tempDir)
	require.NoError(t, err)

	ctx := context.Background()
	key := "test/metadata.txt"
	content := []byte("metadata test")
	metadata := &FileMetadata{
		ContentType: "text/plain",
		Size:        int64(len(content)),
		CustomMeta: map[string]string{
			"custom-key": "custom-value",
		},
	}

	// Upload with metadata
	err = storage.Upload(ctx, key, bytes.NewReader(content), metadata)
	require.NoError(t, err)

	// Get metadata
	gotMetadata, err := storage.GetMetadata(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, metadata.ContentType, gotMetadata.ContentType)
	assert.Equal(t, metadata.CustomMeta["custom-key"], gotMetadata.CustomMeta["custom-key"])
}

func TestLocalStorage_List(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewLocalStorage(tempDir)
	require.NoError(t, err)

	ctx := context.Background()

	// Upload multiple files
	files := map[string][]byte{
		"recordings/room1/file1.txt": []byte("content1"),
		"recordings/room1/file2.txt": []byte("content2"),
		"recordings/room2/file3.txt": []byte("content3"),
		"other/file.txt":             []byte("other"),
	}

	for key, content := range files {
		err = storage.Upload(ctx, key, bytes.NewReader(content), nil)
		require.NoError(t, err)
	}

	// List with prefix
	listed, err := storage.List(ctx, "recordings/room1")
	require.NoError(t, err)
	assert.Len(t, listed, 2)

	// Verify keys
	keys := make([]string, len(listed))
	for i, f := range listed {
		keys[i] = f.Key
	}
	assert.Contains(t, keys, "recordings/room1/file1.txt")
	assert.Contains(t, keys, "recordings/room1/file2.txt")
}

func TestLocalStorage_GetSignedURL(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewLocalStorage(tempDir)
	require.NoError(t, err)

	ctx := context.Background()
	key := "test/signed.txt"

	// Upload first
	err = storage.Upload(ctx, key, bytes.NewReader([]byte("content")), nil)
	require.NoError(t, err)

	// Get signed URL
	url, err := storage.GetSignedURL(ctx, key, time.Hour)
	require.NoError(t, err)
	assert.Contains(t, url, "file://")
	assert.Contains(t, url, key)
}

func TestLocalStorage_GetSignedURL_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewLocalStorage(tempDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = storage.GetSignedURL(ctx, "nonexistent.txt", time.Hour)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestLocalStorage_ContextCancellation(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewLocalStorage(tempDir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// All operations should return context error
	err = storage.Upload(ctx, "key", bytes.NewReader([]byte("data")), nil)
	assert.ErrorIs(t, err, context.Canceled)

	_, err = storage.Download(ctx, "key")
	assert.ErrorIs(t, err, context.Canceled)

	err = storage.Delete(ctx, "key")
	assert.ErrorIs(t, err, context.Canceled)

	_, err = storage.Exists(ctx, "key")
	assert.ErrorIs(t, err, context.Canceled)

	_, err = storage.List(ctx, "prefix")
	assert.ErrorIs(t, err, context.Canceled)
}
