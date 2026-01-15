package webrtc

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func newTestMediaEngine() *webrtc.MediaEngine {
	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		panic(err)
	}
	return m
}

func TestPeerState_String(t *testing.T) {
	tests := []struct {
		state    PeerState
		expected string
	}{
		{PeerStateNew, "new"},
		{PeerStateConnecting, "connecting"},
		{PeerStateConnected, "connected"},
		{PeerStateDisconnected, "disconnected"},
		{PeerStateFailed, "failed"},
		{PeerStateClosed, "closed"},
		{PeerState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("PeerState.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDefaultPeerConfig(t *testing.T) {
	config := DefaultPeerConfig()

	if !config.ICELite {
		t.Error("expected ICELite to be true by default")
	}

	if config.ConnectionTimeout != 30*time.Second {
		t.Errorf("expected ConnectionTimeout to be 30s, got %v", config.ConnectionTimeout)
	}
}

func TestNewPeer(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer-1", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}
	defer peer.Close()

	if peer.ID() != "test-peer-1" {
		t.Errorf("expected peer ID test-peer-1, got %s", peer.ID())
	}

	if peer.State() != PeerStateNew {
		t.Errorf("expected peer state new, got %s", peer.State())
	}

	if peer.PeerConnection() == nil {
		t.Error("expected non-nil peer connection")
	}
}

func TestPeer_CreatedAt(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	before := time.Now()
	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}
	defer peer.Close()
	after := time.Now()

	createdAt := peer.CreatedAt()
	if createdAt.Before(before) || createdAt.After(after) {
		t.Errorf("CreatedAt should be between %v and %v, got %v", before, after, createdAt)
	}
}

func TestPeer_Duration(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	duration := peer.Duration()
	if duration < 10*time.Millisecond {
		t.Errorf("expected duration >= 10ms, got %v", duration)
	}

	peer.Close()
	closedDuration := peer.Duration()
	if closedDuration < duration {
		t.Errorf("expected closedDuration >= duration, got %v < %v", closedDuration, duration)
	}
}

func TestPeer_Close(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}

	if err := peer.Close(); err != nil {
		t.Errorf("failed to close peer: %v", err)
	}

	if peer.State() != PeerStateClosed {
		t.Errorf("expected peer state closed, got %s", peer.State())
	}

	// Close again should not error
	if err := peer.Close(); err != nil {
		t.Errorf("second close should not error: %v", err)
	}
}

func TestPeer_CloseWithContext(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := peer.CloseWithContext(ctx); err != nil {
		t.Errorf("failed to close peer with context: %v", err)
	}

	if peer.State() != PeerStateClosed {
		t.Errorf("expected peer state closed, got %s", peer.State())
	}
}

func TestPeer_CloseWithContext_Timeout(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}
	defer peer.Close()

	// Create an already cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = peer.CloseWithContext(ctx)
	// Either context.Canceled or nil (if close completes before context check)
	if err != nil && err != context.Canceled {
		t.Errorf("expected context.Canceled or nil, got %v", err)
	}
}

func TestPeer_OnTrackHandler(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}
	defer peer.Close()

	peer.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		// Handler is set
	})

	// Verify handler is set (we can't easily test it being called without a full connection)
	peer.mu.RLock()
	hasHandler := peer.onTrack != nil
	peer.mu.RUnlock()

	if !hasHandler {
		t.Error("expected onTrack handler to be set")
	}
}

func TestPeer_OnICECandidateHandler(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}
	defer peer.Close()

	peer.OnICECandidate(func(candidate *webrtc.ICECandidate) {})

	peer.mu.RLock()
	hasHandler := peer.onICECandidate != nil
	peer.mu.RUnlock()

	if !hasHandler {
		t.Error("expected onICECandidate handler to be set")
	}
}

func TestPeer_OnConnectionStateChangeHandler(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}
	defer peer.Close()

	peer.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {})

	peer.mu.RLock()
	hasHandler := peer.onConnectionStateChange != nil
	peer.mu.RUnlock()

	if !hasHandler {
		t.Error("expected onConnectionStateChange handler to be set")
	}
}

func TestPeer_OnICEConnectionStateChangeHandler(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}
	defer peer.Close()

	peer.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {})

	peer.mu.RLock()
	hasHandler := peer.onICEConnectionState != nil
	peer.mu.RUnlock()

	if !hasHandler {
		t.Error("expected onICEConnectionState handler to be set")
	}
}

func TestPeer_OnDataChannelHandler(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}
	defer peer.Close()

	peer.OnDataChannel(func(dc *webrtc.DataChannel) {})

	peer.mu.RLock()
	hasHandler := peer.onDataChannel != nil
	peer.mu.RUnlock()

	if !hasHandler {
		t.Error("expected onDataChannel handler to be set")
	}
}

func TestPeer_OnNegotiationNeededHandler(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}
	defer peer.Close()

	peer.OnNegotiationNeeded(func() {})

	peer.mu.RLock()
	hasHandler := peer.onNegotiationNeeded != nil
	peer.mu.RUnlock()

	if !hasHandler {
		t.Error("expected onNegotiationNeeded handler to be set")
	}
}

func TestPeer_OnICECandidateErrorHandler(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}
	defer peer.Close()

	peer.OnICECandidateError(func(candidate *webrtc.ICECandidateInit, err error) {})

	peer.mu.RLock()
	hasHandler := peer.onICECandidateError != nil
	peer.mu.RUnlock()

	if !hasHandler {
		t.Error("expected onICECandidateError handler to be set")
	}
}

func TestPeer_OnICECandidateError_Called(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	// Disable ICE Lite for this test
	config := PeerConfig{
		ICELite:           false,
		ConnectionTimeout: 30 * time.Second,
	}

	// Create offer peer to generate valid SDP
	offerPeer, err := NewPeer("offer-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create offer peer: %v", err)
	}
	defer offerPeer.Close()

	// Add a transceiver
	_, err = offerPeer.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendrecv,
	})
	if err != nil {
		t.Fatalf("failed to add transceiver: %v", err)
	}

	offer, err := offerPeer.CreateOffer()
	if err != nil {
		t.Fatalf("failed to create offer: %v", err)
	}

	// Create answer peer
	answerPeer, err := NewPeer("answer-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create answer peer: %v", err)
	}
	defer answerPeer.Close()

	// Track error handler calls
	var handlerCalled bool
	var mu sync.Mutex
	answerPeer.OnICECandidateError(func(candidate *webrtc.ICECandidateInit, err error) {
		mu.Lock()
		handlerCalled = true
		mu.Unlock()
	})

	// Add an invalid candidate before setting remote description
	invalidCandidate := webrtc.ICECandidateInit{
		Candidate: "invalid candidate string",
	}
	if err := answerPeer.AddICECandidate(invalidCandidate); err != nil {
		t.Fatalf("AddICECandidate should queue candidate, got error: %v", err)
	}

	// Set remote description - this should process pending candidates
	if err := answerPeer.SetRemoteDescription(offer); err != nil {
		t.Fatalf("failed to set remote description: %v", err)
	}

	// Check if handler was called
	mu.Lock()
	called := handlerCalled
	mu.Unlock()

	if !called {
		t.Error("expected OnICECandidateError handler to be called for invalid candidate")
	}
}

func TestPeer_OnICECandidateError_NotSet(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	// Disable ICE Lite for this test
	config := PeerConfig{
		ICELite:           false,
		ConnectionTimeout: 30 * time.Second,
	}

	// Create offer peer to generate valid SDP
	offerPeer, err := NewPeer("offer-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create offer peer: %v", err)
	}
	defer offerPeer.Close()

	// Add a transceiver
	_, err = offerPeer.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendrecv,
	})
	if err != nil {
		t.Fatalf("failed to add transceiver: %v", err)
	}

	offer, err := offerPeer.CreateOffer()
	if err != nil {
		t.Fatalf("failed to create offer: %v", err)
	}

	// Create answer peer WITHOUT error handler
	answerPeer, err := NewPeer("answer-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create answer peer: %v", err)
	}
	defer answerPeer.Close()

	// Add an invalid candidate before setting remote description
	invalidCandidate := webrtc.ICECandidateInit{
		Candidate: "invalid candidate string",
	}
	if err := answerPeer.AddICECandidate(invalidCandidate); err != nil {
		t.Fatalf("AddICECandidate should queue candidate, got error: %v", err)
	}

	// Set remote description - should not panic even without handler
	if err := answerPeer.SetRemoteDescription(offer); err != nil {
		t.Fatalf("SetRemoteDescription should not fail, got: %v", err)
	}
}

func TestPeer_Close_Idempotent(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}

	// First close
	err1 := peer.Close()

	// Second close should return same error (nil in this case)
	err2 := peer.Close()

	if err1 != err2 {
		t.Errorf("expected idempotent Close, got err1=%v, err2=%v", err1, err2)
	}

	if peer.State() != PeerStateClosed {
		t.Errorf("expected peer state closed, got %s", peer.State())
	}
}

func TestPeer_Close_Concurrent(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}

	var wg sync.WaitGroup
	errors := make([]error, 10)

	// Concurrent close calls
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errors[idx] = peer.Close()
		}(i)
	}

	wg.Wait()

	// All calls should return the same error
	for i := 1; i < 10; i++ {
		if errors[i] != errors[0] {
			t.Errorf("expected all Close calls to return same error, got errors[0]=%v, errors[%d]=%v", errors[0], i, errors[i])
		}
	}

	if peer.State() != PeerStateClosed {
		t.Errorf("expected peer state closed, got %s", peer.State())
	}
}

func TestPeer_CreateOffer(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}
	defer peer.Close()

	offer, err := peer.CreateOffer()
	if err != nil {
		t.Fatalf("failed to create offer: %v", err)
	}

	if offer.Type != webrtc.SDPTypeOffer {
		t.Errorf("expected offer type, got %v", offer.Type)
	}

	if offer.SDP == "" {
		t.Error("expected non-empty SDP")
	}

	// Verify local description is set
	localDesc := peer.LocalDescription()
	if localDesc == nil {
		t.Error("expected local description to be set")
	}
}

func TestPeer_CreateOffer_Closed(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}

	peer.Close()

	_, err = peer.CreateOffer()
	if err != ErrPeerClosed {
		t.Errorf("expected ErrPeerClosed, got %v", err)
	}
}

func TestPeer_SetRemoteDescription_And_CreateAnswer(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	// Disable ICE Lite for this test to allow proper offer/answer exchange
	config := PeerConfig{
		ICELite:           false,
		ConnectionTimeout: 30 * time.Second,
	}

	// Create two peers for offer/answer exchange
	offerPeer, err := NewPeer("offer-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create offer peer: %v", err)
	}
	defer offerPeer.Close()

	answerPeer, err := NewPeer("answer-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create answer peer: %v", err)
	}
	defer answerPeer.Close()

	// Add a transceiver to generate valid SDP
	_, err = offerPeer.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendrecv,
	})
	if err != nil {
		t.Fatalf("failed to add transceiver: %v", err)
	}

	// Create offer
	offer, err := offerPeer.CreateOffer()
	if err != nil {
		t.Fatalf("failed to create offer: %v", err)
	}

	// Set remote description on answer peer
	if err := answerPeer.SetRemoteDescription(offer); err != nil {
		t.Fatalf("failed to set remote description: %v", err)
	}

	// Create answer
	answer, err := answerPeer.CreateAnswer()
	if err != nil {
		t.Fatalf("failed to create answer: %v", err)
	}

	if answer.Type != webrtc.SDPTypeAnswer {
		t.Errorf("expected answer type, got %v", answer.Type)
	}

	if answer.SDP == "" {
		t.Error("expected non-empty SDP")
	}
}

func TestPeer_AddICECandidate_BeforeRemoteDesc(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}
	defer peer.Close()

	// Add candidate before remote description is set
	candidate := webrtc.ICECandidateInit{
		Candidate: "candidate:1234 1 udp 2130706431 192.168.1.1 54321 typ host",
	}

	err = peer.AddICECandidate(candidate)
	if err != nil {
		t.Errorf("AddICECandidate should not error before remote desc: %v", err)
	}

	// Verify candidate is queued
	peer.mu.RLock()
	pendingCount := len(peer.pendingCandidates)
	peer.mu.RUnlock()

	if pendingCount != 1 {
		t.Errorf("expected 1 pending candidate, got %d", pendingCount)
	}
}

func TestPeer_AddICECandidate_Closed(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}

	peer.Close()

	candidate := webrtc.ICECandidateInit{
		Candidate: "candidate:1234 1 udp 2130706431 192.168.1.1 54321 typ host",
	}

	err = peer.AddICECandidate(candidate)
	if err != ErrPeerClosed {
		t.Errorf("expected ErrPeerClosed, got %v", err)
	}
}

func TestPeer_SignalingState(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}
	defer peer.Close()

	state := peer.SignalingState()
	if state != webrtc.SignalingStateStable {
		t.Errorf("expected stable signaling state, got %v", state)
	}
}

func TestPeer_ICEConnectionState(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}
	defer peer.Close()

	state := peer.ICEConnectionState()
	if state != webrtc.ICEConnectionStateNew {
		t.Errorf("expected new ICE connection state, got %v", state)
	}
}

func TestPeer_ConnectionState(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}
	defer peer.Close()

	state := peer.ConnectionState()
	if state != webrtc.PeerConnectionStateNew {
		t.Errorf("expected new connection state, got %v", state)
	}
}

func TestPeer_GetSendersAndReceivers(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}
	defer peer.Close()

	// Initially may be empty, but should not panic
	senders := peer.GetSenders()
	// Empty slice is acceptable
	if senders != nil && len(senders) < 0 {
		t.Error("unexpected negative length")
	}

	receivers := peer.GetReceivers()
	// Empty slice is acceptable
	if receivers != nil && len(receivers) < 0 {
		t.Error("unexpected negative length")
	}
}

func TestPeer_GetTransceivers(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}
	defer peer.Close()

	// Initially may be empty, but should not panic
	transceivers := peer.GetTransceivers()
	// Empty slice is acceptable
	if transceivers != nil && len(transceivers) < 0 {
		t.Error("unexpected negative length")
	}
}

func TestPeer_AddTransceiverFromKind(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}
	defer peer.Close()

	transceiver, err := peer.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	if err != nil {
		t.Fatalf("failed to add transceiver: %v", err)
	}

	if transceiver == nil {
		t.Error("expected non-nil transceiver")
	}

	transceivers := peer.GetTransceivers()
	if len(transceivers) != 1 {
		t.Errorf("expected 1 transceiver, got %d", len(transceivers))
	}
}

func TestPeer_AddTransceiverFromKind_Closed(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}

	peer.Close()

	_, err = peer.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	if err != ErrPeerClosed {
		t.Errorf("expected ErrPeerClosed, got %v", err)
	}
}

func TestPeer_ConcurrentAccess(t *testing.T) {
	mediaEngine := newTestMediaEngine()
	config := DefaultPeerConfig()

	peer, err := NewPeer("test-peer", config, mediaEngine)
	if err != nil {
		t.Fatalf("failed to create peer: %v", err)
	}
	defer peer.Close()

	var wg sync.WaitGroup

	// Concurrent handler registration
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			peer.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {})
			peer.OnICECandidate(func(candidate *webrtc.ICECandidate) {})
			peer.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {})
		}()
	}

	// Concurrent state reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = peer.State()
			_ = peer.ID()
			_ = peer.ConnectionState()
		}()
	}

	wg.Wait()
}

func TestNewPeer_NilMediaEngine(t *testing.T) {
	config := DefaultPeerConfig()

	_, err := NewPeer("test-peer", config, nil)
	if err != ErrNilMediaEngine {
		t.Errorf("expected ErrNilMediaEngine, got %v", err)
	}
}

func TestPeer_NilPeerConnection(t *testing.T) {
	peer := &Peer{id: "test"}

	if _, err := peer.CreateOffer(); err != ErrNoPeerConnection {
		t.Errorf("expected ErrNoPeerConnection, got %v", err)
	}

	if _, err := peer.CreateAnswer(); err != ErrNoPeerConnection {
		t.Errorf("expected ErrNoPeerConnection, got %v", err)
	}

	if err := peer.SetRemoteDescription(webrtc.SessionDescription{}); err != ErrNoPeerConnection {
		t.Errorf("expected ErrNoPeerConnection, got %v", err)
	}

	if err := peer.AddICECandidate(webrtc.ICECandidateInit{}); err != ErrNoPeerConnection {
		t.Errorf("expected ErrNoPeerConnection, got %v", err)
	}

	if _, err := peer.AddTrack(nil); err != ErrNoPeerConnection {
		t.Errorf("expected ErrNoPeerConnection, got %v", err)
	}

	if err := peer.RemoveTrack(nil); err != ErrNoPeerConnection {
		t.Errorf("expected ErrNoPeerConnection, got %v", err)
	}

	if _, err := peer.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo); err != ErrNoPeerConnection {
		t.Errorf("expected ErrNoPeerConnection, got %v", err)
	}

	if err := peer.RestartICE(); err != ErrNoPeerConnection {
		t.Errorf("expected ErrNoPeerConnection, got %v", err)
	}

	// These should return nil/zero values
	if peer.LocalDescription() != nil {
		t.Error("expected nil LocalDescription")
	}

	if peer.RemoteDescription() != nil {
		t.Error("expected nil RemoteDescription")
	}

	if peer.GetSenders() != nil {
		t.Error("expected nil GetSenders")
	}

	if peer.GetReceivers() != nil {
		t.Error("expected nil GetReceivers")
	}

	if peer.GetTransceivers() != nil {
		t.Error("expected nil GetTransceivers")
	}
}
