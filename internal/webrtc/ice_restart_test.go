package webrtc

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func TestDefaultICERestartConfig(t *testing.T) {
	config := DefaultICERestartConfig()

	if config.Timeout != DefaultICERestartTimeout {
		t.Errorf("expected timeout %v, got %v", DefaultICERestartTimeout, config.Timeout)
	}
	if config.DisconnectedThreshold != DefaultDisconnectedThreshold {
		t.Errorf("expected disconnected threshold %v, got %v", DefaultDisconnectedThreshold, config.DisconnectedThreshold)
	}
	if config.MaxAttempts != DefaultMaxRestartAttempts {
		t.Errorf("expected max attempts %d, got %d", DefaultMaxRestartAttempts, config.MaxAttempts)
	}
	if config.Backoff != DefaultRestartBackoff {
		t.Errorf("expected backoff %v, got %v", DefaultRestartBackoff, config.Backoff)
	}
	if !config.AutoRestartOnDisconnect {
		t.Error("expected AutoRestartOnDisconnect to be true")
	}
	if !config.AutoRestartOnFailed {
		t.Error("expected AutoRestartOnFailed to be true")
	}
}

func TestNewICERestartManager(t *testing.T) {
	t.Run("with default config", func(t *testing.T) {
		m := NewICERestartManager(DefaultICERestartConfig())
		if m == nil {
			t.Fatal("expected non-nil manager")
		}
		defer m.Close()

		if m.config.Timeout != DefaultICERestartTimeout {
			t.Errorf("expected timeout %v, got %v", DefaultICERestartTimeout, m.config.Timeout)
		}
	})

	t.Run("with zero values uses defaults", func(t *testing.T) {
		m := NewICERestartManager(ICERestartConfig{})
		if m == nil {
			t.Fatal("expected non-nil manager")
		}
		defer m.Close()

		if m.config.Timeout != DefaultICERestartTimeout {
			t.Errorf("expected timeout %v, got %v", DefaultICERestartTimeout, m.config.Timeout)
		}
		if m.config.MaxAttempts != DefaultMaxRestartAttempts {
			t.Errorf("expected max attempts %d, got %d", DefaultMaxRestartAttempts, m.config.MaxAttempts)
		}
	})
}

func TestICERestartManager_SetPeerConnection(t *testing.T) {
	m := NewICERestartManager(DefaultICERestartConfig())
	defer m.Close()

	// Create a real peer connection for testing
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("failed to create peer connection: %v", err)
	}
	defer pc.Close()

	m.SetPeerConnection(pc)

	// Verify PC was set
	m.mu.RLock()
	if m.pc != pc {
		t.Error("peer connection was not set correctly")
	}
	m.mu.RUnlock()
}

func TestICERestartManager_SetOfferHandler(t *testing.T) {
	m := NewICERestartManager(DefaultICERestartConfig())
	defer m.Close()

	handler := func(ctx context.Context, offer webrtc.SessionDescription) error {
		return nil
	}

	m.SetOfferHandler(handler)

	m.mu.RLock()
	if m.offerHandler == nil {
		t.Error("offer handler was not set")
	}
	m.mu.RUnlock()
}

func TestICERestartManager_Callbacks(t *testing.T) {
	m := NewICERestartManager(DefaultICERestartConfig())
	defer m.Close()

	t.Run("OnRestartStarted", func(t *testing.T) {
		m.OnRestartStarted(func(reason string) {
			// Callback would be called during restart
			_ = reason
		})

		m.mu.RLock()
		if m.onRestartStarted == nil {
			t.Error("OnRestartStarted callback was not set")
		}
		m.mu.RUnlock()
	})

	t.Run("OnRestartCompleted", func(t *testing.T) {
		m.OnRestartCompleted(func(result ICERestartResult) {
			// Callback would be called after restart
			_ = result
		})

		m.mu.RLock()
		if m.onRestartCompleted == nil {
			t.Error("OnRestartCompleted callback was not set")
		}
		m.mu.RUnlock()
	})
}

func TestICERestartManager_TriggerRestart_NoPeerConnection(t *testing.T) {
	m := NewICERestartManager(DefaultICERestartConfig())
	defer m.Close()

	// Set handler but no PC
	m.SetOfferHandler(func(ctx context.Context, offer webrtc.SessionDescription) error {
		return nil
	})

	result := m.TriggerRestart(context.Background(), ICERestartReasonExplicit)

	if result.Success {
		t.Error("expected restart to fail without peer connection")
	}
	if !errors.Is(result.Error, ErrNoPeerConnection) {
		t.Errorf("expected ErrNoPeerConnection, got %v", result.Error)
	}
	if result.Reason != ICERestartReasonExplicit {
		t.Errorf("expected reason %s, got %s", ICERestartReasonExplicit, result.Reason)
	}
}

func TestICERestartManager_TriggerRestart_NoOfferHandler(t *testing.T) {
	m := NewICERestartManager(DefaultICERestartConfig())
	defer m.Close()

	// Create a real peer connection
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("failed to create peer connection: %v", err)
	}
	defer pc.Close()

	m.SetPeerConnection(pc)
	// No offer handler set

	result := m.TriggerRestart(context.Background(), ICERestartReasonExplicit)

	if result.Success {
		t.Error("expected restart to fail without offer handler")
	}
	if !errors.Is(result.Error, ErrNoOfferHandler) {
		t.Errorf("expected ErrNoOfferHandler, got %v", result.Error)
	}
}

func TestICERestartManager_TriggerRestart_Success(t *testing.T) {
	m := NewICERestartManager(ICERestartConfig{
		Timeout:     5 * time.Second,
		MaxAttempts: 1,
	})
	defer m.Close()

	// Create two peer connections and complete negotiation
	pc1, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("failed to create peer connection 1: %v", err)
	}
	defer pc1.Close()

	pc2, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("failed to create peer connection 2: %v", err)
	}
	defer pc2.Close()

	// Add transceiver to pc1
	_, err = pc1.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	if err != nil {
		t.Fatalf("failed to add transceiver: %v", err)
	}

	// Complete offer/answer exchange
	offer, err := pc1.CreateOffer(nil)
	if err != nil {
		t.Fatalf("failed to create offer: %v", err)
	}
	if err := pc1.SetLocalDescription(offer); err != nil {
		t.Fatalf("failed to set local description on pc1: %v", err)
	}
	if err := pc2.SetRemoteDescription(offer); err != nil {
		t.Fatalf("failed to set remote description on pc2: %v", err)
	}

	answer, err := pc2.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("failed to create answer: %v", err)
	}
	if err := pc2.SetLocalDescription(answer); err != nil {
		t.Fatalf("failed to set local description on pc2: %v", err)
	}
	if err := pc1.SetRemoteDescription(answer); err != nil {
		t.Fatalf("failed to set remote description on pc1: %v", err)
	}

	m.SetPeerConnection(pc1)

	handlerCalled := false
	m.SetOfferHandler(func(ctx context.Context, offer webrtc.SessionDescription) error {
		handlerCalled = true
		// In a real scenario, this would send offer to remote and set answer
		return nil
	})

	startedCalled := false
	m.OnRestartStarted(func(reason string) {
		startedCalled = true
	})

	completedCalled := false
	m.OnRestartCompleted(func(result ICERestartResult) {
		completedCalled = true
	})

	result := m.TriggerRestart(context.Background(), ICERestartReasonExplicit)

	if !result.Success {
		t.Errorf("expected restart to succeed, got error: %v", result.Error)
	}
	if result.Offer == nil {
		t.Error("expected offer to be set")
	}
	if !handlerCalled {
		t.Error("expected offer handler to be called")
	}
	if !startedCalled {
		t.Error("expected OnRestartStarted to be called")
	}
	if !completedCalled {
		t.Error("expected OnRestartCompleted to be called")
	}
	if result.Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", result.Attempts)
	}
	if result.Duration == 0 {
		t.Error("expected non-zero duration")
	}
}

func TestICERestartManager_TriggerRestart_InProgress(t *testing.T) {
	m := NewICERestartManager(ICERestartConfig{
		Timeout:     10 * time.Second,
		MaxAttempts: 1,
	})
	defer m.Close()

	// Create two peer connections and complete negotiation
	pc1, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("failed to create peer connection 1: %v", err)
	}
	defer pc1.Close()

	pc2, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("failed to create peer connection 2: %v", err)
	}
	defer pc2.Close()

	_, err = pc1.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	if err != nil {
		t.Fatalf("failed to add transceiver: %v", err)
	}

	offer, err := pc1.CreateOffer(nil)
	if err != nil {
		t.Fatalf("failed to create offer: %v", err)
	}
	if err := pc1.SetLocalDescription(offer); err != nil {
		t.Fatalf("failed to set local description on pc1: %v", err)
	}
	if err := pc2.SetRemoteDescription(offer); err != nil {
		t.Fatalf("failed to set remote description on pc2: %v", err)
	}
	answer, err := pc2.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("failed to create answer: %v", err)
	}
	if err := pc2.SetLocalDescription(answer); err != nil {
		t.Fatalf("failed to set local description on pc2: %v", err)
	}
	if err := pc1.SetRemoteDescription(answer); err != nil {
		t.Fatalf("failed to set remote description on pc1: %v", err)
	}

	m.SetPeerConnection(pc1)

	// Handler that blocks
	blockCh := make(chan struct{})
	m.SetOfferHandler(func(ctx context.Context, offer webrtc.SessionDescription) error {
		<-blockCh
		return nil
	})

	// Start first restart in background
	done := make(chan ICERestartResult)
	go func() {
		done <- m.TriggerRestart(context.Background(), ICERestartReasonExplicit)
	}()

	// Give first restart time to start
	time.Sleep(100 * time.Millisecond)

	// Try second restart while first is in progress
	result := m.TriggerRestart(context.Background(), ICERestartReasonExplicit)

	if result.Success {
		t.Error("expected second restart to fail")
	}
	if !errors.Is(result.Error, ErrICERestartInProgress) {
		t.Errorf("expected ErrICERestartInProgress, got %v", result.Error)
	}

	// Unblock first restart
	close(blockCh)
	<-done
}

func TestICERestartManager_TriggerRestart_Timeout(t *testing.T) {
	m := NewICERestartManager(ICERestartConfig{
		Timeout:     100 * time.Millisecond,
		MaxAttempts: 3,
		Backoff:     50 * time.Millisecond,
	})
	defer m.Close()

	// Create a real peer connection
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("failed to create peer connection: %v", err)
	}
	defer pc.Close()

	m.SetPeerConnection(pc)

	// Handler that always fails
	m.SetOfferHandler(func(ctx context.Context, offer webrtc.SessionDescription) error {
		return errors.New("simulated failure")
	})

	result := m.TriggerRestart(context.Background(), ICERestartReasonExplicit)

	if result.Success {
		t.Error("expected restart to fail")
	}
	// Should either timeout or exhaust attempts
	if result.Error == nil {
		t.Error("expected error to be set")
	}
}

func TestICERestartManager_TriggerRestart_ContextCanceled(t *testing.T) {
	m := NewICERestartManager(ICERestartConfig{
		Timeout:     10 * time.Second,
		MaxAttempts: 1,
	})
	defer m.Close()

	// Create a real peer connection
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("failed to create peer connection: %v", err)
	}
	defer pc.Close()

	m.SetPeerConnection(pc)

	// Handler that blocks
	m.SetOfferHandler(func(ctx context.Context, offer webrtc.SessionDescription) error {
		<-ctx.Done()
		return ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Start restart
	done := make(chan ICERestartResult)
	go func() {
		done <- m.TriggerRestart(ctx, ICERestartReasonExplicit)
	}()

	// Cancel context
	time.Sleep(50 * time.Millisecond)
	cancel()

	result := <-done

	if result.Success {
		t.Error("expected restart to fail when context canceled")
	}
}

func TestICERestartManager_HandleICEConnectionStateChange(t *testing.T) {
	tests := []struct {
		name                 string
		state                webrtc.ICEConnectionState
		autoOnDisconnect     bool
		autoOnFailed         bool
		expectTimer          bool
		expectTriggerRestart bool
	}{
		{
			name:                 "disconnected with auto restart",
			state:                webrtc.ICEConnectionStateDisconnected,
			autoOnDisconnect:     true,
			autoOnFailed:         true,
			expectTimer:          true,
			expectTriggerRestart: false,
		},
		{
			name:                 "disconnected without auto restart",
			state:                webrtc.ICEConnectionStateDisconnected,
			autoOnDisconnect:     false,
			autoOnFailed:         true,
			expectTimer:          false,
			expectTriggerRestart: false,
		},
		{
			name:                 "connected cancels timer",
			state:                webrtc.ICEConnectionStateConnected,
			autoOnDisconnect:     true,
			autoOnFailed:         true,
			expectTimer:          false,
			expectTriggerRestart: false,
		},
		{
			name:                 "completed cancels timer",
			state:                webrtc.ICEConnectionStateCompleted,
			autoOnDisconnect:     true,
			autoOnFailed:         true,
			expectTimer:          false,
			expectTriggerRestart: false,
		},
		{
			name:                 "closed cancels timer",
			state:                webrtc.ICEConnectionStateClosed,
			autoOnDisconnect:     true,
			autoOnFailed:         true,
			expectTimer:          false,
			expectTriggerRestart: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewICERestartManager(ICERestartConfig{
				DisconnectedThreshold:   100 * time.Millisecond,
				AutoRestartOnDisconnect: tt.autoOnDisconnect,
				AutoRestartOnFailed:     tt.autoOnFailed,
			})
			defer m.Close()

			// First set disconnected to start timer if applicable
			if tt.state == webrtc.ICEConnectionStateConnected ||
				tt.state == webrtc.ICEConnectionStateCompleted ||
				tt.state == webrtc.ICEConnectionStateClosed {
				m.HandleICEConnectionStateChange(webrtc.ICEConnectionStateDisconnected)
				time.Sleep(10 * time.Millisecond) // Give time for timer to be set
			}

			m.HandleICEConnectionStateChange(tt.state)

			m.disconnectedMu.Lock()
			timerExists := m.disconnectedTimer != nil
			m.disconnectedMu.Unlock()

			if tt.expectTimer && !timerExists {
				t.Error("expected timer to exist")
			}
			if !tt.expectTimer && timerExists {
				t.Error("expected timer to not exist")
			}
		})
	}
}

func TestICERestartManager_HandleICEConnectionStateChange_Failed(t *testing.T) {
	m := NewICERestartManager(ICERestartConfig{
		Timeout:             100 * time.Millisecond,
		MaxAttempts:         1,
		AutoRestartOnFailed: true,
	})
	defer m.Close()

	// Track if restart was triggered
	restartTriggered := atomic.Bool{}
	m.OnRestartStarted(func(reason string) {
		restartTriggered.Store(true)
		if reason != ICERestartReasonConnectionFailed {
			t.Errorf("expected reason %s, got %s", ICERestartReasonConnectionFailed, reason)
		}
	})

	// Trigger failed state
	m.HandleICEConnectionStateChange(webrtc.ICEConnectionStateFailed)

	// Give goroutine time to start
	time.Sleep(50 * time.Millisecond)

	if !restartTriggered.Load() {
		t.Error("expected restart to be triggered on failed state")
	}
}

func TestICERestartManager_IsRestartInProgress(t *testing.T) {
	m := NewICERestartManager(ICERestartConfig{
		Timeout:     10 * time.Second,
		MaxAttempts: 1,
	})
	defer m.Close()

	if m.IsRestartInProgress() {
		t.Error("expected restart not in progress initially")
	}

	// Create two peer connections and complete negotiation
	pc1, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("failed to create peer connection 1: %v", err)
	}
	defer pc1.Close()

	pc2, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("failed to create peer connection 2: %v", err)
	}
	defer pc2.Close()

	_, err = pc1.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	if err != nil {
		t.Fatalf("failed to add transceiver: %v", err)
	}

	offer, err := pc1.CreateOffer(nil)
	if err != nil {
		t.Fatalf("failed to create offer: %v", err)
	}
	if err := pc1.SetLocalDescription(offer); err != nil {
		t.Fatalf("failed to set local description on pc1: %v", err)
	}
	if err := pc2.SetRemoteDescription(offer); err != nil {
		t.Fatalf("failed to set remote description on pc2: %v", err)
	}
	answer, err := pc2.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("failed to create answer: %v", err)
	}
	if err := pc2.SetLocalDescription(answer); err != nil {
		t.Fatalf("failed to set local description on pc2: %v", err)
	}
	if err := pc1.SetRemoteDescription(answer); err != nil {
		t.Fatalf("failed to set remote description on pc1: %v", err)
	}

	m.SetPeerConnection(pc1)

	blockCh := make(chan struct{})
	m.SetOfferHandler(func(ctx context.Context, offer webrtc.SessionDescription) error {
		<-blockCh
		return nil
	})

	go m.TriggerRestart(context.Background(), ICERestartReasonExplicit)

	time.Sleep(50 * time.Millisecond)

	if !m.IsRestartInProgress() {
		t.Error("expected restart to be in progress")
	}

	close(blockCh)
	time.Sleep(50 * time.Millisecond)

	if m.IsRestartInProgress() {
		t.Error("expected restart to no longer be in progress")
	}
}

func TestICERestartManager_GetTotalRestartAttempts(t *testing.T) {
	m := NewICERestartManager(ICERestartConfig{
		Timeout:     5 * time.Second,
		MaxAttempts: 1,
	})
	defer m.Close()

	if m.GetTotalRestartAttempts() != 0 {
		t.Error("expected 0 attempts initially")
	}

	// Create a real peer connection
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("failed to create peer connection: %v", err)
	}
	defer pc.Close()

	m.SetPeerConnection(pc)
	m.SetOfferHandler(func(ctx context.Context, offer webrtc.SessionDescription) error {
		return nil
	})

	m.TriggerRestart(context.Background(), ICERestartReasonExplicit)

	if m.GetTotalRestartAttempts() < 1 {
		t.Error("expected at least 1 attempt")
	}
}

func TestICERestartManager_GetLastRestartTime(t *testing.T) {
	m := NewICERestartManager(ICERestartConfig{
		Timeout:     5 * time.Second,
		MaxAttempts: 1,
	})
	defer m.Close()

	if !m.GetLastRestartTime().IsZero() {
		t.Error("expected zero time initially")
	}

	// Create a real peer connection
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("failed to create peer connection: %v", err)
	}
	defer pc.Close()

	m.SetPeerConnection(pc)
	m.SetOfferHandler(func(ctx context.Context, offer webrtc.SessionDescription) error {
		return nil
	})

	beforeRestart := time.Now()
	m.TriggerRestart(context.Background(), ICERestartReasonExplicit)

	lastRestart := m.GetLastRestartTime()
	if lastRestart.Before(beforeRestart) {
		t.Error("expected last restart time to be after test start")
	}
}

func TestICERestartManager_Reset(t *testing.T) {
	m := NewICERestartManager(ICERestartConfig{
		Timeout:     5 * time.Second,
		MaxAttempts: 1,
	})
	defer m.Close()

	// Create a real peer connection
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("failed to create peer connection: %v", err)
	}
	defer pc.Close()

	m.SetPeerConnection(pc)
	m.SetOfferHandler(func(ctx context.Context, offer webrtc.SessionDescription) error {
		return nil
	})

	m.TriggerRestart(context.Background(), ICERestartReasonExplicit)

	m.Reset()

	if m.GetTotalRestartAttempts() != 0 {
		t.Error("expected 0 attempts after reset")
	}
	if !m.GetLastRestartTime().IsZero() {
		t.Error("expected zero time after reset")
	}
}

func TestICERestartResult_ToEvent(t *testing.T) {
	result := ICERestartResult{
		Success:  true,
		Reason:   ICERestartReasonExplicit,
		Attempts: 2,
		Duration: 500 * time.Millisecond,
		Error:    nil,
	}

	event := result.ToEvent()

	if !event.Success {
		t.Error("expected success to be true")
	}
	if event.Reason != ICERestartReasonExplicit {
		t.Errorf("expected reason %s, got %s", ICERestartReasonExplicit, event.Reason)
	}
	if event.Attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", event.Attempts)
	}
	if event.Duration != 500*time.Millisecond {
		t.Errorf("expected duration %v, got %v", 500*time.Millisecond, event.Duration)
	}
	if event.ErrorMessage != "" {
		t.Error("expected empty error message")
	}
	if event.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestICERestartResult_ToEvent_WithError(t *testing.T) {
	result := ICERestartResult{
		Success:  false,
		Reason:   ICERestartReasonConnectionFailed,
		Attempts: 3,
		Duration: 1 * time.Second,
		Error:    errors.New("test error"),
	}

	event := result.ToEvent()

	if event.Success {
		t.Error("expected success to be false")
	}
	if event.ErrorMessage != "test error" {
		t.Errorf("expected error message 'test error', got '%s'", event.ErrorMessage)
	}
}

func TestNeedsICERestart(t *testing.T) {
	tests := []struct {
		state    webrtc.ICEConnectionState
		expected bool
	}{
		{webrtc.ICEConnectionStateFailed, true},
		{webrtc.ICEConnectionStateDisconnected, true},
		{webrtc.ICEConnectionStateNew, false},
		{webrtc.ICEConnectionStateChecking, false},
		{webrtc.ICEConnectionStateConnected, false},
		{webrtc.ICEConnectionStateCompleted, false},
		{webrtc.ICEConnectionStateClosed, false},
	}

	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			result := NeedsICERestart(tt.state)
			if result != tt.expected {
				t.Errorf("NeedsICERestart(%s) = %v, want %v", tt.state, result, tt.expected)
			}
		})
	}
}

func TestCanAttemptICERestart(t *testing.T) {
	tests := []struct {
		state    webrtc.ICEConnectionState
		expected bool
	}{
		{webrtc.ICEConnectionStateFailed, true},
		{webrtc.ICEConnectionStateDisconnected, true},
		{webrtc.ICEConnectionStateChecking, true},
		{webrtc.ICEConnectionStateConnected, true},
		{webrtc.ICEConnectionStateCompleted, true},
		{webrtc.ICEConnectionStateNew, false},
		{webrtc.ICEConnectionStateClosed, false},
	}

	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			result := CanAttemptICERestart(tt.state)
			if result != tt.expected {
				t.Errorf("CanAttemptICERestart(%s) = %v, want %v", tt.state, result, tt.expected)
			}
		})
	}
}

func TestICERestartWithBackoff_NoPeerConnection(t *testing.T) {
	_, err := ICERestartWithBackoff(context.Background(), nil, 3, 100*time.Millisecond)
	if !errors.Is(err, ErrNoPeerConnection) {
		t.Errorf("expected ErrNoPeerConnection, got %v", err)
	}
}

func TestICERestartWithBackoff_Success(t *testing.T) {
	// Create two peer connections and complete negotiation
	pc1, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("failed to create peer connection 1: %v", err)
	}
	defer pc1.Close()

	pc2, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("failed to create peer connection 2: %v", err)
	}
	defer pc2.Close()

	_, err = pc1.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	if err != nil {
		t.Fatalf("failed to add transceiver: %v", err)
	}

	initialOffer, err := pc1.CreateOffer(nil)
	if err != nil {
		t.Fatalf("failed to create offer: %v", err)
	}
	if err := pc1.SetLocalDescription(initialOffer); err != nil {
		t.Fatalf("failed to set local description on pc1: %v", err)
	}
	if err := pc2.SetRemoteDescription(initialOffer); err != nil {
		t.Fatalf("failed to set remote description on pc2: %v", err)
	}
	answer, err := pc2.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("failed to create answer: %v", err)
	}
	if err := pc2.SetLocalDescription(answer); err != nil {
		t.Fatalf("failed to set local description on pc2: %v", err)
	}
	if err := pc1.SetRemoteDescription(answer); err != nil {
		t.Fatalf("failed to set remote description on pc1: %v", err)
	}

	offer, err := ICERestartWithBackoff(context.Background(), pc1, 3, 100*time.Millisecond)
	if err != nil {
		t.Errorf("expected success, got error: %v", err)
	}
	if offer == nil {
		t.Error("expected offer to be returned")
	}
	if offer != nil && offer.Type != webrtc.SDPTypeOffer {
		t.Errorf("expected offer type, got %v", offer.Type)
	}
}

func TestICERestartWithBackoff_ContextCanceled(t *testing.T) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("failed to create peer connection: %v", err)
	}
	defer pc.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = ICERestartWithBackoff(ctx, pc, 3, 100*time.Millisecond)
	if err == nil {
		t.Error("expected error when context is canceled")
	}
}

func TestDetectNetworkChange(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := DetectNetworkChange(ctx)

	if ch == nil {
		t.Error("expected non-nil channel")
	}

	cancel()

	// Channel should be closed after context cancellation
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected channel to be closed after context cancellation")
	}
}

func TestICERestartReasons(t *testing.T) {
	// Verify reason constants are defined
	reasons := []string{
		ICERestartReasonNetworkChange,
		ICERestartReasonConnectionFailed,
		ICERestartReasonExplicit,
		ICERestartReasonDisconnected,
	}

	for _, reason := range reasons {
		if reason == "" {
			t.Error("expected non-empty reason constant")
		}
	}
}
