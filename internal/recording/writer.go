package recording

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/ivfwriter"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
)

// TrackKind represents the type of media track.
type TrackKind string

const (
	// TrackKindAudio represents an audio track.
	TrackKindAudio TrackKind = "audio"
	// TrackKindVideo represents a video track.
	TrackKindVideo TrackKind = "video"
)

// MediaWriter handles writing media samples to files.
type MediaWriter interface {
	// WriteRTP writes an RTP packet to the media file.
	WriteRTP(packet *rtp.Packet) error
	// Close closes the writer and finalizes the file.
	Close() error
	// FilePath returns the path to the output file.
	FilePath() string
}

// TrackWriter writes media samples for a single track.
type TrackWriter struct {
	mu       sync.Mutex
	filePath string
	writer   media.Writer
	kind     TrackKind
	codec    string
	closed   bool
}

// WriterConfig contains configuration for creating a writer.
type WriterConfig struct {
	OutputDir string
	TrackID   string
	Kind      TrackKind
	Codec     string
}

// NewTrackWriter creates a new TrackWriter for the specified track.
func NewTrackWriter(cfg WriterConfig) (*TrackWriter, error) {
	// Ensure output directory exists
	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	var writer media.Writer
	var filePath string
	var err error

	switch cfg.Kind {
	case TrackKindVideo:
		// Use IVF format for VP8/VP9 video
		filePath = filepath.Join(cfg.OutputDir, fmt.Sprintf("%s.ivf", cfg.TrackID))
		writer, err = ivfwriter.New(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to create IVF writer: %w", err)
		}

	case TrackKindAudio:
		// Use OGG format for Opus audio
		filePath = filepath.Join(cfg.OutputDir, fmt.Sprintf("%s.ogg", cfg.TrackID))
		writer, err = oggwriter.New(filePath, 48000, 2)
		if err != nil {
			return nil, fmt.Errorf("failed to create OGG writer: %w", err)
		}

	default:
		return nil, fmt.Errorf("unsupported track kind: %s", cfg.Kind)
	}

	return &TrackWriter{
		filePath: filePath,
		writer:   writer,
		kind:     cfg.Kind,
		codec:    cfg.Codec,
	}, nil
}

// WriteRTP writes an RTP packet to the media file.
func (w *TrackWriter) WriteRTP(packet *rtp.Packet) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return ErrWriterClosed
	}

	if err := w.writer.WriteRTP(packet); err != nil {
		return fmt.Errorf("failed to write RTP packet: %w", err)
	}

	return nil
}

// Close closes the writer and finalizes the file.
func (w *TrackWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}

	w.closed = true
	if err := w.writer.Close(); err != nil {
		return fmt.Errorf("failed to close writer: %w", err)
	}

	return nil
}

// FilePath returns the path to the output file.
func (w *TrackWriter) FilePath() string {
	return w.filePath
}

// Kind returns the track kind.
func (w *TrackWriter) Kind() TrackKind {
	return w.kind
}

// Codec returns the codec name.
func (w *TrackWriter) Codec() string {
	return w.codec
}

// MultiTrackWriter manages multiple track writers for a recording session.
type MultiTrackWriter struct {
	mu                sync.RWMutex
	outputDir         string
	writers           map[string]*TrackWriter
	removedTrackFiles map[string]string // trackID -> filePath for removed tracks
	startTime         time.Time
	closed            bool
}

// NewMultiTrackWriter creates a new MultiTrackWriter.
func NewMultiTrackWriter(outputDir string) (*MultiTrackWriter, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	return &MultiTrackWriter{
		outputDir:         outputDir,
		writers:           make(map[string]*TrackWriter),
		removedTrackFiles: make(map[string]string),
		startTime:         time.Now(),
	}, nil
}

// AddTrack adds a new track writer.
func (m *MultiTrackWriter) AddTrack(trackID string, kind TrackKind, codec string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrWriterClosed
	}

	if _, exists := m.writers[trackID]; exists {
		return fmt.Errorf("track already exists: %s", trackID)
	}

	writer, err := NewTrackWriter(WriterConfig{
		OutputDir: m.outputDir,
		TrackID:   trackID,
		Kind:      kind,
		Codec:     codec,
	})
	if err != nil {
		return err
	}

	m.writers[trackID] = writer
	return nil
}

// WriteRTP writes an RTP packet to the appropriate track writer.
func (m *MultiTrackWriter) WriteRTP(trackID string, packet *rtp.Packet) error {
	m.mu.RLock()
	writer, exists := m.writers[trackID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("track not found: %s", trackID)
	}

	return writer.WriteRTP(packet)
}

// RemoveTrack removes and closes a track writer.
// The file path is preserved in removedTrackFiles regardless of close errors
// to ensure the track file can still be uploaded.
func (m *MultiTrackWriter) RemoveTrack(trackID string) error {
	m.mu.Lock()
	writer, exists := m.writers[trackID]
	if exists {
		delete(m.writers, trackID)
		// Save file path before closing - this ensures the path is preserved
		// even if Close() fails, since the file may still contain valid data
		m.removedTrackFiles[trackID] = writer.FilePath()
	}
	m.mu.Unlock()

	if !exists {
		return fmt.Errorf("track not found: %s", trackID)
	}

	return writer.Close()
}

// Close closes all track writers.
func (m *MultiTrackWriter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true

	var errs []error
	for trackID, writer := range m.writers {
		if err := writer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close track %s: %w", trackID, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing writers: %v", errs)
	}

	return nil
}

// GetTrackFiles returns a map of track IDs to their file paths (including removed tracks).
func (m *MultiTrackWriter) GetTrackFiles() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	files := make(map[string]string, len(m.writers)+len(m.removedTrackFiles))
	// Include active tracks
	for trackID, writer := range m.writers {
		files[trackID] = writer.FilePath()
	}
	// Include removed tracks (preserved for upload)
	for trackID, filePath := range m.removedTrackFiles {
		files[trackID] = filePath
	}
	return files
}

// GetTrackFilePath returns the file path for a specific track.
func (m *MultiTrackWriter) GetTrackFilePath(trackID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if writer, exists := m.writers[trackID]; exists {
		return writer.FilePath()
	}
	if filePath, exists := m.removedTrackFiles[trackID]; exists {
		return filePath
	}
	return ""
}

// AddRemovedTrackFile stores the file path for a removed track.
func (m *MultiTrackWriter) AddRemovedTrackFile(trackID, filePath string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removedTrackFiles[trackID] = filePath
}

// OutputDir returns the output directory.
func (m *MultiTrackWriter) OutputDir() string {
	return m.outputDir
}

// StartTime returns the recording start time.
func (m *MultiTrackWriter) StartTime() time.Time {
	return m.startTime
}

// Duration returns the recording duration since start.
func (m *MultiTrackWriter) Duration() time.Duration {
	return time.Since(m.startTime)
}

// FileWriter wraps an io.WriteCloser for use with recording.
type FileWriter struct {
	file     *os.File
	filePath string
}

// NewFileWriter creates a new FileWriter.
func NewFileWriter(filePath string) (*FileWriter, error) {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	return &FileWriter{
		file:     file,
		filePath: filePath,
	}, nil
}

// Write writes data to the file.
func (w *FileWriter) Write(p []byte) (n int, err error) {
	return w.file.Write(p)
}

// Close closes the file.
func (w *FileWriter) Close() error {
	return w.file.Close()
}

// FilePath returns the file path.
func (w *FileWriter) FilePath() string {
	return w.filePath
}

// Reader returns an io.Reader for the file.
func (w *FileWriter) Reader() (io.ReadCloser, error) {
	return os.Open(w.filePath)
}

// Size returns the current file size.
func (w *FileWriter) Size() (int64, error) {
	info, err := w.file.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// ErrWriterClosed is returned when trying to write to a closed writer.
var ErrWriterClosed = fmt.Errorf("writer is closed")
