package webrtc

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

// PeerConnectionOptions represents options for peer connection
type PeerConnectionOptions struct {
	ICEServers          []webrtc.ICEServer
	ICECandidateTimeout time.Duration
}

// DefaultPeerConnectionOptions returns default options
func DefaultPeerConnectionOptions() PeerConnectionOptions {
	return PeerConnectionOptions{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
		ICECandidateTimeout: 30 * time.Second,
	}
}

// PeerConnection wraps a WebRTC peer connection
type PeerConnection struct {
	ctx context.Context

	pc      *webrtc.PeerConnection
	options PeerConnectionOptions

	pendingCandidates []webrtc.ICECandidateInit
	candidatesMu      sync.Mutex

	audioTracks   map[string]*AudioTrack
	audioTracksMu sync.RWMutex

	onICECandidate       func(*webrtc.ICECandidate) error
	onDataChannel        func(*webrtc.DataChannel)
	onConnectionState    func(webrtc.PeerConnectionState)
	onICEConnectionState func(webrtc.ICEConnectionState)
	onICEGatheringState  func(webrtc.ICEGatheringState)
	onTrack              func(*webrtc.TrackRemote, *webrtc.RTPReceiver)

	cancel context.CancelFunc
}

// NewPeerConnection creates a new peer connection
func NewPeerConnection(ctx context.Context, id string, options PeerConnectionOptions, mediaEngine *webrtc.MediaEngine) (*PeerConnection, error) {
	config := webrtc.Configuration{
		ICEServers: options.ICEServers,
	}

	settingEngine := webrtc.SettingEngine{}
	// Use QueryOnly to resolve mDNS candidates from client while still gathering regular host candidates
	settingEngine.SetICEMulticastDNSMode(ice.MulticastDNSModeQueryOnly)

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithSettingEngine(settingEngine),
	)

	pc, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, errors.New("failed to create peer connection: " + err.Error())
	}

	ctx, cancel := context.WithCancel(ctx)

	p := &PeerConnection{
		ctx:               ctx,
		pc:                pc,
		options:           options,
		cancel:            cancel,
		pendingCandidates: make([]webrtc.ICECandidateInit, 0),
		audioTracks:       make(map[string]*AudioTrack),
	}
	p.setupEventHandlers()

	return p, nil
}

// Close closes the peer connection
func (p *PeerConnection) Close() error {
	return p.pc.Close()
}

// CreateOffer creates an SDP offer
func (p *PeerConnection) CreateOffer(options *webrtc.OfferOptions) (webrtc.SessionDescription, error) {
	offer, err := p.pc.CreateOffer(options)
	if err != nil {
		return webrtc.SessionDescription{}, errors.New("failed to create offer: " + err.Error())
	}

	if err := p.pc.SetLocalDescription(offer); err != nil {
		return webrtc.SessionDescription{}, errors.New("failed to set local description: " + err.Error())
	}

	<-webrtc.GatheringCompletePromise(p.pc)

	return offer, nil
}

// CreateAnswer creates an SDP answer
func (p *PeerConnection) CreateAnswer(options *webrtc.AnswerOptions) (webrtc.SessionDescription, error) {
	answer, err := p.pc.CreateAnswer(options)
	if err != nil {
		return webrtc.SessionDescription{}, errors.New("failed to create answer: " + err.Error())
	}

	if err := p.pc.SetLocalDescription(answer); err != nil {
		return webrtc.SessionDescription{}, errors.New("failed to set local description: " + err.Error())
	}

	return answer, nil
}

// SetRemoteDescription sets the remote SDP
func (p *PeerConnection) SetRemoteDescription(sdp webrtc.SessionDescription) error {
	if err := p.pc.SetRemoteDescription(sdp); err != nil {
		return errors.New("failed to set remote description: " + err.Error())
	}

	// Process pending ICE candidates if any
	p.processPendingCandidates()

	return nil
}

// AddICECandidate adds an ICE candidate
func (p *PeerConnection) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	// If remote description is not set yet, queue the candidate
	if p.pc.RemoteDescription() == nil {
		p.candidatesMu.Lock()
		p.pendingCandidates = append(p.pendingCandidates, candidate)
		p.candidatesMu.Unlock()
		return nil
	}

	if err := p.pc.AddICECandidate(candidate); err != nil {
		return errors.New("failed to add ICE candidate: " + err.Error())
	}

	return nil
}

// processPendingCandidates processes queued ICE candidates
func (p *PeerConnection) processPendingCandidates() {
	p.candidatesMu.Lock()
	candidates := p.pendingCandidates
	p.pendingCandidates = nil
	p.candidatesMu.Unlock()

	for _, candidate := range candidates {
		if err := p.pc.AddICECandidate(candidate); err != nil {
		}
	}
}

// SetOnICECandidate sets the ICE candidate handler
func (p *PeerConnection) SetOnICECandidate(handler func(*webrtc.ICECandidate) error) {
	p.onICECandidate = handler
}

// SetOnDataChannel sets the data channel handler
func (p *PeerConnection) SetOnDataChannel(handler func(*webrtc.DataChannel)) {
	p.onDataChannel = handler
}

// SetOnConnectionStateChange sets the connection state change handler
func (p *PeerConnection) SetOnConnectionStateChange(handler func(webrtc.PeerConnectionState)) {
	p.onConnectionState = handler
}

// SetOnTrack sets the track handler for incoming media
func (p *PeerConnection) SetOnTrack(handler func(*webrtc.TrackRemote, *webrtc.RTPReceiver)) {
	p.onTrack = handler
}

// SetOnICEConnectionStateChange sets the ICE connection state change handler
func (p *PeerConnection) SetOnICEConnectionStateChange(handler func(webrtc.ICEConnectionState)) {
	p.onICEConnectionState = handler
}

// SetOnICEGatheringStateChange sets the ICE gathering state change handler
func (p *PeerConnection) SetOnICEGatheringStateChange(handler func(webrtc.ICEGatheringState)) {
	p.onICEGatheringState = handler
}

// CreateDataChannel creates a new data channel
func (p *PeerConnection) CreateDataChannel(label string, options *webrtc.DataChannelInit) (*webrtc.DataChannel, error) {
	return p.pc.CreateDataChannel(label, options)
}

// AddTrack adds a track to the peer connection
func (p *PeerConnection) AddTrack(track webrtc.TrackLocal) (*webrtc.RTPSender, error) {
	return p.pc.AddTrack(track)
}

// AddTransceiverFromKind adds a transceiver for the specified media kind
func (p *PeerConnection) AddTransceiverFromKind(kind webrtc.RTPCodecType, direction webrtc.RTPTransceiverDirection) (*webrtc.RTPTransceiver, error) {
	return p.pc.AddTransceiverFromKind(kind, webrtc.RTPTransceiverInit{
		Direction: direction,
	})
}

// RemoveTrack removes a track from the peer connection
func (p *PeerConnection) RemoveTrack(sender *webrtc.RTPSender) error {
	return p.pc.RemoveTrack(sender)
}

// setupEventHandlers sets up WebRTC event handlers
func (p *PeerConnection) setupEventHandlers() {
	// ICE candidate handler
	p.pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}

		if p.onICECandidate != nil {
			if err := p.onICECandidate(candidate); err != nil {
				// Log error but continue
			}
		}
	})

	// Data channel handler
	p.pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if p.onDataChannel != nil {
			p.onDataChannel(dc)
		}
	})

	// Connection state handler
	p.pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if p.onConnectionState != nil {
			p.onConnectionState(state)
		}
	})

	// ICE connection state handler
	p.pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if p.onICEConnectionState != nil {
			p.onICEConnectionState(state)
		}
	})

	// ICE gathering state handler
	p.pc.OnICEGatheringStateChange(func(state webrtc.ICEGatheringState) {
		if p.onICEGatheringState != nil {
			p.onICEGatheringState(state)
		}
	})

	// Track handler for incoming audio/video
	p.pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("Received track: ID=%s, Kind=%s, Codec=%s",
			track.ID(), track.Kind(), track.Codec().MimeType)

		if track.Kind() == webrtc.RTPCodecTypeAudio {
			log.Printf("Processing audio track: %s", track.ID())

			audioTrack, err := NewAudioTrack(p.ctx, track.ID(), track.Codec().RTPCodecCapability)
			if err != nil {
				log.Printf("Failed to create audio track: %v", err)
				return
			}

			audioTrack.SetRemoteTrack(track)

			// Set up sample handler with detailed logging
			audioTrack.SetOnSample(func(sample *media.Sample) {
				log.Printf("Received audio sample from client: %d bytes, duration: %v",
					len(sample.Data), sample.Duration)
			})

			p.audioTracksMu.Lock()
			p.audioTracks[track.ID()] = audioTrack
			p.audioTracksMu.Unlock()

			log.Printf("Starting audio sample reading for track: %s", track.ID())
			go func() {
				if err := audioTrack.ReadSamples(p.ctx); err != nil {
					log.Printf("Error reading samples from track %s: %v", track.ID(), err)
					return
				}
			}()
		}

		if p.onTrack != nil {
			p.onTrack(track, receiver)
		}
	})
}
