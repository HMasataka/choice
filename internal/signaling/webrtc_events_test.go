package signaling

import (
	"context"
	"testing"

	pion "github.com/pion/webrtc/v4"

	"github.com/HMasataka/choice/internal/media"
	"github.com/HMasataka/choice/internal/signaling/protocol"
	"github.com/HMasataka/choice/pkg/logger"
)

// mockMediaRouter is a mock implementation of media.MediaRouter for testing.
type mockMediaRouter struct {
	tracks map[media.TrackID]*media.LocalTrack
}

func newMockMediaRouter() *mockMediaRouter {
	return &mockMediaRouter{
		tracks: make(map[media.TrackID]*media.LocalTrack),
	}
}

func (m *mockMediaRouter) AddTrack(_ context.Context, track *media.LocalTrack) error {
	if track == nil {
		return protocol.NewInternalError("track cannot be nil")
	}
	m.tracks[track.ID] = track
	return nil
}

func (m *mockMediaRouter) RemoveTrack(_ context.Context, trackID media.TrackID) error {
	delete(m.tracks, trackID)
	return nil
}

func (m *mockMediaRouter) Subscribe(_ context.Context, subscriberID string, trackID media.TrackID, opts *media.SubscribeOptions) (*media.Subscription, error) {
	return nil, nil
}

func (m *mockMediaRouter) Unsubscribe(_ context.Context, subscriptionID media.SubscriptionID) error {
	return nil
}

func (m *mockMediaRouter) GetTrack(_ context.Context, trackID media.TrackID) (*media.LocalTrack, error) {
	track, ok := m.tracks[trackID]
	if !ok {
		return nil, protocol.NewError(protocol.CodeTrackNotFound, "track not found", nil)
	}
	return track, nil
}

func (m *mockMediaRouter) ListTracks(_ context.Context) ([]*media.LocalTrack, error) {
	tracks := make([]*media.LocalTrack, 0, len(m.tracks))
	for _, track := range m.tracks {
		tracks = append(tracks, track)
	}
	return tracks, nil
}

func TestCheckTrackLimit(t *testing.T) {
	log, _ := logger.New(logger.DefaultConfig())
	notifier := NewNotifier()
	mediaRouter := newMockMediaRouter()
	bridge := NewWebRTCEventsBridge(notifier, mediaRouter, *log)

	participantID := "test-participant"

	// Test video track limits (3 max)
	for i := 0; i < 3; i++ {
		if err := bridge.checkTrackLimit(participantID, pion.RTPCodecTypeVideo); err != nil {
			t.Errorf("Expected no error for video track %d, got: %v", i, err)
		}
		bridge.incrementTrackCount(participantID, pion.RTPCodecTypeVideo)
	}

	// 4th video track should exceed limit
	err := bridge.checkTrackLimit(participantID, pion.RTPCodecTypeVideo)
	if err == nil {
		t.Error("Expected error for 4th video track, got nil")
	}
	if protoErr, ok := err.(*protocol.Error); ok {
		if protoErr.Code != protocol.CodeTrackLimitExceeded {
			t.Errorf("Expected CodeTrackLimitExceeded, got %d", protoErr.Code)
		}
	} else {
		t.Error("Expected protocol.Error type")
	}

	// Test audio track limits (2 max)
	for i := 0; i < 2; i++ {
		if err := bridge.checkTrackLimit(participantID, pion.RTPCodecTypeAudio); err != nil {
			t.Errorf("Expected no error for audio track %d, got: %v", i, err)
		}
		bridge.incrementTrackCount(participantID, pion.RTPCodecTypeAudio)
	}

	// 3rd audio track should exceed limit
	err = bridge.checkTrackLimit(participantID, pion.RTPCodecTypeAudio)
	if err == nil {
		t.Error("Expected error for 3rd audio track, got nil")
	}
	if protoErr, ok := err.(*protocol.Error); ok {
		if protoErr.Code != protocol.CodeTrackLimitExceeded {
			t.Errorf("Expected CodeTrackLimitExceeded, got %d", protoErr.Code)
		}
	} else {
		t.Error("Expected protocol.Error type")
	}
}

func TestIncrementTrackCount(t *testing.T) {
	log, _ := logger.New(logger.DefaultConfig())
	notifier := NewNotifier()
	mediaRouter := newMockMediaRouter()
	bridge := NewWebRTCEventsBridge(notifier, mediaRouter, *log)

	participantID := "test-participant"

	// Initial counts should be 0
	bridge.mu.RLock()
	tracks := bridge.participantTracks[participantID]
	bridge.mu.RUnlock()
	if tracks != nil {
		t.Error("Expected nil tracks for new participant")
	}

	// Increment video count
	bridge.incrementTrackCount(participantID, pion.RTPCodecTypeVideo)
	bridge.mu.RLock()
	tracks = bridge.participantTracks[participantID]
	bridge.mu.RUnlock()
	if tracks == nil {
		t.Fatal("Expected tracks to be created")
	}
	if tracks.VideoCount != 1 {
		t.Errorf("Expected VideoCount=1, got %d", tracks.VideoCount)
	}
	if tracks.AudioCount != 0 {
		t.Errorf("Expected AudioCount=0, got %d", tracks.AudioCount)
	}

	// Increment audio count
	bridge.incrementTrackCount(participantID, pion.RTPCodecTypeAudio)
	bridge.mu.RLock()
	tracks = bridge.participantTracks[participantID]
	bridge.mu.RUnlock()
	if tracks.VideoCount != 1 {
		t.Errorf("Expected VideoCount=1, got %d", tracks.VideoCount)
	}
	if tracks.AudioCount != 1 {
		t.Errorf("Expected AudioCount=1, got %d", tracks.AudioCount)
	}
}

func TestRegisterUnregisterParticipant(t *testing.T) {
	log, _ := logger.New(logger.DefaultConfig())
	notifier := NewNotifier()
	mediaRouter := newMockMediaRouter()
	bridge := NewWebRTCEventsBridge(notifier, mediaRouter, *log)

	participantID := "test-participant"
	roomID := "test-room"

	// Create a mock connection with room_id data
	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	conn.SetData("room_id", roomID)

	// Register participant
	bridge.RegisterParticipant(participantID, conn)

	// Check participant is registered
	bridge.mu.RLock()
	registeredConn, hasConn := bridge.participantConnections[participantID]
	registeredRoom, hasRoom := bridge.participantRooms[participantID]
	bridge.mu.RUnlock()

	if !hasConn {
		t.Error("Expected participant to be registered")
	}
	if registeredConn != conn {
		t.Error("Expected registered connection to match")
	}
	if !hasRoom {
		t.Error("Expected room ID to be registered")
	}
	if registeredRoom != roomID {
		t.Errorf("Expected room ID %s, got %s", roomID, registeredRoom)
	}

	// Unregister participant
	bridge.UnregisterParticipant(participantID)

	// Check participant is unregistered
	bridge.mu.RLock()
	_, hasConn = bridge.participantConnections[participantID]
	_, hasRoom = bridge.participantRooms[participantID]
	_, hasTracks := bridge.participantTracks[participantID]
	bridge.mu.RUnlock()

	if hasConn {
		t.Error("Expected participant connection to be unregistered")
	}
	if hasRoom {
		t.Error("Expected participant room to be unregistered")
	}
	if hasTracks {
		t.Error("Expected participant tracks to be unregistered")
	}
}

func TestRegisterParticipantNilConnection(t *testing.T) {
	log, _ := logger.New(logger.DefaultConfig())
	notifier := NewNotifier()
	mediaRouter := newMockMediaRouter()
	bridge := NewWebRTCEventsBridge(notifier, mediaRouter, *log)

	// Register with nil connection should not panic
	bridge.RegisterParticipant("test-participant", nil)

	// Check participant is not registered
	bridge.mu.RLock()
	_, hasConn := bridge.participantConnections["test-participant"]
	bridge.mu.RUnlock()

	if hasConn {
		t.Error("Expected participant not to be registered with nil connection")
	}
}

func TestRegisterParticipantEmptyID(t *testing.T) {
	log, _ := logger.New(logger.DefaultConfig())
	notifier := NewNotifier()
	mediaRouter := newMockMediaRouter()
	bridge := NewWebRTCEventsBridge(notifier, mediaRouter, *log)

	// Create a mock connection
	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}

	// Register with empty ID should not panic
	bridge.RegisterParticipant("", conn)

	// Check no participant is registered
	bridge.mu.RLock()
	count := len(bridge.participantConnections)
	bridge.mu.RUnlock()

	if count != 0 {
		t.Errorf("Expected no participants registered, got %d", count)
	}
}
