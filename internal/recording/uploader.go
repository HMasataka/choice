package recording

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/HMasataka/choice/internal/recording/storage"
	"github.com/HMasataka/choice/pkg/logger"
)

// Uploader handles uploading recordings to storage.
type Uploader struct {
	storage     storage.Storage
	logger      *logger.Logger
	queue       chan *Recording
	wg          sync.WaitGroup
	done        chan struct{}
	shutdown    bool
	shutdownMu  sync.RWMutex
	maxRetries  int
	concurrency int
}

// UploaderConfig contains configuration for the uploader.
type UploaderConfig struct {
	Storage     storage.Storage
	Logger      *logger.Logger
	MaxRetries  int
	Concurrency int
}

// NewUploader creates a new Uploader.
func NewUploader(cfg UploaderConfig) *Uploader {
	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 2
	}

	u := &Uploader{
		storage:     cfg.Storage,
		logger:      cfg.Logger,
		queue:       make(chan *Recording, 100),
		done:        make(chan struct{}),
		maxRetries:  maxRetries,
		concurrency: concurrency,
	}

	// Start worker goroutines
	for i := 0; i < concurrency; i++ {
		u.wg.Add(1)
		go u.worker()
	}

	return u
}

// QueueRecording adds a recording to the upload queue.
// This method blocks if the queue is full until space is available or shutdown is called.
func (u *Uploader) QueueRecording(recording *Recording) {
	u.shutdownMu.RLock()
	isShutdown := u.shutdown
	u.shutdownMu.RUnlock()

	if isShutdown {
		if u.logger != nil {
			u.logger.Warn("uploader is shutdown, cannot queue recording",
				"recording_id", recording.ID)
		}
		return
	}

	// Block until we can queue or shutdown is signaled
	select {
	case u.queue <- recording:
		if u.logger != nil {
			u.logger.Info("recording queued for upload",
				"recording_id", recording.ID,
				"room_id", recording.RoomID)
		}
	case <-u.done:
		if u.logger != nil {
			u.logger.Warn("uploader shutdown while queueing recording",
				"recording_id", recording.ID)
		}
	}
}

// worker processes recordings from the queue.
func (u *Uploader) worker() {
	defer u.wg.Done()

	for {
		select {
		case recording := <-u.queue:
			if recording != nil {
				u.uploadRecording(recording)
			}
		case <-u.done:
			// Drain remaining items from the queue before exiting
			u.drainQueue()
			return
		}
	}
}

// drainQueue processes any remaining recordings in the queue.
func (u *Uploader) drainQueue() {
	for {
		select {
		case recording := <-u.queue:
			if recording != nil {
				u.uploadRecording(recording)
			}
		default:
			return
		}
	}
}

// uploadRecording uploads a single recording with retries.
func (u *Uploader) uploadRecording(recording *Recording) {
	ctx := context.Background()

	var lastErr error
	for attempt := 1; attempt <= u.maxRetries; attempt++ {
		if err := u.doUpload(ctx, recording); err != nil {
			lastErr = err
			if u.logger != nil {
				u.logger.Warn("upload attempt failed",
					"recording_id", recording.ID,
					"attempt", attempt,
					"max_retries", u.maxRetries,
					"error", err)
			}

			// Exponential backoff
			backoff := time.Duration(attempt*attempt) * time.Second
			select {
			case <-time.After(backoff):
			case <-u.done:
				return
			}
			continue
		}

		if u.logger != nil {
			u.logger.Info("recording uploaded successfully",
				"recording_id", recording.ID,
				"room_id", recording.RoomID)
		}
		return
	}

	if u.logger != nil {
		u.logger.Error("failed to upload recording after max retries",
			"recording_id", recording.ID,
			"error", lastErr)
	}
}

// doUpload performs the actual upload.
func (u *Uploader) doUpload(ctx context.Context, recording *Recording) error {
	basePath := fmt.Sprintf("recordings/%s/%s", recording.RoomID, recording.ID)

	// Upload each track file
	for trackID, filePath := range recording.TrackFiles {
		if err := u.uploadFile(ctx, filePath, basePath, trackID); err != nil {
			return fmt.Errorf("failed to upload track %s: %w", trackID, err)
		}
	}

	// Create and upload metadata
	metadata := u.buildMetadata(recording)
	if err := u.uploadMetadata(ctx, basePath, metadata); err != nil {
		return fmt.Errorf("failed to upload metadata: %w", err)
	}

	// Clean up local files after successful upload
	if err := u.cleanupLocal(recording); err != nil {
		if u.logger != nil {
			u.logger.Warn("failed to cleanup local files",
				"recording_id", recording.ID,
				"error", err)
		}
	}

	return nil
}

// uploadFile uploads a single file to storage.
func (u *Uploader) uploadFile(ctx context.Context, localPath, basePath, trackID string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	ext := filepath.Ext(localPath)
	key := fmt.Sprintf("%s/%s%s", basePath, trackID, ext)

	contentType := "application/octet-stream"
	switch ext {
	case ".ivf":
		contentType = "video/x-ivf"
	case ".ogg":
		contentType = "audio/ogg"
	case ".webm":
		contentType = "video/webm"
	}

	metadata := &storage.FileMetadata{
		ContentType: contentType,
		Size:        info.Size(),
	}

	return u.storage.Upload(ctx, key, file, metadata)
}

// uploadMetadata uploads the recording metadata as JSON.
func (u *Uploader) uploadMetadata(ctx context.Context, basePath string, metadata *storage.RecordingMetadata) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	key := fmt.Sprintf("%s/metadata.json", basePath)

	reader := &bytesReader{data: data}
	return u.storage.Upload(ctx, key, reader, &storage.FileMetadata{
		ContentType: "application/json",
		Size:        int64(len(data)),
	})
}

// buildMetadata builds storage metadata from a recording.
func (u *Uploader) buildMetadata(recording *Recording) *storage.RecordingMetadata {
	participants := make([]storage.ParticipantMetadata, 0, len(recording.Participants))
	for _, p := range recording.Participants {
		participants = append(participants, storage.ParticipantMetadata{
			ParticipantID:    p.ParticipantID,
			JoinedAt:         p.JoinedAt,
			LeftAt:           p.LeftAt,
			ConsentGiven:     p.ConsentGiven,
			ConsentTimestamp: p.ConsentTimestamp,
		})
	}

	tracks := make([]storage.TrackMetadata, 0, len(recording.Tracks))
	for _, t := range recording.Tracks {
		tracks = append(tracks, storage.TrackMetadata{
			TrackID:     t.TrackID,
			Kind:        string(t.Kind),
			PublisherID: t.PublisherID,
			Codec:       t.Codec,
		})
	}

	// Calculate total file size
	var totalSize int64
	for _, filePath := range recording.TrackFiles {
		if info, err := os.Stat(filePath); err == nil {
			totalSize += info.Size()
		}
	}

	// Format duration as ISO 8601 duration
	duration := formatISO8601Duration(recording.Duration)

	return &storage.RecordingMetadata{
		RecordingID:  recording.ID,
		RoomID:       recording.RoomID,
		StartedAt:    recording.StartedAt,
		StoppedAt:    recording.StoppedAt,
		Duration:     duration,
		StartedBy:    recording.StartedBy,
		StoppedBy:    recording.StoppedBy,
		Participants: participants,
		Tracks:       tracks,
		Format:       recording.Format,
		FileSize:     totalSize,
		Status:       storage.RecordingStatusCompleted,
	}
}

// cleanupLocal removes local recording files after upload.
func (u *Uploader) cleanupLocal(recording *Recording) error {
	// Remove the entire output directory
	return os.RemoveAll(recording.OutputDir)
}

// Shutdown stops the uploader and waits for pending uploads to complete.
func (u *Uploader) Shutdown(ctx context.Context) {
	u.shutdownMu.Lock()
	u.shutdown = true
	u.shutdownMu.Unlock()

	close(u.done)

	// Wait for workers with timeout
	done := make(chan struct{})
	go func() {
		u.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if u.logger != nil {
			u.logger.Info("uploader shutdown completed")
		}
	case <-ctx.Done():
		if u.logger != nil {
			u.logger.Warn("uploader shutdown timed out, some recordings may not be uploaded")
		}
	}
}

// formatISO8601Duration formats a duration as ISO 8601.
func formatISO8601Duration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("PT%dH%dM%dS", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("PT%dM%dS", minutes, seconds)
	}
	return fmt.Sprintf("PT%dS", seconds)
}

// bytesReader wraps a byte slice as an io.Reader.
type bytesReader struct {
	data []byte
	pos  int
}

func (r *bytesReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
