package recording

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/HMasataka/choice/internal/recording/storage"
)

func TestRecorder_NewRecorder(t *testing.T) {
	tempDir := t.TempDir()

	recorder, err := NewRecorder(RecorderConfig{
		RoomID:    "room-123",
		TempDir:   tempDir,
		Format:    "webm",
		StartedBy: "admin",
	})
	require.NoError(t, err)
	assert.NotNil(t, recorder)
	assert.NotEmpty(t, recorder.ID())
	assert.Equal(t, "room-123", recorder.RoomID())
	assert.Equal(t, storage.RecordingStatusPending, recorder.Status())
}

func TestRecorder_StartStop(t *testing.T) {
	tempDir := t.TempDir()

	recorder, err := NewRecorder(RecorderConfig{
		RoomID:    "room-123",
		TempDir:   tempDir,
		Format:    "webm",
		StartedBy: "admin",
	})
	require.NoError(t, err)

	// Start recording
	err = recorder.Start()
	require.NoError(t, err)
	assert.Equal(t, storage.RecordingStatusRecording, recorder.Status())
	assert.False(t, recorder.StartedAt().IsZero())

	// Wait a bit for duration
	time.Sleep(10 * time.Millisecond)

	// Stop recording
	recording, err := recorder.Stop("admin")
	require.NoError(t, err)
	assert.Equal(t, storage.RecordingStatusStopped, recorder.Status())
	assert.NotNil(t, recording)
	assert.Equal(t, "room-123", recording.RoomID)
	assert.True(t, recording.Duration > 0)
}

func TestRecorder_StartAlreadyStarted(t *testing.T) {
	tempDir := t.TempDir()

	recorder, err := NewRecorder(RecorderConfig{
		RoomID:    "room-123",
		TempDir:   tempDir,
		StartedBy: "admin",
	})
	require.NoError(t, err)

	err = recorder.Start()
	require.NoError(t, err)

	// Try to start again
	err = recorder.Start()
	assert.ErrorIs(t, err, ErrRecordingAlreadyStarted)
}

func TestRecorder_StopNotStarted(t *testing.T) {
	tempDir := t.TempDir()

	recorder, err := NewRecorder(RecorderConfig{
		RoomID:    "room-123",
		TempDir:   tempDir,
		StartedBy: "admin",
	})
	require.NoError(t, err)

	// Try to stop without starting
	_, err = recorder.Stop("admin")
	assert.ErrorIs(t, err, ErrRecordingNotStarted)
}

func TestRecorder_Participants(t *testing.T) {
	tempDir := t.TempDir()

	recorder, err := NewRecorder(RecorderConfig{
		RoomID:    "room-123",
		TempDir:   tempDir,
		StartedBy: "admin",
	})
	require.NoError(t, err)

	err = recorder.Start()
	require.NoError(t, err)

	// Add participants
	recorder.AddParticipant("user-1")
	recorder.AddParticipant("user-2")

	// Set consent
	recorder.SetParticipantConsent("user-1", true)

	// Remove participant
	recorder.RemoveParticipant("user-2")

	recording, err := recorder.Stop("admin")
	require.NoError(t, err)

	assert.Len(t, recording.Participants, 2)

	// Find user-1 and verify consent
	var user1Found bool
	for _, p := range recording.Participants {
		if p.ParticipantID == "user-1" {
			user1Found = true
			assert.True(t, p.ConsentGiven)
			assert.NotNil(t, p.ConsentTimestamp)
		}
		if p.ParticipantID == "user-2" {
			assert.NotNil(t, p.LeftAt)
		}
	}
	assert.True(t, user1Found)
}

func TestRecorder_Duration(t *testing.T) {
	tempDir := t.TempDir()

	recorder, err := NewRecorder(RecorderConfig{
		RoomID:    "room-123",
		TempDir:   tempDir,
		StartedBy: "admin",
	})
	require.NoError(t, err)

	// Duration should be 0 before start
	assert.Equal(t, time.Duration(0), recorder.Duration())

	err = recorder.Start()
	require.NoError(t, err)

	// Wait and check duration is increasing
	time.Sleep(50 * time.Millisecond)
	duration1 := recorder.Duration()
	assert.True(t, duration1 > 0)

	time.Sleep(50 * time.Millisecond)
	duration2 := recorder.Duration()
	assert.True(t, duration2 > duration1)

	// Stop and check duration is fixed
	_, err = recorder.Stop("admin")
	require.NoError(t, err)

	duration3 := recorder.Duration()
	time.Sleep(50 * time.Millisecond)
	duration4 := recorder.Duration()
	assert.Equal(t, duration3, duration4)
}

func TestRecordingService_StartStopRecording(t *testing.T) {
	tempDir := t.TempDir()

	service := NewRecordingService(RecordingServiceConfig{
		Enabled: true,
		TempDir: tempDir,
		Format:  "webm",
	})

	ctx := context.Background()
	roomID := "room-456"

	// Start recording
	recording, err := service.StartRecording(ctx, roomID, "admin")
	require.NoError(t, err)
	assert.NotNil(t, recording)
	assert.Equal(t, roomID, recording.RoomID)

	// Check is recording
	assert.True(t, service.IsRecording(roomID))

	// Stop recording
	recording, err = service.StopRecording(ctx, roomID, "admin")
	require.NoError(t, err)
	assert.NotNil(t, recording)
	assert.Equal(t, storage.RecordingStatusStopped, recording.Status)

	// Check not recording anymore
	assert.False(t, service.IsRecording(roomID))
}

func TestRecordingService_StartRecordingAlreadyExists(t *testing.T) {
	tempDir := t.TempDir()

	service := NewRecordingService(RecordingServiceConfig{
		Enabled: true,
		TempDir: tempDir,
	})

	ctx := context.Background()
	roomID := "room-456"

	// Start recording
	_, err := service.StartRecording(ctx, roomID, "admin")
	require.NoError(t, err)

	// Try to start again
	_, err = service.StartRecording(ctx, roomID, "admin")
	assert.ErrorIs(t, err, ErrRecordingAlreadyExists)
}

func TestRecordingService_StopRecordingNotFound(t *testing.T) {
	tempDir := t.TempDir()

	service := NewRecordingService(RecordingServiceConfig{
		Enabled: true,
		TempDir: tempDir,
	})

	ctx := context.Background()

	_, err := service.StopRecording(ctx, "nonexistent-room", "admin")
	assert.ErrorIs(t, err, ErrRecordingNotFound)
}

func TestRecordingService_Disabled(t *testing.T) {
	service := NewRecordingService(RecordingServiceConfig{
		Enabled: false,
	})

	assert.False(t, service.IsEnabled())

	ctx := context.Background()
	_, err := service.StartRecording(ctx, "room-123", "admin")
	assert.ErrorIs(t, err, ErrRecordingDisabled)
}

func TestRecordingService_GetRecording(t *testing.T) {
	tempDir := t.TempDir()

	service := NewRecordingService(RecordingServiceConfig{
		Enabled: true,
		TempDir: tempDir,
	})

	ctx := context.Background()
	roomID := "room-789"

	// Should not find recording before start
	_, err := service.GetRecording(ctx, roomID)
	assert.ErrorIs(t, err, ErrRecordingNotFound)

	// Start recording
	_, err = service.StartRecording(ctx, roomID, "admin")
	require.NoError(t, err)

	// Should find recording now
	recording, err := service.GetRecording(ctx, roomID)
	require.NoError(t, err)
	assert.Equal(t, roomID, recording.RoomID)
	assert.Equal(t, storage.RecordingStatusRecording, recording.Status)
}

func TestRecordingService_ParticipantOperations(t *testing.T) {
	tempDir := t.TempDir()

	service := NewRecordingService(RecordingServiceConfig{
		Enabled: true,
		TempDir: tempDir,
	})

	ctx := context.Background()
	roomID := "room-participants"

	// Operations on non-recording room should not error
	service.AddParticipant(roomID, "user-1")
	service.RemoveParticipant(roomID, "user-1")
	service.SetParticipantConsent(roomID, "user-1", true)

	// Start recording
	_, err := service.StartRecording(ctx, roomID, "admin")
	require.NoError(t, err)

	// Now operations should work
	service.AddParticipant(roomID, "user-1")
	service.SetParticipantConsent(roomID, "user-1", true)
	service.RemoveParticipant(roomID, "user-1")

	recording, err := service.StopRecording(ctx, roomID, "admin")
	require.NoError(t, err)
	assert.Len(t, recording.Participants, 1)
}

func TestRecordingService_Shutdown(t *testing.T) {
	tempDir := t.TempDir()

	service := NewRecordingService(RecordingServiceConfig{
		Enabled: true,
		TempDir: tempDir,
	})

	ctx := context.Background()

	// Start multiple recordings
	_, err := service.StartRecording(ctx, "room-1", "admin")
	require.NoError(t, err)
	_, err = service.StartRecording(ctx, "room-2", "admin")
	require.NoError(t, err)

	// Shutdown should stop all recordings
	err = service.Shutdown(ctx)
	require.NoError(t, err)

	// All recordings should be stopped
	assert.False(t, service.IsRecording("room-1"))
	assert.False(t, service.IsRecording("room-2"))
}
