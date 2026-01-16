package recording

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pion/rtp"

	"github.com/HMasataka/choice/internal/recording/storage"
	"github.com/HMasataka/choice/pkg/logger"
)

// Recorder manages recording sessions for a room.
type Recorder struct {
	mu            sync.RWMutex
	roomID        string
	recordingID   string
	status        storage.RecordingStatus
	startedAt     time.Time
	stoppedAt     *time.Time
	startedBy     string
	stoppedBy     string
	writer        *MultiTrackWriter
	tracks        map[string]*TrackInfo
	removedTracks map[string]*TrackInfo // Tracks removed during recording (still need upload)
	participants  map[string]*ParticipantInfo
	tempDir       string
	format        string
	logger        *logger.Logger
	onStopFn      func(recording *Recording)
}

// TrackInfo contains information about a track being recorded.
type TrackInfo struct {
	TrackID     string
	Kind        TrackKind
	Codec       string
	PublisherID string
	AddedAt     time.Time
}

// ParticipantInfo contains information about a participant in a recording.
type ParticipantInfo struct {
	ParticipantID    string
	JoinedAt         time.Time
	LeftAt           *time.Time
	ConsentGiven     bool
	ConsentTimestamp *time.Time
}

// Recording represents a completed recording.
type Recording struct {
	ID           string
	RoomID       string
	Status       storage.RecordingStatus
	StartedAt    time.Time
	StoppedAt    *time.Time
	Duration     time.Duration
	StartedBy    string
	StoppedBy    string
	Format       string
	OutputDir    string
	TrackFiles   map[string]string
	Tracks       []*TrackInfo
	Participants []*ParticipantInfo
}

// RecorderConfig contains configuration for a recorder.
type RecorderConfig struct {
	RoomID    string
	TempDir   string
	Format    string
	StartedBy string
	Logger    *logger.Logger
	OnStop    func(recording *Recording)
}

// NewRecorder creates a new Recorder for a room.
func NewRecorder(cfg RecorderConfig) (*Recorder, error) {
	recordingID := uuid.New().String()

	outputDir := filepath.Join(cfg.TempDir, cfg.RoomID, recordingID)
	writer, err := NewMultiTrackWriter(outputDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create multi-track writer: %w", err)
	}

	format := cfg.Format
	if format == "" {
		format = "webm"
	}

	return &Recorder{
		roomID:        cfg.RoomID,
		recordingID:   recordingID,
		status:        storage.RecordingStatusPending,
		startedBy:     cfg.StartedBy,
		writer:        writer,
		tracks:        make(map[string]*TrackInfo),
		removedTracks: make(map[string]*TrackInfo),
		participants:  make(map[string]*ParticipantInfo),
		tempDir:       cfg.TempDir,
		format:        format,
		logger:        cfg.Logger,
		onStopFn:      cfg.OnStop,
	}, nil
}

// Start starts the recording.
func (r *Recorder) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.status != storage.RecordingStatusPending {
		return ErrRecordingAlreadyStarted
	}

	r.status = storage.RecordingStatusRecording
	r.startedAt = time.Now()

	if r.logger != nil {
		r.logger.Info("recording started",
			"recording_id", r.recordingID,
			"room_id", r.roomID,
			"started_by", r.startedBy)
	}

	return nil
}

// Stop stops the recording.
// Note: Close errors are logged but not returned. The recording data is still
// returned to allow upload of any successfully written data. This prevents
// API 500 errors that would confuse clients when the upload actually succeeds.
func (r *Recorder) Stop(stoppedBy string) (*Recording, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.status != storage.RecordingStatusRecording {
		return nil, ErrRecordingNotStarted
	}

	now := time.Now()
	r.status = storage.RecordingStatusStopped
	r.stoppedAt = &now
	r.stoppedBy = stoppedBy

	// Close all writers - errors are logged but not returned
	// since the recording data may still be valid for upload
	if err := r.writer.Close(); err != nil {
		if r.logger != nil {
			r.logger.Warn("failed to close writer, recording will still be uploaded",
				"recording_id", r.recordingID,
				"error", err)
		}
	}

	recording := r.buildRecording()

	if r.logger != nil {
		r.logger.Info("recording stopped",
			"recording_id", r.recordingID,
			"room_id", r.roomID,
			"stopped_by", stoppedBy,
			"duration", recording.Duration)
	}

	// Call onStop callback to queue upload
	if r.onStopFn != nil {
		go r.onStopFn(recording)
	}

	return recording, nil
}

// AddTrack adds a track to the recording.
func (r *Recorder) AddTrack(trackID string, kind TrackKind, codec string, publisherID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.status != storage.RecordingStatusRecording {
		return ErrRecordingNotStarted
	}

	if _, exists := r.tracks[trackID]; exists {
		return fmt.Errorf("track already exists: %s", trackID)
	}

	// Add track to writer
	if err := r.writer.AddTrack(trackID, kind, codec); err != nil {
		return fmt.Errorf("failed to add track to writer: %w", err)
	}

	r.tracks[trackID] = &TrackInfo{
		TrackID:     trackID,
		Kind:        kind,
		Codec:       codec,
		PublisherID: publisherID,
		AddedAt:     time.Now(),
	}

	if r.logger != nil {
		r.logger.Debug("track added to recording",
			"recording_id", r.recordingID,
			"track_id", trackID,
			"kind", kind,
			"codec", codec)
	}

	return nil
}

// RemoveTrack removes a track from active recording but preserves it for upload.
func (r *Recorder) RemoveTrack(trackID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.status != storage.RecordingStatusRecording {
		return ErrRecordingNotStarted
	}

	trackInfo, exists := r.tracks[trackID]
	if !exists {
		return fmt.Errorf("track not found: %s", trackID)
	}

	// Remove track from writer (closes the writer for this track)
	// Note: MultiTrackWriter.RemoveTrack preserves the file path in removedTrackFiles
	// regardless of close errors, so the file will still be uploaded
	if err := r.writer.RemoveTrack(trackID); err != nil {
		// Log the error but don't fail - the file is already preserved for upload
		if r.logger != nil {
			r.logger.Warn("failed to close track writer, file will still be uploaded",
				"recording_id", r.recordingID,
				"track_id", trackID,
				"error", err)
		}
	}

	// Move to removedTracks to preserve for upload
	r.removedTracks[trackID] = trackInfo
	delete(r.tracks, trackID)

	if r.logger != nil {
		r.logger.Debug("track removed from recording (preserved for upload)",
			"recording_id", r.recordingID,
			"track_id", trackID)
	}

	return nil
}

// WriteRTP writes an RTP packet to the recording.
func (r *Recorder) WriteRTP(trackID string, packet *rtp.Packet) error {
	r.mu.RLock()
	if r.status != storage.RecordingStatusRecording {
		r.mu.RUnlock()
		return ErrRecordingNotStarted
	}
	r.mu.RUnlock()

	return r.writer.WriteRTP(trackID, packet)
}

// AddParticipant adds a participant to the recording.
func (r *Recorder) AddParticipant(participantID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.participants[participantID]; exists {
		return
	}

	r.participants[participantID] = &ParticipantInfo{
		ParticipantID: participantID,
		JoinedAt:      time.Now(),
	}
}

// RemoveParticipant marks a participant as having left the recording.
func (r *Recorder) RemoveParticipant(participantID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if info, exists := r.participants[participantID]; exists {
		now := time.Now()
		info.LeftAt = &now
	}
}

// SetParticipantConsent sets the consent status for a participant.
func (r *Recorder) SetParticipantConsent(participantID string, consented bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if info, exists := r.participants[participantID]; exists {
		info.ConsentGiven = consented
		now := time.Now()
		info.ConsentTimestamp = &now
	}
}

// ID returns the recording ID.
func (r *Recorder) ID() string {
	return r.recordingID
}

// RoomID returns the room ID.
func (r *Recorder) RoomID() string {
	return r.roomID
}

// Status returns the current recording status.
func (r *Recorder) Status() storage.RecordingStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.status
}

// StartedAt returns the recording start time.
func (r *Recorder) StartedAt() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.startedAt
}

// Duration returns the current recording duration.
func (r *Recorder) Duration() time.Duration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.status == storage.RecordingStatusPending {
		return 0
	}

	if r.stoppedAt != nil {
		return r.stoppedAt.Sub(r.startedAt)
	}

	return time.Since(r.startedAt)
}

// buildRecording builds a Recording from the current state.
func (r *Recorder) buildRecording() *Recording {
	tracks := make([]*TrackInfo, 0, len(r.tracks)+len(r.removedTracks))
	for _, t := range r.tracks {
		tracks = append(tracks, t)
	}
	// Include removed tracks in the recording metadata
	for _, t := range r.removedTracks {
		tracks = append(tracks, t)
	}

	participants := make([]*ParticipantInfo, 0, len(r.participants))
	for _, p := range r.participants {
		participants = append(participants, p)
	}

	var duration time.Duration
	if r.stoppedAt != nil {
		duration = r.stoppedAt.Sub(r.startedAt)
	}

	return &Recording{
		ID:           r.recordingID,
		RoomID:       r.roomID,
		Status:       r.status,
		StartedAt:    r.startedAt,
		StoppedAt:    r.stoppedAt,
		Duration:     duration,
		StartedBy:    r.startedBy,
		StoppedBy:    r.stoppedBy,
		Format:       r.format,
		OutputDir:    r.writer.OutputDir(),
		TrackFiles:   r.writer.GetTrackFiles(),
		Tracks:       tracks,
		Participants: participants,
	}
}

// GetRecording returns the current recording state.
func (r *Recorder) GetRecording() *Recording {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.buildRecording()
}

// RecordingService manages recordings for all rooms.
type RecordingService struct {
	mu        sync.RWMutex
	recorders map[string]*Recorder // roomID -> Recorder
	uploader  *Uploader
	tempDir   string
	format    string
	logger    *logger.Logger
	enabled   bool
}

// RecordingServiceConfig contains configuration for the recording service.
type RecordingServiceConfig struct {
	Enabled bool
	TempDir string
	Format  string
	Storage storage.Storage
	Logger  *logger.Logger
}

// NewRecordingService creates a new RecordingService.
func NewRecordingService(cfg RecordingServiceConfig) *RecordingService {
	var uploader *Uploader
	if cfg.Storage != nil {
		uploader = NewUploader(UploaderConfig{
			Storage:     cfg.Storage,
			Logger:      cfg.Logger,
			MaxRetries:  3,
			Concurrency: 2,
		})
	}

	return &RecordingService{
		recorders: make(map[string]*Recorder),
		uploader:  uploader,
		tempDir:   cfg.TempDir,
		format:    cfg.Format,
		logger:    cfg.Logger,
		enabled:   cfg.Enabled,
	}
}

// IsEnabled returns whether recording is enabled.
func (s *RecordingService) IsEnabled() bool {
	return s.enabled
}

// StartRecording starts recording for a room.
func (s *RecordingService) StartRecording(ctx context.Context, roomID string, startedBy string) (*Recording, error) {
	if !s.enabled {
		return nil, ErrRecordingDisabled
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if recording already exists for this room
	if _, exists := s.recorders[roomID]; exists {
		return nil, ErrRecordingAlreadyExists
	}

	// Create new recorder
	recorder, err := NewRecorder(RecorderConfig{
		RoomID:    roomID,
		TempDir:   s.tempDir,
		Format:    s.format,
		StartedBy: startedBy,
		Logger:    s.logger,
		OnStop:    s.onRecordingStopped,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create recorder: %w", err)
	}

	// Start recording
	if err := recorder.Start(); err != nil {
		return nil, fmt.Errorf("failed to start recording: %w", err)
	}

	s.recorders[roomID] = recorder

	return recorder.GetRecording(), nil
}

// StopRecording stops recording for a room.
func (s *RecordingService) StopRecording(ctx context.Context, roomID string, stoppedBy string) (*Recording, error) {
	s.mu.Lock()
	recorder, exists := s.recorders[roomID]
	if exists {
		delete(s.recorders, roomID)
	}
	s.mu.Unlock()

	if !exists {
		return nil, ErrRecordingNotFound
	}

	recording, err := recorder.Stop(stoppedBy)
	if err != nil {
		return nil, fmt.Errorf("failed to stop recording: %w", err)
	}

	return recording, nil
}

// GetRecording returns the current recording for a room.
func (s *RecordingService) GetRecording(ctx context.Context, roomID string) (*Recording, error) {
	s.mu.RLock()
	recorder, exists := s.recorders[roomID]
	s.mu.RUnlock()

	if !exists {
		return nil, ErrRecordingNotFound
	}

	return recorder.GetRecording(), nil
}

// IsRecording checks if a room is currently being recorded.
func (s *RecordingService) IsRecording(roomID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.recorders[roomID]
	return exists
}

// GetRecorder returns the recorder for a room.
func (s *RecordingService) GetRecorder(roomID string) (*Recorder, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	recorder, exists := s.recorders[roomID]
	if !exists {
		return nil, ErrRecordingNotFound
	}
	return recorder, nil
}

// AddTrack adds a track to the recording for a room.
func (s *RecordingService) AddTrack(roomID, trackID string, kind TrackKind, codec string, publisherID string) error {
	s.mu.RLock()
	recorder, exists := s.recorders[roomID]
	s.mu.RUnlock()

	if !exists {
		return nil // Not recording, no error
	}

	return recorder.AddTrack(trackID, kind, codec, publisherID)
}

// RemoveTrack removes a track from the recording for a room.
func (s *RecordingService) RemoveTrack(roomID, trackID string) error {
	s.mu.RLock()
	recorder, exists := s.recorders[roomID]
	s.mu.RUnlock()

	if !exists {
		return nil // Not recording, no error
	}

	return recorder.RemoveTrack(trackID)
}

// WriteRTP writes an RTP packet to the recording for a room.
func (s *RecordingService) WriteRTP(roomID, trackID string, packet *rtp.Packet) error {
	s.mu.RLock()
	recorder, exists := s.recorders[roomID]
	s.mu.RUnlock()

	if !exists {
		return nil // Not recording, no error
	}

	return recorder.WriteRTP(trackID, packet)
}

// AddParticipant adds a participant to the recording for a room.
func (s *RecordingService) AddParticipant(roomID, participantID string) {
	s.mu.RLock()
	recorder, exists := s.recorders[roomID]
	s.mu.RUnlock()

	if !exists {
		return // Not recording
	}

	recorder.AddParticipant(participantID)
}

// RemoveParticipant marks a participant as having left the recording.
func (s *RecordingService) RemoveParticipant(roomID, participantID string) {
	s.mu.RLock()
	recorder, exists := s.recorders[roomID]
	s.mu.RUnlock()

	if !exists {
		return // Not recording
	}

	recorder.RemoveParticipant(participantID)
}

// SetParticipantConsent sets the consent status for a participant.
func (s *RecordingService) SetParticipantConsent(roomID, participantID string, consented bool) {
	s.mu.RLock()
	recorder, exists := s.recorders[roomID]
	s.mu.RUnlock()

	if !exists {
		return // Not recording
	}

	recorder.SetParticipantConsent(participantID, consented)
}

// onRecordingStopped handles the callback when a recording is stopped.
func (s *RecordingService) onRecordingStopped(recording *Recording) {
	if s.uploader == nil {
		if s.logger != nil {
			s.logger.Warn("no uploader configured, recording will not be uploaded",
				"recording_id", recording.ID)
		}
		return
	}

	// Queue recording for upload
	s.uploader.QueueRecording(recording)
}

// Shutdown stops all recordings and shuts down the service.
func (s *RecordingService) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	recorders := make([]*Recorder, 0, len(s.recorders))
	for _, r := range s.recorders {
		recorders = append(recorders, r)
	}
	s.recorders = make(map[string]*Recorder)
	s.mu.Unlock()

	// Stop all recordings
	for _, r := range recorders {
		if _, err := r.Stop("system_shutdown"); err != nil {
			if s.logger != nil {
				s.logger.Error("failed to stop recording during shutdown",
					"recording_id", r.ID(),
					"error", err)
			}
		}
	}

	// Shutdown uploader
	if s.uploader != nil {
		s.uploader.Shutdown(ctx)
	}

	return nil
}

// Errors
var (
	ErrRecordingDisabled       = errors.New("recording is disabled")
	ErrRecordingAlreadyExists  = errors.New("recording already exists for this room")
	ErrRecordingNotFound       = errors.New("recording not found")
	ErrRecordingAlreadyStarted = errors.New("recording already started")
	ErrRecordingNotStarted     = errors.New("recording not started")
)
