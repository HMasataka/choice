package webrtc

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/webrtc/v4"
)

var (
	// ErrICERestartInProgress is returned when an ICE restart is already in progress.
	ErrICERestartInProgress = errors.New("ICE restart already in progress")
	// ErrICERestartTimeout is returned when ICE restart times out.
	ErrICERestartTimeout = errors.New("ICE restart timeout")
	// ErrICERestartFailed is returned when ICE restart fails.
	ErrICERestartFailed = errors.New("ICE restart failed")
	// ErrNoOfferHandler is returned when no offer handler is set.
	ErrNoOfferHandler = errors.New("no offer handler set for ICE restart")
)

// ICE restart trigger reasons.
const (
	// ICERestartReasonNetworkChange indicates network change detected.
	ICERestartReasonNetworkChange = "network_change"
	// ICERestartReasonConnectionFailed indicates ICE connection failed.
	ICERestartReasonConnectionFailed = "connection_failed"
	// ICERestartReasonExplicit indicates explicit restart request.
	ICERestartReasonExplicit = "explicit"
	// ICERestartReasonDisconnected indicates ICE disconnected state.
	ICERestartReasonDisconnected = "disconnected"
)

// Default ICE restart configuration values.
const (
	// DefaultICERestartTimeout is the default timeout for ICE restart.
	DefaultICERestartTimeout = 30 * time.Second
	// DefaultDisconnectedThreshold is the time to wait before triggering restart on disconnect.
	DefaultDisconnectedThreshold = 5 * time.Second
	// DefaultMaxRestartAttempts is the maximum number of restart attempts.
	DefaultMaxRestartAttempts = 3
	// DefaultRestartBackoff is the backoff duration between restart attempts.
	DefaultRestartBackoff = 2 * time.Second
)

// ICERestartConfig contains configuration for ICE restart behavior.
type ICERestartConfig struct {
	// Timeout for ICE restart completion.
	// Default: 30 seconds
	Timeout time.Duration

	// DisconnectedThreshold is the time to wait before triggering restart on disconnect.
	// Default: 5 seconds
	DisconnectedThreshold time.Duration

	// MaxAttempts is the maximum number of restart attempts before giving up.
	// Default: 3
	MaxAttempts int

	// Backoff is the duration to wait between restart attempts.
	// Default: 2 seconds
	Backoff time.Duration

	// AutoRestartOnDisconnect enables automatic ICE restart when connection disconnects.
	// Default: true
	AutoRestartOnDisconnect bool

	// AutoRestartOnFailed enables automatic ICE restart when connection fails.
	// Default: true
	AutoRestartOnFailed bool
}

// DefaultICERestartConfig returns the default ICE restart configuration.
func DefaultICERestartConfig() ICERestartConfig {
	return ICERestartConfig{
		Timeout:                 DefaultICERestartTimeout,
		DisconnectedThreshold:   DefaultDisconnectedThreshold,
		MaxAttempts:             DefaultMaxRestartAttempts,
		Backoff:                 DefaultRestartBackoff,
		AutoRestartOnDisconnect: true,
		AutoRestartOnFailed:     true,
	}
}

// ICERestartResult contains the result of an ICE restart operation.
// Note: Success indicates the restart offer was created and the handler completed
// successfully. It does NOT guarantee ICE connectivity has been restored.
// The caller should monitor ICE connection state changes to confirm recovery.
type ICERestartResult struct {
	// Success indicates if the restart offer was created and handler completed successfully.
	// Note: This does not guarantee ICE connectivity recovery - monitor ICE state for that.
	Success bool
	// Reason is the trigger reason for the restart.
	Reason string
	// Attempts is the number of attempts made.
	Attempts int
	// Duration is the total duration of the restart operation.
	Duration time.Duration
	// Error is the error if restart failed.
	Error error
	// Offer is the new SDP offer generated for the restart.
	Offer *webrtc.SessionDescription
}

// ICERestartHandler handles the SDP offer generated during ICE restart.
// The handler should send the offer to the remote peer and set the answer.
type ICERestartHandler func(ctx context.Context, offer webrtc.SessionDescription) error

// ICERestartManager manages ICE restart operations for a peer connection.
type ICERestartManager struct {
	config ICERestartConfig

	// Peer connection reference (set via SetPeerConnection)
	pc *webrtc.PeerConnection

	// Handler for sending restart offer to remote peer
	offerHandler ICERestartHandler

	// State tracking
	restartInProgress atomic.Bool
	restartAttempts   atomic.Int32
	lastRestartTime   atomic.Value // time.Time

	// Callbacks
	onRestartStarted   func(reason string)
	onRestartCompleted func(result ICERestartResult)

	// Disconnected state tracking
	disconnectedTimer *time.Timer
	disconnectedMu    sync.Mutex

	mu sync.RWMutex
}

// NewICERestartManager creates a new ICE restart manager.
func NewICERestartManager(config ICERestartConfig) *ICERestartManager {
	// Normalize zero/negative values to defaults
	if config.Timeout <= 0 {
		config.Timeout = DefaultICERestartTimeout
	}
	if config.DisconnectedThreshold <= 0 {
		config.DisconnectedThreshold = DefaultDisconnectedThreshold
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = DefaultMaxRestartAttempts
	}
	if config.Backoff <= 0 {
		config.Backoff = DefaultRestartBackoff
	}

	m := &ICERestartManager{
		config: config,
	}
	m.lastRestartTime.Store(time.Time{})

	return m
}

// SetPeerConnection sets the peer connection to manage.
func (m *ICERestartManager) SetPeerConnection(pc *webrtc.PeerConnection) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pc = pc
}

// SetOfferHandler sets the handler for sending restart offers.
func (m *ICERestartManager) SetOfferHandler(handler ICERestartHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.offerHandler = handler
}

// OnRestartStarted sets a callback for when restart begins.
func (m *ICERestartManager) OnRestartStarted(fn func(reason string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onRestartStarted = fn
}

// OnRestartCompleted sets a callback for when restart completes.
func (m *ICERestartManager) OnRestartCompleted(fn func(result ICERestartResult)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onRestartCompleted = fn
}

// HandleICEConnectionStateChange should be called when ICE connection state changes.
// This enables automatic restart based on configuration.
func (m *ICERestartManager) HandleICEConnectionStateChange(state webrtc.ICEConnectionState) {
	switch state {
	case webrtc.ICEConnectionStateDisconnected:
		if m.config.AutoRestartOnDisconnect {
			m.scheduleRestartOnDisconnect()
		}
	case webrtc.ICEConnectionStateFailed:
		if m.config.AutoRestartOnFailed {
			go m.TriggerRestart(context.Background(), ICERestartReasonConnectionFailed)
		}
	case webrtc.ICEConnectionStateConnected, webrtc.ICEConnectionStateCompleted:
		m.cancelDisconnectedTimer()
	case webrtc.ICEConnectionStateClosed:
		m.cancelDisconnectedTimer()
	}
}

// scheduleRestartOnDisconnect schedules an ICE restart after disconnected threshold.
func (m *ICERestartManager) scheduleRestartOnDisconnect() {
	m.disconnectedMu.Lock()
	defer m.disconnectedMu.Unlock()

	// Cancel existing timer
	if m.disconnectedTimer != nil {
		m.disconnectedTimer.Stop()
	}

	m.disconnectedTimer = time.AfterFunc(m.config.DisconnectedThreshold, func() {
		// Check if we should still restart (connection might have recovered)
		m.mu.RLock()
		pc := m.pc
		m.mu.RUnlock()

		if pc == nil {
			return
		}

		// Only trigger restart if still in a state that needs restart
		iceState := pc.ICEConnectionState()
		if NeedsICERestart(iceState) {
			m.TriggerRestart(context.Background(), ICERestartReasonDisconnected)
		}
	})
}

// cancelDisconnectedTimer cancels the pending disconnected restart timer.
func (m *ICERestartManager) cancelDisconnectedTimer() {
	m.disconnectedMu.Lock()
	defer m.disconnectedMu.Unlock()

	if m.disconnectedTimer != nil {
		m.disconnectedTimer.Stop()
		m.disconnectedTimer = nil
	}
}

// TriggerRestart initiates an ICE restart with the given reason.
func (m *ICERestartManager) TriggerRestart(ctx context.Context, reason string) ICERestartResult {
	startTime := time.Now()
	result := ICERestartResult{
		Reason: reason,
	}

	// Check if restart is already in progress
	if !m.restartInProgress.CompareAndSwap(false, true) {
		result.Error = ErrICERestartInProgress
		return result
	}
	defer m.restartInProgress.Store(false)

	// Notify restart started
	m.mu.RLock()
	startedCallback := m.onRestartStarted
	m.mu.RUnlock()
	if startedCallback != nil {
		startedCallback(reason)
	}

	// Apply timeout
	if m.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.config.Timeout)
		defer cancel()
	}

	// Perform restart with retry
	for attempt := 1; attempt <= m.config.MaxAttempts; attempt++ {
		result.Attempts = attempt

		err := m.performRestart(ctx, &result)
		if err == nil {
			result.Success = true
			break
		}

		result.Error = err

		// Check if context is done
		if ctx.Err() != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				result.Error = ErrICERestartTimeout
			}
			break
		}

		// Wait before retry if not last attempt
		if attempt < m.config.MaxAttempts {
			select {
			case <-time.After(m.config.Backoff):
			case <-ctx.Done():
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					result.Error = ErrICERestartTimeout
				}
				break
			}
		}
	}

	result.Duration = time.Since(startTime)
	m.lastRestartTime.Store(time.Now())
	m.restartAttempts.Add(int32(result.Attempts))

	// Notify restart completed
	m.mu.RLock()
	completedCallback := m.onRestartCompleted
	m.mu.RUnlock()
	if completedCallback != nil {
		completedCallback(result)
	}

	return result
}

// performRestart performs a single ICE restart attempt.
func (m *ICERestartManager) performRestart(ctx context.Context, result *ICERestartResult) error {
	m.mu.RLock()
	pc := m.pc
	handler := m.offerHandler
	m.mu.RUnlock()

	if pc == nil {
		return ErrNoPeerConnection
	}

	if handler == nil {
		return ErrNoOfferHandler
	}

	// Create offer with ICE restart flag
	offer, err := pc.CreateOffer(&webrtc.OfferOptions{
		ICERestart: true,
	})
	if err != nil {
		return err
	}

	result.Offer = &offer

	// Set local description
	if err := pc.SetLocalDescription(offer); err != nil {
		return err
	}

	// Call handler to send offer and receive answer
	if err := handler(ctx, offer); err != nil {
		return err
	}

	return nil
}

// IsRestartInProgress returns true if an ICE restart is currently in progress.
func (m *ICERestartManager) IsRestartInProgress() bool {
	return m.restartInProgress.Load()
}

// GetTotalRestartAttempts returns the total number of restart attempts made.
func (m *ICERestartManager) GetTotalRestartAttempts() int32 {
	return m.restartAttempts.Load()
}

// GetLastRestartTime returns the time of the last restart attempt.
func (m *ICERestartManager) GetLastRestartTime() time.Time {
	return m.lastRestartTime.Load().(time.Time)
}

// Reset resets the restart manager state.
func (m *ICERestartManager) Reset() {
	m.cancelDisconnectedTimer()
	m.restartAttempts.Store(0)
	m.lastRestartTime.Store(time.Time{})
}

// Close cleans up the restart manager.
func (m *ICERestartManager) Close() {
	m.cancelDisconnectedTimer()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.pc = nil
	m.offerHandler = nil
	m.onRestartStarted = nil
	m.onRestartCompleted = nil
}

// ICERestartEvent represents an ICE restart event for logging/metrics.
type ICERestartEvent struct {
	// Timestamp of the event.
	Timestamp time.Time
	// Reason for the restart.
	Reason string
	// Success indicates if restart was successful.
	Success bool
	// Attempts is the number of attempts made.
	Attempts int
	// Duration of the restart operation.
	Duration time.Duration
	// Error message if failed.
	ErrorMessage string
}

// ToEvent converts ICERestartResult to ICERestartEvent.
func (r ICERestartResult) ToEvent() ICERestartEvent {
	event := ICERestartEvent{
		Timestamp: time.Now(),
		Reason:    r.Reason,
		Success:   r.Success,
		Attempts:  r.Attempts,
		Duration:  r.Duration,
	}
	if r.Error != nil {
		event.ErrorMessage = r.Error.Error()
	}
	return event
}

// NeedsICERestart checks if the given ICE connection state indicates
// that an ICE restart might be needed.
func NeedsICERestart(state webrtc.ICEConnectionState) bool {
	return state == webrtc.ICEConnectionStateFailed ||
		state == webrtc.ICEConnectionStateDisconnected
}

// CanAttemptICERestart checks if an ICE restart can be attempted
// based on the current connection state.
func CanAttemptICERestart(state webrtc.ICEConnectionState) bool {
	return state != webrtc.ICEConnectionStateClosed &&
		state != webrtc.ICEConnectionStateNew
}

// ICERestartWithBackoff performs an ICE restart with exponential backoff.
// This is a convenience function for simple use cases.
func ICERestartWithBackoff(
	ctx context.Context,
	pc *webrtc.PeerConnection,
	maxAttempts int,
	initialBackoff time.Duration,
) (*webrtc.SessionDescription, error) {
	if pc == nil {
		return nil, ErrNoPeerConnection
	}

	var lastErr error
	backoff := initialBackoff

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		offer, err := pc.CreateOffer(&webrtc.OfferOptions{
			ICERestart: true,
		})
		if err != nil {
			lastErr = err
		} else {
			if err := pc.SetLocalDescription(offer); err != nil {
				lastErr = err
			} else {
				return &offer, nil
			}
		}

		// Check context
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Wait before retry
		if attempt < maxAttempts {
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			backoff *= 2 // Exponential backoff
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrICERestartFailed
}

// DetectNetworkChange is a placeholder for network change detection.
// In a real implementation, this would monitor network interfaces.
// Returns a channel that signals when a network change is detected.
func DetectNetworkChange(ctx context.Context) <-chan struct{} {
	ch := make(chan struct{})

	// Note: Actual network change detection would require platform-specific
	// implementation (e.g., using netlink on Linux, SystemConfiguration on macOS).
	// This is a placeholder that never fires, as network change detection
	// is typically handled by the client browser in WebRTC scenarios.
	// For server-side network changes, external monitoring should be used.

	go func() {
		<-ctx.Done()
		close(ch)
	}()

	return ch
}
