package storage

import (
	"context"
	"io"
	"time"
)

// Storage defines the interface for recording storage backends.
type Storage interface {
	// Upload uploads a recording file to the storage backend.
	Upload(ctx context.Context, key string, reader io.Reader, metadata *FileMetadata) error

	// Download downloads a recording file from the storage backend.
	Download(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete deletes a recording file from the storage backend.
	Delete(ctx context.Context, key string) error

	// Exists checks if a file exists in the storage backend.
	Exists(ctx context.Context, key string) (bool, error)

	// GetMetadata retrieves metadata for a file.
	GetMetadata(ctx context.Context, key string) (*FileMetadata, error)

	// List lists files with the given prefix.
	List(ctx context.Context, prefix string) ([]FileInfo, error)

	// GetSignedURL generates a signed URL for temporary access to a file.
	GetSignedURL(ctx context.Context, key string, expiration time.Duration) (string, error)
}

// FileMetadata contains metadata for a recording file.
type FileMetadata struct {
	ContentType string
	Size        int64
	Checksum    string
	CustomMeta  map[string]string
}

// FileInfo contains information about a file in storage.
type FileInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
	ContentType  string
}

// RecordingMetadata contains metadata for a recording session.
type RecordingMetadata struct {
	RecordingID  string                 `json:"recordingId"`
	RoomID       string                 `json:"roomId"`
	StartedAt    time.Time              `json:"startedAt"`
	StoppedAt    *time.Time             `json:"stoppedAt,omitempty"`
	Duration     string                 `json:"duration,omitempty"`
	StartedBy    string                 `json:"startedBy"`
	StoppedBy    string                 `json:"stoppedBy,omitempty"`
	Participants []ParticipantMetadata  `json:"participants"`
	Tracks       []TrackMetadata        `json:"tracks"`
	Format       string                 `json:"format"`
	FileSize     int64                  `json:"fileSize,omitempty"`
	Status       RecordingStatus        `json:"status"`
	Extra        map[string]interface{} `json:"extra,omitempty"`
}

// ParticipantMetadata contains metadata about a participant in a recording.
type ParticipantMetadata struct {
	ParticipantID    string     `json:"participantId"`
	JoinedAt         time.Time  `json:"joinedAt"`
	LeftAt           *time.Time `json:"leftAt,omitempty"`
	ConsentGiven     bool       `json:"consentGiven"`
	ConsentTimestamp *time.Time `json:"consentTimestamp,omitempty"`
}

// TrackMetadata contains metadata about a track in a recording.
type TrackMetadata struct {
	TrackID     string `json:"trackId"`
	Kind        string `json:"kind"`
	PublisherID string `json:"publisherId"`
	Codec       string `json:"codec"`
}

// RecordingStatus represents the status of a recording.
type RecordingStatus string

const (
	// RecordingStatusPending indicates the recording is pending.
	RecordingStatusPending RecordingStatus = "pending"
	// RecordingStatusRecording indicates the recording is in progress.
	RecordingStatusRecording RecordingStatus = "recording"
	// RecordingStatusStopped indicates the recording has been stopped.
	RecordingStatusStopped RecordingStatus = "stopped"
	// RecordingStatusUploading indicates the recording is being uploaded.
	RecordingStatusUploading RecordingStatus = "uploading"
	// RecordingStatusCompleted indicates the recording is complete and uploaded.
	RecordingStatusCompleted RecordingStatus = "completed"
	// RecordingStatusFailed indicates the recording failed.
	RecordingStatusFailed RecordingStatus = "failed"
)
