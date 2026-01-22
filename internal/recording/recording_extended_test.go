package recording

import (
	"context"
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/HMasataka/choice/internal/recording/storage"
	"github.com/HMasataka/choice/pkg/logger"
)

func newTestLogger(t *testing.T) *logger.Logger {
	log, err := logger.New(logger.Config{Level: "error"})
	require.NoError(t, err)
	return log
}

func TestRecorder_AddTrack(t *testing.T) {
	tempDir := t.TempDir()

	recorder, err := NewRecorder(RecorderConfig{
		RoomID:    "room-tracks",
		TempDir:   tempDir,
		Format:    "webm",
		StartedBy: "admin",
		Logger:    newTestLogger(t),
	})
	require.NoError(t, err)

	// Cannot add track before starting
	err = recorder.AddTrack("track-1", TrackKindVideo, "VP8", "user-1")
	assert.ErrorIs(t, err, ErrRecordingNotStarted)

	// Start recording
	err = recorder.Start()
	require.NoError(t, err)

	// Add video track
	err = recorder.AddTrack("track-1", TrackKindVideo, "VP8", "user-1")
	require.NoError(t, err)

	// Add audio track
	err = recorder.AddTrack("track-2", TrackKindAudio, "Opus", "user-1")
	require.NoError(t, err)

	// Cannot add duplicate track
	err = recorder.AddTrack("track-1", TrackKindVideo, "VP8", "user-1")
	assert.Error(t, err)

	recording, err := recorder.Stop("admin")
	require.NoError(t, err)
	assert.Len(t, recording.Tracks, 2)
}

func TestRecorder_RemoveTrack(t *testing.T) {
	tempDir := t.TempDir()

	recorder, err := NewRecorder(RecorderConfig{
		RoomID:    "room-remove",
		TempDir:   tempDir,
		Format:    "webm",
		StartedBy: "admin",
		Logger:    newTestLogger(t),
	})
	require.NoError(t, err)

	// Cannot remove track before starting
	err = recorder.RemoveTrack("track-1")
	assert.ErrorIs(t, err, ErrRecordingNotStarted)

	// Start recording
	err = recorder.Start()
	require.NoError(t, err)

	// Add then remove track
	err = recorder.AddTrack("track-1", TrackKindVideo, "VP8", "user-1")
	require.NoError(t, err)

	err = recorder.RemoveTrack("track-1")
	require.NoError(t, err)

	// Cannot remove non-existent track
	err = recorder.RemoveTrack("track-nonexistent")
	assert.Error(t, err)

	recording, err := recorder.Stop("admin")
	require.NoError(t, err)
	// Track should still be in recording for upload (moved to removedTracks)
	assert.Len(t, recording.Tracks, 1)
}

func TestRecorder_WriteRTP(t *testing.T) {
	tempDir := t.TempDir()

	recorder, err := NewRecorder(RecorderConfig{
		RoomID:    "room-rtp",
		TempDir:   tempDir,
		Format:    "webm",
		StartedBy: "admin",
		Logger:    newTestLogger(t),
	})
	require.NoError(t, err)

	// Cannot write before starting
	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			SequenceNumber: 1,
			Timestamp:      12345,
			SSRC:           99999,
		},
		Payload: []byte{0x01, 0x02, 0x03},
	}
	err = recorder.WriteRTP("track-1", packet)
	assert.ErrorIs(t, err, ErrRecordingNotStarted)

	// Start recording
	err = recorder.Start()
	require.NoError(t, err)

	// Add track
	err = recorder.AddTrack("track-1", TrackKindVideo, "VP8", "user-1")
	require.NoError(t, err)

	// Write RTP packet
	err = recorder.WriteRTP("track-1", packet)
	require.NoError(t, err)

	// Write more packets
	for i := 0; i < 10; i++ {
		packet.Header.SequenceNumber = uint16(i + 2)
		err = recorder.WriteRTP("track-1", packet)
		require.NoError(t, err)
	}

	_, err = recorder.Stop("admin")
	require.NoError(t, err)
}

func TestRecorder_OnStopCallback(t *testing.T) {
	tempDir := t.TempDir()

	callbackCalled := make(chan *Recording, 1)

	recorder, err := NewRecorder(RecorderConfig{
		RoomID:    "room-callback",
		TempDir:   tempDir,
		Format:    "webm",
		StartedBy: "admin",
		Logger:    newTestLogger(t),
		OnStop: func(recording *Recording) {
			callbackCalled <- recording
		},
	})
	require.NoError(t, err)

	err = recorder.Start()
	require.NoError(t, err)

	_, err = recorder.Stop("admin")
	require.NoError(t, err)

	// Callback should be called
	select {
	case recording := <-callbackCalled:
		assert.NotNil(t, recording)
		assert.Equal(t, "room-callback", recording.RoomID)
	case <-time.After(1 * time.Second):
		t.Error("callback was not called")
	}
}

func TestRecorder_GetRecording(t *testing.T) {
	tempDir := t.TempDir()

	recorder, err := NewRecorder(RecorderConfig{
		RoomID:    "room-get",
		TempDir:   tempDir,
		Format:    "webm",
		StartedBy: "admin",
	})
	require.NoError(t, err)

	// Get recording before start
	recording := recorder.GetRecording()
	assert.Equal(t, storage.RecordingStatusPending, recording.Status)

	err = recorder.Start()
	require.NoError(t, err)

	// Get recording while recording
	recording = recorder.GetRecording()
	assert.Equal(t, storage.RecordingStatusRecording, recording.Status)

	_, err = recorder.Stop("admin")
	require.NoError(t, err)

	// Get recording after stop
	recording = recorder.GetRecording()
	assert.Equal(t, storage.RecordingStatusStopped, recording.Status)
}

func TestRecorder_AddParticipantDuplicate(t *testing.T) {
	tempDir := t.TempDir()

	recorder, err := NewRecorder(RecorderConfig{
		RoomID:    "room-dup",
		TempDir:   tempDir,
		StartedBy: "admin",
	})
	require.NoError(t, err)

	err = recorder.Start()
	require.NoError(t, err)

	// Add participant twice should not duplicate
	recorder.AddParticipant("user-1")
	recorder.AddParticipant("user-1")

	recording, err := recorder.Stop("admin")
	require.NoError(t, err)
	assert.Len(t, recording.Participants, 1)
}

func TestRecordingService_GetRecorder(t *testing.T) {
	tempDir := t.TempDir()

	service := NewRecordingService(RecordingServiceConfig{
		Enabled: true,
		TempDir: tempDir,
	})

	ctx := context.Background()
	roomID := "room-get-recorder"

	// Should not find recorder before start
	_, err := service.GetRecorder(roomID)
	assert.ErrorIs(t, err, ErrRecordingNotFound)

	// Start recording
	_, err = service.StartRecording(ctx, roomID, "admin")
	require.NoError(t, err)

	// Should find recorder now
	recorder, err := service.GetRecorder(roomID)
	require.NoError(t, err)
	assert.NotNil(t, recorder)
	assert.Equal(t, roomID, recorder.RoomID())
}

func TestRecordingService_TrackOperations(t *testing.T) {
	tempDir := t.TempDir()

	service := NewRecordingService(RecordingServiceConfig{
		Enabled: true,
		TempDir: tempDir,
	})

	ctx := context.Background()
	roomID := "room-tracks"

	// Operations on non-recording room should not error
	err := service.AddTrack(roomID, "track-1", TrackKindVideo, "VP8", "user-1")
	assert.NoError(t, err) // Returns nil, not recording

	err = service.RemoveTrack(roomID, "track-1")
	assert.NoError(t, err) // Returns nil, not recording

	// Start recording
	_, err = service.StartRecording(ctx, roomID, "admin")
	require.NoError(t, err)

	// Add track
	err = service.AddTrack(roomID, "track-1", TrackKindVideo, "VP8", "user-1")
	require.NoError(t, err)

	// Remove track
	err = service.RemoveTrack(roomID, "track-1")
	require.NoError(t, err)

	_, err = service.StopRecording(ctx, roomID, "admin")
	require.NoError(t, err)
}

func TestRecordingService_WriteRTPOperation(t *testing.T) {
	tempDir := t.TempDir()

	service := NewRecordingService(RecordingServiceConfig{
		Enabled: true,
		TempDir: tempDir,
	})

	ctx := context.Background()
	roomID := "room-rtp"

	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			SequenceNumber: 1,
			Timestamp:      12345,
			SSRC:           99999,
		},
		Payload: []byte{0x01, 0x02, 0x03},
	}

	// Write to non-recording room should not error
	err := service.WriteRTP(roomID, "track-1", packet)
	assert.NoError(t, err) // Returns nil, not recording

	// Start recording
	_, err = service.StartRecording(ctx, roomID, "admin")
	require.NoError(t, err)

	// Add track
	err = service.AddTrack(roomID, "track-1", TrackKindVideo, "VP8", "user-1")
	require.NoError(t, err)

	// Write RTP
	err = service.WriteRTP(roomID, "track-1", packet)
	require.NoError(t, err)

	_, err = service.StopRecording(ctx, roomID, "admin")
	require.NoError(t, err)
}

func TestRecordingService_WithStorage(t *testing.T) {
	tempDir := t.TempDir()

	// Create local storage
	localStorage, err := storage.NewLocalStorage(tempDir)
	require.NoError(t, err)

	service := NewRecordingService(RecordingServiceConfig{
		Enabled: true,
		TempDir: tempDir,
		Format:  "webm",
		Storage: localStorage,
		Logger:  newTestLogger(t),
	})

	ctx := context.Background()
	roomID := "room-storage"

	// Start recording
	_, err = service.StartRecording(ctx, roomID, "admin")
	require.NoError(t, err)

	// Stop recording - should trigger upload
	recording, err := service.StopRecording(ctx, roomID, "admin")
	require.NoError(t, err)
	assert.NotNil(t, recording)

	// Wait for upload
	time.Sleep(100 * time.Millisecond)

	// Shutdown
	err = service.Shutdown(ctx)
	require.NoError(t, err)
}

func TestRecordingService_ShutdownWithNoUploader(t *testing.T) {
	tempDir := t.TempDir()

	service := NewRecordingService(RecordingServiceConfig{
		Enabled: true,
		TempDir: tempDir,
		Logger:  newTestLogger(t),
	})

	ctx := context.Background()

	// Start recording
	_, err := service.StartRecording(ctx, "room-1", "admin")
	require.NoError(t, err)

	// Shutdown should still work without uploader
	err = service.Shutdown(ctx)
	require.NoError(t, err)
}

func TestRecorder_DefaultFormat(t *testing.T) {
	tempDir := t.TempDir()

	recorder, err := NewRecorder(RecorderConfig{
		RoomID:    "room-format",
		TempDir:   tempDir,
		Format:    "", // Empty should default to webm
		StartedBy: "admin",
	})
	require.NoError(t, err)

	err = recorder.Start()
	require.NoError(t, err)

	recording, err := recorder.Stop("admin")
	require.NoError(t, err)
	assert.Equal(t, "webm", recording.Format)
}
