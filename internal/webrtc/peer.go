package webrtc

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
)

// PeerState represents the state of a peer connection.
type PeerState int32

const (
	// PeerStateNew indicates the peer connection is new.
	PeerStateNew PeerState = iota
	// PeerStateConnecting indicates the peer connection is connecting.
	PeerStateConnecting
	// PeerStateConnected indicates the peer connection is connected.
	PeerStateConnected
	// PeerStateDisconnected indicates the peer connection is disconnected.
	PeerStateDisconnected
	// PeerStateFailed indicates the peer connection has failed.
	PeerStateFailed
	// PeerStateClosed indicates the peer connection is closed.
	PeerStateClosed
)

// String returns the string representation of PeerState.
func (s PeerState) String() string {
	switch s {
	case PeerStateNew:
		return "new"
	case PeerStateConnecting:
		return "connecting"
	case PeerStateConnected:
		return "connected"
	case PeerStateDisconnected:
		return "disconnected"
	case PeerStateFailed:
		return "failed"
	case PeerStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// PeerConfig contains configuration for creating a Peer.
type PeerConfig struct {
	// ICEServers is the list of ICE servers to use.
	ICEServers []webrtc.ICEServer
	// ICELite enables ICE Lite mode.
	ICELite bool
	// UDPMux is the UDP mux to use for ICE.
	UDPMux ice.UDPMux
	// NAT1To1IPs are the IPs to use for NAT 1:1 mapping.
	NAT1To1IPs []string
	// ConnectionTimeout is the timeout for establishing a connection.
	// Note: Currently stored for reference; actual timeout enforcement
	// is implemented in higher-level connection management.
	ConnectionTimeout time.Duration
}

// DefaultPeerConfig returns a default PeerConfig.
func DefaultPeerConfig() PeerConfig {
	return PeerConfig{
		ICELite:           true,
		ConnectionTimeout: 30 * time.Second,
	}
}

// TrackHandler is called when a new track is received.
type TrackHandler func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver)

// ICECandidateHandler is called when a new ICE candidate is available.
type ICECandidateHandler func(candidate *webrtc.ICECandidate)

// ConnectionStateHandler is called when the connection state changes.
type ConnectionStateHandler func(state webrtc.PeerConnectionState)

// ICEConnectionStateHandler is called when the ICE connection state changes.
type ICEConnectionStateHandler func(state webrtc.ICEConnectionState)

// DataChannelHandler is called when a new data channel is opened.
type DataChannelHandler func(dc *webrtc.DataChannel)

// NegotiationNeededHandler is called when negotiation is needed.
type NegotiationNeededHandler func()

// ICECandidateErrorHandler is called when adding an ICE candidate fails.
type ICECandidateErrorHandler func(candidate *webrtc.ICECandidateInit, err error)

// Peer wraps a pion/webrtc PeerConnection with additional functionality.
type Peer struct {
	id string
	pc *webrtc.PeerConnection

	state atomic.Int32

	mu                      sync.RWMutex
	onTrack                 TrackHandler
	onICECandidate          ICECandidateHandler
	onConnectionStateChange ConnectionStateHandler
	onICEConnectionState    ICEConnectionStateHandler
	onDataChannel           DataChannelHandler
	onNegotiationNeeded     NegotiationNeededHandler
	onICECandidateError     ICECandidateErrorHandler

	createdAt time.Time
	closedAt  time.Time
	closeOnce sync.Once
	closeErr  error

	// pendingCandidates stores ICE candidates received before remote description is set.
	pendingCandidates []*webrtc.ICECandidateInit
	hasRemoteDesc     bool
}

var (
	// ErrPeerClosed is returned when the peer is closed.
	ErrPeerClosed = errors.New("peer is closed")
	// ErrNoPeerConnection is returned when there is no peer connection.
	ErrNoPeerConnection = errors.New("no peer connection")
	// ErrInvalidSDP is returned when the SDP is invalid.
	ErrInvalidSDP = errors.New("invalid SDP")
	// ErrNilMediaEngine is returned when the media engine is nil.
	ErrNilMediaEngine = errors.New("media engine is nil")
)

// NewPeer creates a new Peer with the given configuration.
func NewPeer(id string, config PeerConfig, mediaEngine *webrtc.MediaEngine) (*Peer, error) {
	if mediaEngine == nil {
		return nil, ErrNilMediaEngine
	}

	settingEngine := webrtc.SettingEngine{}

	// Enable ICE Lite mode
	if config.ICELite {
		settingEngine.SetLite(true)
	}

	// Set NAT 1:1 IPs
	if len(config.NAT1To1IPs) > 0 {
		settingEngine.SetNAT1To1IPs(config.NAT1To1IPs, webrtc.ICECandidateTypeHost)
	}

	// Set UDP mux if provided
	if config.UDPMux != nil {
		settingEngine.SetICEUDPMux(config.UDPMux)
	}

	// Create API with media engine
	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithSettingEngine(settingEngine),
	)

	// Create peer connection configuration
	pcConfig := webrtc.Configuration{
		ICEServers:         config.ICEServers,
		BundlePolicy:       webrtc.BundlePolicyMaxBundle,
		RTCPMuxPolicy:      webrtc.RTCPMuxPolicyRequire,
		SDPSemantics:       webrtc.SDPSemanticsUnifiedPlan,
		ICETransportPolicy: webrtc.ICETransportPolicyAll,
	}

	// Create peer connection
	pc, err := api.NewPeerConnection(pcConfig)
	if err != nil {
		return nil, err
	}

	peer := &Peer{
		id:                id,
		pc:                pc,
		createdAt:         time.Now(),
		pendingCandidates: make([]*webrtc.ICECandidateInit, 0),
	}
	peer.state.Store(int32(PeerStateNew))

	// Set up event handlers
	peer.setupEventHandlers()

	return peer, nil
}

// setupEventHandlers sets up the event handlers for the peer connection.
func (p *Peer) setupEventHandlers() {
	p.pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		p.mu.RLock()
		handler := p.onTrack
		p.mu.RUnlock()
		if handler != nil {
			handler(track, receiver)
		}
	})

	p.pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		p.mu.RLock()
		handler := p.onICECandidate
		p.mu.RUnlock()
		if handler != nil {
			handler(candidate)
		}
	})

	p.pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		p.updateState(state)
		p.mu.RLock()
		handler := p.onConnectionStateChange
		p.mu.RUnlock()
		if handler != nil {
			handler(state)
		}
	})

	p.pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		p.mu.RLock()
		handler := p.onICEConnectionState
		p.mu.RUnlock()
		if handler != nil {
			handler(state)
		}
	})

	p.pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		p.mu.RLock()
		handler := p.onDataChannel
		p.mu.RUnlock()
		if handler != nil {
			handler(dc)
		}
	})

	p.pc.OnNegotiationNeeded(func() {
		p.mu.RLock()
		handler := p.onNegotiationNeeded
		p.mu.RUnlock()
		if handler != nil {
			handler()
		}
	})
}

// updateState updates the peer state based on the connection state.
func (p *Peer) updateState(state webrtc.PeerConnectionState) {
	switch state {
	case webrtc.PeerConnectionStateNew:
		p.state.Store(int32(PeerStateNew))
	case webrtc.PeerConnectionStateConnecting:
		p.state.Store(int32(PeerStateConnecting))
	case webrtc.PeerConnectionStateConnected:
		p.state.Store(int32(PeerStateConnected))
	case webrtc.PeerConnectionStateDisconnected:
		p.state.Store(int32(PeerStateDisconnected))
	case webrtc.PeerConnectionStateFailed:
		p.state.Store(int32(PeerStateFailed))
	case webrtc.PeerConnectionStateClosed:
		p.state.Store(int32(PeerStateClosed))
	case webrtc.PeerConnectionStateUnknown:
		// Unknown state, keep current state
	}
}

// ID returns the peer ID.
func (p *Peer) ID() string {
	return p.id
}

// State returns the current peer state.
func (p *Peer) State() PeerState {
	return PeerState(p.state.Load())
}

// PeerConnection returns the underlying pion PeerConnection.
func (p *Peer) PeerConnection() *webrtc.PeerConnection {
	return p.pc
}

// CreatedAt returns the time when the peer was created.
func (p *Peer) CreatedAt() time.Time {
	return p.createdAt
}

// OnTrack sets the handler for new tracks.
func (p *Peer) OnTrack(handler TrackHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onTrack = handler
}

// OnICECandidate sets the handler for ICE candidates.
func (p *Peer) OnICECandidate(handler ICECandidateHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onICECandidate = handler
}

// OnConnectionStateChange sets the handler for connection state changes.
func (p *Peer) OnConnectionStateChange(handler ConnectionStateHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onConnectionStateChange = handler
}

// OnICEConnectionStateChange sets the handler for ICE connection state changes.
func (p *Peer) OnICEConnectionStateChange(handler ICEConnectionStateHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onICEConnectionState = handler
}

// OnDataChannel sets the handler for data channels.
func (p *Peer) OnDataChannel(handler DataChannelHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onDataChannel = handler
}

// OnNegotiationNeeded sets the handler for negotiation needed events.
func (p *Peer) OnNegotiationNeeded(handler NegotiationNeededHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onNegotiationNeeded = handler
}

// OnICECandidateError sets the handler for ICE candidate errors.
// This is called when adding a pending ICE candidate fails during SetRemoteDescription.
func (p *Peer) OnICECandidateError(handler ICECandidateErrorHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onICECandidateError = handler
}

// SetRemoteDescription sets the remote description.
func (p *Peer) SetRemoteDescription(sdp webrtc.SessionDescription) error {
	if p.pc == nil {
		return ErrNoPeerConnection
	}

	if p.State() == PeerStateClosed {
		return ErrPeerClosed
	}

	if err := p.pc.SetRemoteDescription(sdp); err != nil {
		return err
	}

	// Process pending ICE candidates
	p.mu.Lock()
	p.hasRemoteDesc = true
	candidates := p.pendingCandidates
	p.pendingCandidates = make([]*webrtc.ICECandidateInit, 0)
	errorHandler := p.onICECandidateError
	p.mu.Unlock()

	// Process pending ICE candidates, notifying handler on errors
	// ICE candidate errors are non-fatal - the connection may still succeed
	for _, candidate := range candidates {
		if err := p.pc.AddICECandidate(*candidate); err != nil {
			if errorHandler != nil {
				errorHandler(candidate, err)
			}
		}
	}

	return nil
}

// CreateAnswer creates an SDP answer.
func (p *Peer) CreateAnswer() (webrtc.SessionDescription, error) {
	if p.pc == nil {
		return webrtc.SessionDescription{}, ErrNoPeerConnection
	}

	if p.State() == PeerStateClosed {
		return webrtc.SessionDescription{}, ErrPeerClosed
	}

	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}

	if err := p.pc.SetLocalDescription(answer); err != nil {
		return webrtc.SessionDescription{}, err
	}

	return answer, nil
}

// CreateOffer creates an SDP offer.
func (p *Peer) CreateOffer() (webrtc.SessionDescription, error) {
	if p.pc == nil {
		return webrtc.SessionDescription{}, ErrNoPeerConnection
	}

	if p.State() == PeerStateClosed {
		return webrtc.SessionDescription{}, ErrPeerClosed
	}

	offer, err := p.pc.CreateOffer(nil)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}

	if err := p.pc.SetLocalDescription(offer); err != nil {
		return webrtc.SessionDescription{}, err
	}

	return offer, nil
}

// AddICECandidate adds an ICE candidate.
func (p *Peer) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	if p.pc == nil {
		return ErrNoPeerConnection
	}

	if p.State() == PeerStateClosed {
		return ErrPeerClosed
	}

	p.mu.Lock()
	hasRemoteDesc := p.hasRemoteDesc
	if !hasRemoteDesc {
		// Queue the candidate until remote description is set
		p.pendingCandidates = append(p.pendingCandidates, &candidate)
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	return p.pc.AddICECandidate(candidate)
}

// AddTrack adds a track to the peer connection.
func (p *Peer) AddTrack(track webrtc.TrackLocal) (*webrtc.RTPSender, error) {
	if p.pc == nil {
		return nil, ErrNoPeerConnection
	}

	if p.State() == PeerStateClosed {
		return nil, ErrPeerClosed
	}

	return p.pc.AddTrack(track)
}

// RemoveTrack removes a track from the peer connection.
func (p *Peer) RemoveTrack(sender *webrtc.RTPSender) error {
	if p.pc == nil {
		return ErrNoPeerConnection
	}

	if p.State() == PeerStateClosed {
		return ErrPeerClosed
	}

	return p.pc.RemoveTrack(sender)
}

// GetSenders returns all RTP senders.
func (p *Peer) GetSenders() []*webrtc.RTPSender {
	if p.pc == nil {
		return nil
	}
	return p.pc.GetSenders()
}

// GetReceivers returns all RTP receivers.
func (p *Peer) GetReceivers() []*webrtc.RTPReceiver {
	if p.pc == nil {
		return nil
	}
	return p.pc.GetReceivers()
}

// GetTransceivers returns all transceivers.
func (p *Peer) GetTransceivers() []*webrtc.RTPTransceiver {
	if p.pc == nil {
		return nil
	}
	return p.pc.GetTransceivers()
}

// AddTransceiverFromKind adds a transceiver for the given kind.
func (p *Peer) AddTransceiverFromKind(kind webrtc.RTPCodecType, init ...webrtc.RTPTransceiverInit) (*webrtc.RTPTransceiver, error) {
	if p.pc == nil {
		return nil, ErrNoPeerConnection
	}

	if p.State() == PeerStateClosed {
		return nil, ErrPeerClosed
	}

	return p.pc.AddTransceiverFromKind(kind, init...)
}

// LocalDescription returns the local description.
func (p *Peer) LocalDescription() *webrtc.SessionDescription {
	if p.pc == nil {
		return nil
	}
	return p.pc.LocalDescription()
}

// RemoteDescription returns the remote description.
func (p *Peer) RemoteDescription() *webrtc.SessionDescription {
	if p.pc == nil {
		return nil
	}
	return p.pc.RemoteDescription()
}

// SignalingState returns the signaling state.
func (p *Peer) SignalingState() webrtc.SignalingState {
	if p.pc == nil {
		return webrtc.SignalingStateClosed
	}
	return p.pc.SignalingState()
}

// ICEConnectionState returns the ICE connection state.
func (p *Peer) ICEConnectionState() webrtc.ICEConnectionState {
	if p.pc == nil {
		return webrtc.ICEConnectionStateClosed
	}
	return p.pc.ICEConnectionState()
}

// ConnectionState returns the connection state.
func (p *Peer) ConnectionState() webrtc.PeerConnectionState {
	if p.pc == nil {
		return webrtc.PeerConnectionStateClosed
	}
	return p.pc.ConnectionState()
}

// RestartICE triggers an ICE restart.
func (p *Peer) RestartICE() error {
	if p.pc == nil {
		return ErrNoPeerConnection
	}

	if p.State() == PeerStateClosed {
		return ErrPeerClosed
	}

	offer, err := p.pc.CreateOffer(&webrtc.OfferOptions{
		ICERestart: true,
	})
	if err != nil {
		return err
	}

	return p.pc.SetLocalDescription(offer)
}

// Close closes the peer connection.
// This method is idempotent and safe to call multiple times concurrently.
func (p *Peer) Close() error {
	if p.pc == nil {
		return nil
	}

	p.closeOnce.Do(func() {
		// Close the underlying peer connection
		p.closeErr = p.pc.Close()

		// Update state after close attempt
		p.mu.Lock()
		p.closedAt = time.Now()
		p.mu.Unlock()
		p.state.Store(int32(PeerStateClosed))
	})

	return p.closeErr
}

// CloseWithContext closes the peer connection with a context.
func (p *Peer) CloseWithContext(ctx context.Context) error {
	done := make(chan error, 1)
	go func() {
		done <- p.Close()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Duration returns how long the peer has been alive.
func (p *Peer) Duration() time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.closedAt.IsZero() {
		return p.closedAt.Sub(p.createdAt)
	}
	return time.Since(p.createdAt)
}

// HandleOffer processes an SDP offer and returns an SDP answer.
// This is a high-level method that combines SetRemoteDescription and CreateAnswer.
func (p *Peer) HandleOffer(ctx context.Context, sdp string) (string, error) {
	if p.State() == PeerStateClosed {
		return "", ErrPeerClosed
	}

	// Set the remote offer
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdp,
	}

	if err := p.SetRemoteDescription(offer); err != nil {
		return "", err
	}

	// Create answer
	answer, err := p.CreateAnswer()
	if err != nil {
		return "", err
	}

	return answer.SDP, nil
}

// HandleAnswer processes an SDP answer.
// This is a high-level method that wraps SetRemoteDescription for answers.
func (p *Peer) HandleAnswer(ctx context.Context, sdp string) error {
	if p.State() == PeerStateClosed {
		return ErrPeerClosed
	}

	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  sdp,
	}

	return p.SetRemoteDescription(answer)
}

// HandleCandidate processes an ICE candidate.
// This is a high-level method that constructs an ICECandidateInit and calls AddICECandidate.
func (p *Peer) HandleCandidate(ctx context.Context, candidate string, sdpMid string, sdpMLineIndex *int) error {
	if p.State() == PeerStateClosed {
		return ErrPeerClosed
	}

	// Convert *int to *uint16 for ICECandidateInit
	var mLineIndex *uint16
	if sdpMLineIndex != nil {
		idx := uint16(*sdpMLineIndex)
		mLineIndex = &idx
	}

	candidateInit := webrtc.ICECandidateInit{
		Candidate:     candidate,
		SDPMid:        &sdpMid,
		SDPMLineIndex: mLineIndex,
	}

	return p.AddICECandidate(candidateInit)
}
