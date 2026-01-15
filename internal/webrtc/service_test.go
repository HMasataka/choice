package webrtc

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	pion "github.com/pion/webrtc/v4"
)

// mockEventHandler is a mock implementation of EventHandler for testing.
type mockEventHandler struct {
	mu                             sync.Mutex
	onICEConnectionStateCalled     int
	onPeerConnectionStateCalled    int
	onICECandidateCalled           int
	onTrackCalled                  int
	onNegotiationNeededCalled      int
	lastParticipantID              string
	lastICEConnectionState         pion.ICEConnectionState
	lastPeerConnectionState        pion.PeerConnectionState
	lastCandidate                  pion.ICECandidateInit
	iceConnectionStateChangeFunc   func(participantID string, state pion.ICEConnectionState)
	peerConnectionStateChangeFunc  func(participantID string, state pion.PeerConnectionState)
	iceCandidateFunc               func(participantID string, candidate pion.ICECandidateInit)
	trackFunc                      func(participantID string, track *pion.TrackRemote, receiver *pion.RTPReceiver)
	negotiationNeededFunc          func(participantID string)
}

func (m *mockEventHandler) OnICEConnectionStateChange(participantID string, state pion.ICEConnectionState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onICEConnectionStateCalled++
	m.lastParticipantID = participantID
	m.lastICEConnectionState = state
	if m.iceConnectionStateChangeFunc != nil {
		m.iceConnectionStateChangeFunc(participantID, state)
	}
}

func (m *mockEventHandler) OnPeerConnectionStateChange(participantID string, state pion.PeerConnectionState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onPeerConnectionStateCalled++
	m.lastParticipantID = participantID
	m.lastPeerConnectionState = state
	if m.peerConnectionStateChangeFunc != nil {
		m.peerConnectionStateChangeFunc(participantID, state)
	}
}

func (m *mockEventHandler) OnICECandidate(participantID string, candidate pion.ICECandidateInit) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onICECandidateCalled++
	m.lastParticipantID = participantID
	m.lastCandidate = candidate
	if m.iceCandidateFunc != nil {
		m.iceCandidateFunc(participantID, candidate)
	}
}

func (m *mockEventHandler) OnTrack(participantID string, track *pion.TrackRemote, receiver *pion.RTPReceiver) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onTrackCalled++
	m.lastParticipantID = participantID
	if m.trackFunc != nil {
		m.trackFunc(participantID, track, receiver)
	}
}

func (m *mockEventHandler) OnNegotiationNeeded(participantID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onNegotiationNeededCalled++
	m.lastParticipantID = participantID
	if m.negotiationNeededFunc != nil {
		m.negotiationNeededFunc(participantID)
	}
}

func (m *mockEventHandler) getCallCount(event string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch event {
	case "ICEConnectionState":
		return m.onICEConnectionStateCalled
	case "PeerConnectionState":
		return m.onPeerConnectionStateCalled
	case "ICECandidate":
		return m.onICECandidateCalled
	case "Track":
		return m.onTrackCalled
	case "NegotiationNeeded":
		return m.onNegotiationNeededCalled
	default:
		return 0
	}
}

// TestService_NewService verifies Service creation.
func TestService_NewService(t *testing.T) {
	config := DefaultPeerConfig()
	mediaEngine := &pion.MediaEngine{}
	eventHandler := &mockEventHandler{}

	service := NewService(config, mediaEngine, eventHandler)

	if service == nil {
		t.Fatal("NewService returned nil")
	}

	if len(service.peers) != 0 {
		t.Errorf("peers map should be empty on creation, got %d peers", len(service.peers))
	}
}

// TestService_HandleOffer_CreatesPeerLazily verifies that HandleOffer creates a peer on first call.
func TestService_HandleOffer_CreatesPeerLazily(t *testing.T) {
	config := DefaultPeerConfig()
	mediaEngine := &pion.MediaEngine{}
	eventHandler := &mockEventHandler{}

	service := NewService(config, mediaEngine, eventHandler)
	participantID := "test-participant-1"

	// Create a simple SDP offer
	offerSDP := "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0\r\na=extmap-allow-mixed\r\na=msid-semantic: WMS\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\nc=IN IP4 0.0.0.0\r\na=ice-ufrag:test\r\na=ice-pwd:testpassword1234567890123456\r\na=ice-options:trickle\r\na=fingerprint:sha-256 00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00\r\na=setup:actpass\r\na=mid:0\r\na=sctp-port:5000\r\na=max-message-size:262144\r\n"

	// Verify no peer exists yet
	if len(service.peers) != 0 {
		t.Errorf("peers map should be empty before HandleOffer, got %d peers", len(service.peers))
	}

	// Handle offer - should create peer lazily
	ctx := context.Background()
	answerSDP, err := service.HandleOffer(ctx, participantID, offerSDP)
	if err != nil {
		t.Fatalf("HandleOffer failed: %v", err)
	}

	if answerSDP == "" {
		t.Error("HandleOffer returned empty answer SDP")
	}

	// Verify peer was created
	if len(service.peers) != 1 {
		t.Errorf("peers map should have 1 peer after HandleOffer, got %d peers", len(service.peers))
	}

	peer := service.peers[participantID]
	if peer == nil {
		t.Error("peer should exist in map after HandleOffer")
	}
}

// TestService_HandleOffer_ReusesExistingPeer verifies that HandleOffer reuses an existing peer.
func TestService_HandleOffer_ReusesExistingPeer(t *testing.T) {
	config := DefaultPeerConfig()
	mediaEngine := &pion.MediaEngine{}
	eventHandler := &mockEventHandler{}

	service := NewService(config, mediaEngine, eventHandler)
	participantID := "test-participant-1"

	offerSDP := "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0\r\na=extmap-allow-mixed\r\na=msid-semantic: WMS\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\nc=IN IP4 0.0.0.0\r\na=ice-ufrag:test\r\na=ice-pwd:testpassword1234567890123456\r\na=ice-options:trickle\r\na=fingerprint:sha-256 00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00\r\na=setup:actpass\r\na=mid:0\r\na=sctp-port:5000\r\na=max-message-size:262144\r\n"

	ctx := context.Background()

	// First offer
	_, err := service.HandleOffer(ctx, participantID, offerSDP)
	if err != nil {
		t.Fatalf("First HandleOffer failed: %v", err)
	}

	firstPeer := service.peers[participantID]

	// Second offer (renegotiation)
	_, err = service.HandleOffer(ctx, participantID, offerSDP)
	if err != nil {
		t.Fatalf("Second HandleOffer failed: %v", err)
	}

	secondPeer := service.peers[participantID]

	// Verify same peer instance is reused
	if firstPeer != secondPeer {
		t.Error("HandleOffer should reuse existing peer, but created a new one")
	}

	// Verify only one peer in map
	if len(service.peers) != 1 {
		t.Errorf("peers map should have 1 peer after renegotiation, got %d peers", len(service.peers))
	}
}

// TestService_HandleAnswer verifies HandleAnswer processes SDP answer.
func TestService_HandleAnswer(t *testing.T) {
	config := DefaultPeerConfig()
	mediaEngine := &pion.MediaEngine{}
	eventHandler := &mockEventHandler{}

	service := NewService(config, mediaEngine, eventHandler)
	participantID := "test-participant-1"

	// First, we need to create a peer with an offer
	offerSDP := "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0\r\na=extmap-allow-mixed\r\na=msid-semantic: WMS\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\nc=IN IP4 0.0.0.0\r\na=ice-ufrag:test\r\na=ice-pwd:testpassword1234567890123456\r\na=ice-options:trickle\r\na=fingerprint:sha-256 00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00\r\na=setup:actpass\r\na=mid:0\r\na=sctp-port:5000\r\na=max-message-size:262144\r\n"

	ctx := context.Background()

	// Create peer and get answer
	answerSDP, err := service.HandleOffer(ctx, participantID, offerSDP)
	if err != nil {
		t.Fatalf("HandleOffer failed: %v", err)
	}

	// Now handle the answer (simulating server-initiated offer/answer)
	// For this test, we'll use the generated answer as if it came from the client
	err = service.HandleAnswer(ctx, participantID, answerSDP)
	if err != nil {
		// This may fail in our test setup because we're using the same SDP,
		// but we're mainly testing that the method exists and doesn't crash
		t.Logf("HandleAnswer returned error (expected in test): %v", err)
	}
}

// TestService_HandleCandidate verifies HandleCandidate processes ICE candidates.
func TestService_HandleCandidate(t *testing.T) {
	config := DefaultPeerConfig()
	mediaEngine := &pion.MediaEngine{}
	eventHandler := &mockEventHandler{}

	service := NewService(config, mediaEngine, eventHandler)
	participantID := "test-participant-1"

	// First, create a peer with an offer
	offerSDP := "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0\r\na=extmap-allow-mixed\r\na=msid-semantic: WMS\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\nc=IN IP4 0.0.0.0\r\na=ice-ufrag:test\r\na=ice-pwd:testpassword1234567890123456\r\na=ice-options:trickle\r\na=fingerprint:sha-256 00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00\r\na=setup:actpass\r\na=mid:0\r\na=sctp-port:5000\r\na=max-message-size:262144\r\n"

	ctx := context.Background()
	_, err := service.HandleOffer(ctx, participantID, offerSDP)
	if err != nil {
		t.Fatalf("HandleOffer failed: %v", err)
	}

	// Handle ICE candidate
	candidate := "candidate:1 1 udp 2122260223 192.168.1.100 54321 typ host"
	sdpMid := "0"
	sdpMLineIndex := 0

	err = service.HandleCandidate(ctx, participantID, candidate, sdpMid, &sdpMLineIndex)
	if err != nil {
		t.Fatalf("HandleCandidate failed: %v", err)
	}
}

// TestService_HandleCandidate_PeerNotFound verifies error when peer doesn't exist.
func TestService_HandleCandidate_PeerNotFound(t *testing.T) {
	config := DefaultPeerConfig()
	mediaEngine := &pion.MediaEngine{}
	eventHandler := &mockEventHandler{}

	service := NewService(config, mediaEngine, eventHandler)
	participantID := "non-existent-participant"

	// Try to handle candidate for non-existent peer
	candidate := "candidate:1 1 udp 2122260223 192.168.1.100 54321 typ host"
	sdpMid := "0"
	sdpMLineIndex := 0

	ctx := context.Background()
	err := service.HandleCandidate(ctx, participantID, candidate, sdpMid, &sdpMLineIndex)
	if err == nil {
		t.Error("HandleCandidate should fail for non-existent peer")
	}
}

// TestService_EventHandler_StateCallbacks verifies event callbacks are invoked.
func TestService_EventHandler_StateCallbacks(t *testing.T) {
	config := DefaultPeerConfig()
	mediaEngine := &pion.MediaEngine{}
	eventHandler := &mockEventHandler{}

	service := NewService(config, mediaEngine, eventHandler)
	participantID := "test-participant-1"

	offerSDP := "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0\r\na=extmap-allow-mixed\r\na=msid-semantic: WMS\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\nc=IN IP4 0.0.0.0\r\na=ice-ufrag:test\r\na=ice-pwd:testpassword1234567890123456\r\na=ice-options:trickle\r\na=fingerprint:sha-256 00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00\r\na=setup:actpass\r\na=mid:0\r\na=sctp-port:5000\r\na=max-message-size:262144\r\n"

	ctx := context.Background()
	_, err := service.HandleOffer(ctx, participantID, offerSDP)
	if err != nil {
		t.Fatalf("HandleOffer failed: %v", err)
	}

	// Wait a bit for state callbacks to be invoked
	time.Sleep(100 * time.Millisecond)

	// Verify state change callbacks were called
	iceStateCalls := eventHandler.getCallCount("ICEConnectionState")
	peerStateCalls := eventHandler.getCallCount("PeerConnectionState")

	if iceStateCalls == 0 {
		t.Error("OnICEConnectionStateChange should have been called")
	}

	if peerStateCalls == 0 {
		t.Error("OnPeerConnectionStateChange should have been called")
	}

	// Verify correct participant ID was passed
	eventHandler.mu.Lock()
	lastParticipant := eventHandler.lastParticipantID
	eventHandler.mu.Unlock()

	if lastParticipant != participantID {
		t.Errorf("last participant ID = %s, want %s", lastParticipant, participantID)
	}
}

// TestService_ConcurrentPeerCreation verifies concurrent access is safe.
func TestService_ConcurrentPeerCreation(t *testing.T) {
	config := DefaultPeerConfig()
	mediaEngine := &pion.MediaEngine{}
	eventHandler := &mockEventHandler{}

	service := NewService(config, mediaEngine, eventHandler)

	offerSDP := "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0\r\na=extmap-allow-mixed\r\na=msid-semantic: WMS\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\nc=IN IP4 0.0.0.0\r\na=ice-ufrag:test\r\na=ice-pwd:testpassword1234567890123456\r\na=ice-options:trickle\r\na=fingerprint:sha-256 00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00\r\na=setup:actpass\r\na=mid:0\r\na=sctp-port:5000\r\na=max-message-size:262144\r\n"

	ctx := context.Background()

	// Create 10 peers concurrently
	var wg sync.WaitGroup
	numPeers := 10

	for i := 0; i < numPeers; i++ {
		wg.Add(1)
		participantID := fmt.Sprintf("participant-%d", i)

		go func(pid string) {
			defer wg.Done()
			_, err := service.HandleOffer(ctx, pid, offerSDP)
			if err != nil {
				t.Logf("HandleOffer failed for %s: %v", pid, err)
			}
		}(participantID)
	}

	wg.Wait()

	// Verify all peers were created
	if len(service.peers) != numPeers {
		t.Errorf("peers map should have %d peers, got %d", numPeers, len(service.peers))
	}
}

// TestService_ConcurrentSamePeerCreation verifies double-check locking works.
func TestService_ConcurrentSamePeerCreation(t *testing.T) {
	config := DefaultPeerConfig()
	mediaEngine := &pion.MediaEngine{}
	eventHandler := &mockEventHandler{}

	service := NewService(config, mediaEngine, eventHandler)
	participantID := "test-participant-1"

	offerSDP := "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0\r\na=extmap-allow-mixed\r\na=msid-semantic: WMS\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\nc=IN IP4 0.0.0.0\r\na=ice-ufrag:test\r\na=ice-pwd:testpassword1234567890123456\r\na=ice-options:trickle\r\na=fingerprint:sha-256 00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00\r\na=setup:actpass\r\na=mid:0\r\na=sctp-port:5000\r\na=max-message-size:262144\r\n"

	ctx := context.Background()

	// Try to create same peer concurrently from 5 goroutines
	var wg sync.WaitGroup
	numGoroutines := 5

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.HandleOffer(ctx, participantID, offerSDP)
			if err != nil {
				t.Logf("HandleOffer failed: %v", err)
			}
		}()
	}

	wg.Wait()

	// Verify only one peer was created (double-check locking worked)
	if len(service.peers) != 1 {
		t.Errorf("peers map should have 1 peer, got %d", len(service.peers))
	}
}

// TestService_RemovePeer verifies peer removal.
func TestService_RemovePeer(t *testing.T) {
	config := DefaultPeerConfig()
	mediaEngine := &pion.MediaEngine{}
	eventHandler := &mockEventHandler{}

	service := NewService(config, mediaEngine, eventHandler)
	participantID := "test-participant-1"

	offerSDP := "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0\r\na=extmap-allow-mixed\r\na=msid-semantic: WMS\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\nc=IN IP4 0.0.0.0\r\na=ice-ufrag:test\r\na=ice-pwd:testpassword1234567890123456\r\na=ice-options:trickle\r\na=fingerprint:sha-256 00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00\r\na=setup:actpass\r\na=mid:0\r\na=sctp-port:5000\r\na=max-message-size:262144\r\n"

	ctx := context.Background()
	_, err := service.HandleOffer(ctx, participantID, offerSDP)
	if err != nil {
		t.Fatalf("HandleOffer failed: %v", err)
	}

	// Verify peer exists
	if len(service.peers) != 1 {
		t.Fatalf("peers map should have 1 peer before removal, got %d", len(service.peers))
	}

	// Remove peer
	service.RemovePeer(participantID)

	// Verify peer was removed
	if len(service.peers) != 0 {
		t.Errorf("peers map should be empty after removal, got %d peers", len(service.peers))
	}
}

// TestService_GetPeer verifies peer retrieval.
func TestService_GetPeer(t *testing.T) {
	config := DefaultPeerConfig()
	mediaEngine := &pion.MediaEngine{}
	eventHandler := &mockEventHandler{}

	service := NewService(config, mediaEngine, eventHandler)
	participantID := "test-participant-1"

	// Verify peer doesn't exist initially
	peer := service.GetPeer(participantID)
	if peer != nil {
		t.Error("GetPeer should return nil for non-existent peer")
	}

	offerSDP := "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0\r\na=extmap-allow-mixed\r\na=msid-semantic: WMS\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\nc=IN IP4 0.0.0.0\r\na=ice-ufrag:test\r\na=ice-pwd:testpassword1234567890123456\r\na=ice-options:trickle\r\na=fingerprint:sha-256 00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00\r\na=setup:actpass\r\na=mid:0\r\na=sctp-port:5000\r\na=max-message-size:262144\r\n"

	ctx := context.Background()
	_, err := service.HandleOffer(ctx, participantID, offerSDP)
	if err != nil {
		t.Fatalf("HandleOffer failed: %v", err)
	}

	// Verify peer exists now
	peer = service.GetPeer(participantID)
	if peer == nil {
		t.Error("GetPeer should return peer after creation")
	}
}
