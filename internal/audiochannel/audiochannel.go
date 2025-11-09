package audiochannel

import (
	"context"
	"errors"
	"log"
	"sync"

	pkgwebrtc "github.com/HMasataka/choice/pkg/webrtc"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

var (
	ErrAudioChannelClosed    = errors.New("audio channel closed")
	ErrAudioChannelNotReady  = errors.New("audio channel not ready")
	ErrPeerConnectionNotSet  = errors.New("peer connection not set")
)

// AudioChannelConfig holds configuration for an audio channel
type AudioChannelConfig struct {
	Label      string
	Codec      webrtc.RTPCodecCapability
	SampleRate uint32
	Channels   uint8
}

// DefaultOpusConfig returns default configuration for Opus audio
func DefaultOpusConfig() AudioChannelConfig {
	return AudioChannelConfig{
		Label:      "audio",
		Codec:      pkgwebrtc.GetOpusCodec(),
		SampleRate: 48000,
		Channels:   2,
	}
}

// AudioChannel manages audio communication through WebRTC
type AudioChannel struct {
	ctx    context.Context
	cancel context.CancelFunc
	config AudioChannelConfig
	pc     *pkgwebrtc.PeerConnection

	// Audio track management
	audioTrack     *pkgwebrtc.AudioTrack
	localTrack     *webrtc.TrackLocalStaticSample
	remoteTrack    *webrtc.TrackRemote
	rtpSender      *webrtc.RTPSender

	// Event handlers
	onTrackReceived func(*webrtc.TrackRemote)
	onSample        func(*media.Sample)
	onOpen          func()
	onClose         func()
	onError         func(error)

	// State management
	mu     sync.RWMutex
	closed bool
}

// NewAudioChannel creates a new audio channel
func NewAudioChannel(ctx context.Context, config AudioChannelConfig, pc *pkgwebrtc.PeerConnection) (*AudioChannel, error) {
	if pc == nil {
		return nil, ErrPeerConnectionNotSet
	}

	audioCtx, cancel := context.WithCancel(ctx)

	ac := &AudioChannel{
		ctx:    audioCtx,
		cancel: cancel,
		config: config,
		pc:     pc,
	}

	// Initialize audio track
	if err := ac.initializeAudioTrack(); err != nil {
		cancel()
		return nil, err
	}

	// Setup event handlers
	ac.setupEventHandlers()

	return ac, nil
}

// initializeAudioTrack initializes the audio track for this channel
func (ac *AudioChannel) initializeAudioTrack() error {
	// Create audio track
	audioTrack, err := pkgwebrtc.NewAudioTrack(ac.ctx, ac.config.Label, ac.config.Codec)
	if err != nil {
		return err
	}

	ac.audioTrack = audioTrack

	// Create local track for sending audio
	localTrack, err := webrtc.NewTrackLocalStaticSample(
		ac.config.Codec,
		ac.config.Label,
		"audio-stream",
	)
	if err != nil {
		return err
	}

	ac.localTrack = localTrack

	// Note: In this implementation, we'll handle track addition through the peer connection's API
	// The rtpSender will be managed when actually adding the track to the peer connection
	ac.rtpSender = nil

	log.Printf("Audio channel initialized with label: %s, codec: %s", ac.config.Label, ac.config.Codec.MimeType)

	return nil
}

// setupEventHandlers sets up WebRTC event handlers
func (ac *AudioChannel) setupEventHandlers() {
	// Handle incoming tracks
	ac.pc.SetOnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		if track.Kind() != webrtc.RTPCodecTypeAudio {
			return
		}

		ac.mu.Lock()
		ac.remoteTrack = track
		ac.mu.Unlock()

		log.Printf("Received remote audio track: %s", track.ID())

		// Set remote track to audio track
		ac.audioTrack.SetRemoteTrack(track)

		// Trigger onTrackReceived callback
		ac.mu.RLock()
		handler := ac.onTrackReceived
		ac.mu.RUnlock()

		if handler != nil {
			handler(track)
		}

		// Start reading samples from remote track
		go func() {
			if err := ac.audioTrack.ReadSamples(ac.ctx); err != nil {
				log.Printf("Error reading audio samples: %v", err)
				ac.mu.RLock()
				errorHandler := ac.onError
				ac.mu.RUnlock()

				if errorHandler != nil {
					errorHandler(err)
				}
			}
		}()
	})

	// Setup sample handler
	ac.audioTrack.SetOnSample(func(sample *media.Sample) {
		ac.mu.RLock()
		handler := ac.onSample
		ac.mu.RUnlock()

		if handler != nil {
			handler(sample)
		}
	})
}

// SendSample sends an audio sample through the local track
func (ac *AudioChannel) SendSample(sample *media.Sample) error {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	if ac.closed {
		return ErrAudioChannelClosed
	}

	if ac.localTrack == nil {
		return ErrAudioChannelNotReady
	}

	return ac.localTrack.WriteSample(*sample)
}

// OnTrackReceived sets the handler for when a remote audio track is received
func (ac *AudioChannel) OnTrackReceived(handler func(*webrtc.TrackRemote)) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.onTrackReceived = handler
}

// OnSample sets the handler for when an audio sample is received
func (ac *AudioChannel) OnSample(handler func(*media.Sample)) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.onSample = handler
}

// OnOpen sets the handler for when the audio channel opens
func (ac *AudioChannel) OnOpen(handler func()) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.onOpen = handler
}

// OnClose sets the handler for when the audio channel closes
func (ac *AudioChannel) OnClose(handler func()) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.onClose = handler
}

// OnError sets the handler for audio channel errors
func (ac *AudioChannel) OnError(handler func(error)) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.onError = handler
}

// GetStats returns audio channel statistics
func (ac *AudioChannel) GetStats() AudioChannelStats {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	stats := AudioChannelStats{
		Label:      ac.config.Label,
		Codec:      ac.config.Codec.MimeType,
		SampleRate: ac.config.SampleRate,
		Channels:   ac.config.Channels,
		Closed:     ac.closed,
	}

	if ac.audioTrack != nil {
		audioStats := ac.audioTrack.Stats()
		stats.PacketsSent = audioStats.PacketsSent
		stats.PacketsReceived = audioStats.PacketsReceived
		stats.BytesSent = audioStats.BytesSent
		stats.BytesReceived = audioStats.BytesReceived
	}

	return stats
}

// LocalTrack returns the local audio track
func (ac *AudioChannel) LocalTrack() *webrtc.TrackLocalStaticSample {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.localTrack
}

// RemoteTrack returns the remote audio track
func (ac *AudioChannel) RemoteTrack() *webrtc.TrackRemote {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.remoteTrack
}

// Close closes the audio channel and releases resources
func (ac *AudioChannel) Close() error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if ac.closed {
		return nil
	}

	ac.closed = true

	// Cancel context
	if ac.cancel != nil {
		ac.cancel()
	}

	// Close audio track
	if ac.audioTrack != nil {
		ac.audioTrack.Close()
	}

	// Note: In this implementation, track removal would be handled by the peer connection
	// when the connection is closed

	// Trigger onClose callback
	if ac.onClose != nil {
		ac.onClose()
	}

	log.Printf("Audio channel closed: %s", ac.config.Label)

	return nil
}

// AudioChannelStats contains statistics for an audio channel
type AudioChannelStats struct {
	Label           string `json:"label"`
	Codec           string `json:"codec"`
	SampleRate      uint32 `json:"sample_rate"`
	Channels        uint8  `json:"channels"`
	PacketsSent     uint64 `json:"packets_sent"`
	PacketsReceived uint64 `json:"packets_received"`
	BytesSent       uint64 `json:"bytes_sent"`
	BytesReceived   uint64 `json:"bytes_received"`
	Closed          bool   `json:"closed"`
}