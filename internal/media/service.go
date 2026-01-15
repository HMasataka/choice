package media

import (
	"context"
	"fmt"
	"io"
	"sync"

	pion "github.com/pion/webrtc/v4"

	"github.com/HMasataka/choice/internal/signaling/protocol"
)

// WebRTCService defines the interface for WebRTC operations.
// This is a subset of the actual webrtc.Service that we need for media forwarding.
type WebRTCService interface {
	// GetPeer returns the peer for the given participant ID.
	// Returns nil if no peer exists.
	GetPeer(participantID string) WebRTCPeer
}

// WebRTCPeer represents a WebRTC peer connection.
// This is a subset of the actual webrtc.Peer interface we need for media service.
type WebRTCPeer interface {
	// AddTrack adds a track to the peer connection.
	AddTrack(track pion.TrackLocal) (*pion.RTPSender, error)
	// RemoveTrack removes a track from the peer connection.
	RemoveTrack(sender *pion.RTPSender) error
	// PeerConnection returns the underlying pion PeerConnection.
	PeerConnection() *pion.PeerConnection
}

// Service implements the MediaService interface for media operations.
// Task 1.7.3: Basic track forwarding implementation.
type Service struct {
	mu            sync.RWMutex
	mediaRouter   MediaRouter
	webrtcService WebRTCService

	// Track forwarding management
	forwarders map[SubscriptionID]*trackForwarder
}

// trackForwarder manages RTP packet forwarding from a remote track to a local track.
type trackForwarder struct {
	subscriptionID SubscriptionID
	remoteTrack    *pion.TrackRemote
	localTrack     *pion.TrackLocalStaticRTP
	sender         *pion.RTPSender
	cancel         context.CancelFunc
	done           chan struct{}
}

// NewService creates a new MediaService instance.
func NewService(mediaRouter MediaRouter, webrtcService WebRTCService) *Service {
	return &Service{
		mediaRouter:   mediaRouter,
		webrtcService: webrtcService,
		forwarders:    make(map[SubscriptionID]*trackForwarder),
	}
}

// PublishResponse contains the response data for a successful publish.
type PublishResponse struct {
	TrackID string
	Mid     string
}

// Publish handles a participant publishing a track.
// Phase 1: Stub implementation (actual publishing is handled by WebRTCEventsBridge.OnTrack).
func (s *Service) Publish(ctx context.Context, participantID string, kind protocol.TrackKind, simulcast bool, metadata map[string]interface{}, label string) (*PublishResponse, error) {
	// TODO(Phase 2+): Implement publish logic if needed
	// Currently, tracks are published via WebRTCEventsBridge.OnTrack
	return nil, fmt.Errorf("publish not yet implemented")
}

// Unpublish handles a participant unpublishing a track.
// Phase 1: Stub implementation.
func (s *Service) Unpublish(ctx context.Context, participantID string, trackID string) error {
	// TODO(Task 1.7.3+): Implement unpublish logic
	// - Remove track from MediaRouter
	// - Stop all subscriptions to this track
	// - Send trackUnpublished notification
	return fmt.Errorf("unpublish not yet implemented")
}

// SubscribeResponse contains the response data for a successful subscribe.
type SubscribeResponse struct {
	SubscriptionID string
	TrackID        string
	PublisherID    string
}

// Subscribe handles a participant subscribing to a track.
// Task 1.7.3: Implements subscriber track forwarding.
func (s *Service) Subscribe(ctx context.Context, subscriberID string, publisherID string, trackID string, preferredLayer protocol.SimulcastLayer) (*SubscribeResponse, error) {
	// Validate inputs
	if subscriberID == "" {
		return nil, fmt.Errorf("subscriber ID cannot be empty")
	}
	if trackID == "" {
		return nil, fmt.Errorf("track ID cannot be empty")
	}

	// Get subscriber peer connection
	subscriberPeer := s.webrtcService.GetPeer(subscriberID)
	if subscriberPeer == nil {
		return nil, fmt.Errorf("subscriber peer not found: %s", subscriberID)
	}

	// Convert protocol.SimulcastLayer to media.SimulcastLayer
	var mediaLayer SimulcastLayer
	switch preferredLayer {
	case protocol.SimulcastLayerHigh:
		mediaLayer = SimulcastLayerHigh
	case protocol.SimulcastLayerMedium:
		mediaLayer = SimulcastLayerMedium
	case protocol.SimulcastLayerLow:
		mediaLayer = SimulcastLayerLow
	default:
		mediaLayer = SimulcastLayerHigh // default to high
	}

	// Create subscription in MediaRouter
	opts := &SubscribeOptions{
		PreferredLayer: mediaLayer,
	}
	sub, err := s.mediaRouter.Subscribe(ctx, subscriberID, TrackID(trackID), opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}

	// Get the published track
	localTrack, err := s.mediaRouter.GetTrack(ctx, sub.TrackID)
	if err != nil {
		// Cleanup subscription
		_ = s.mediaRouter.Unsubscribe(ctx, sub.ID) //nolint:errcheck // Best effort cleanup
		return nil, fmt.Errorf("failed to get track: %w", err)
	}

	// Create TrackLocalStaticRTP for forwarding
	// Get codec capability from the remote track
	codec := localTrack.Track.Codec()

	// Create TrackLocalStaticRTP
	trackLocal, err := pion.NewTrackLocalStaticRTP(
		codec.RTPCodecCapability,
		localTrack.ID.String(),
		localTrack.PublisherID,
	)
	if err != nil {
		_ = s.mediaRouter.Unsubscribe(ctx, sub.ID) //nolint:errcheck // Best effort cleanup
		return nil, fmt.Errorf("failed to create local track: %w", err)
	}

	// Add track to subscriber's peer connection
	sender, err := subscriberPeer.AddTrack(trackLocal)
	if err != nil {
		_ = s.mediaRouter.Unsubscribe(ctx, sub.ID) //nolint:errcheck // Best effort cleanup
		return nil, fmt.Errorf("failed to add track to peer connection: %w", err)
	}

	// Create track forwarder
	forwardCtx, cancel := context.WithCancel(context.Background())
	forwarder := &trackForwarder{
		subscriptionID: sub.ID,
		remoteTrack:    localTrack.Track,
		localTrack:     trackLocal,
		sender:         sender,
		cancel:         cancel,
		done:           make(chan struct{}),
	}

	// Register forwarder
	s.mu.Lock()
	s.forwarders[sub.ID] = forwarder
	s.mu.Unlock()

	// Start RTP packet forwarding in background
	go s.forwardRTP(forwardCtx, forwarder)

	// OnNegotiationNeeded will be triggered automatically by pion/webrtc
	// when we call AddTrack. The WebRTC service will handle the event
	// and send an offer notification to the subscriber.

	return &SubscribeResponse{
		SubscriptionID: sub.ID.String(),
		TrackID:        sub.TrackID.String(),
		PublisherID:    sub.PublisherID,
	}, nil
}

// Unsubscribe handles a participant unsubscribing from a track.
// Task 1.7.3: Implements subscription cleanup.
func (s *Service) Unsubscribe(ctx context.Context, participantID string, subscriptionID string) error {
	if participantID == "" {
		return fmt.Errorf("participant ID cannot be empty")
	}
	if subscriptionID == "" {
		return fmt.Errorf("subscription ID cannot be empty")
	}

	subID := SubscriptionID(subscriptionID)
	if err := subID.Validate(); err != nil {
		return fmt.Errorf("invalid subscription ID: %w", err)
	}

	// Get forwarder
	s.mu.Lock()
	forwarder, exists := s.forwarders[subID]
	if exists {
		delete(s.forwarders, subID)
	}
	s.mu.Unlock()

	// Stop forwarder if it exists
	if forwarder != nil {
		forwarder.cancel()
		<-forwarder.done // Wait for forwarder to stop

		// Remove track from peer connection
		subscriberPeer := s.webrtcService.GetPeer(participantID)
		if subscriberPeer != nil {
			_ = subscriberPeer.RemoveTrack(forwarder.sender) //nolint:errcheck // Best effort cleanup
		}
	}

	// Remove subscription from MediaRouter
	if err := s.mediaRouter.Unsubscribe(ctx, subID); err != nil {
		return fmt.Errorf("failed to unsubscribe: %w", err)
	}

	// TODO(Task 1.7.3+): Trigger renegotiation after track removal
	// OnNegotiationNeeded should be triggered by pion/webrtc

	return nil
}

// SetPreferredLayer sets the preferred simulcast layer for a subscription.
// Phase 1: Stub implementation (simulcast layer switching will be implemented in Phase 3).
func (s *Service) SetPreferredLayer(ctx context.Context, participantID string, trackID string, layer protocol.SimulcastLayer) error {
	// TODO(Phase 3): Implement simulcast layer switching
	return fmt.Errorf("setPreferredLayer not yet implemented")
}

// forwardRTP forwards RTP packets from the remote track to the local track.
// This runs in a goroutine for each subscription.
func (s *Service) forwardRTP(ctx context.Context, fwd *trackForwarder) {
	defer close(fwd.done)

	// Read RTP packets from the remote track and write to the local track
	rtpBuf := make([]byte, 1500) // MTU size

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Read RTP packet from remote track
		n, _, err := fwd.remoteTrack.Read(rtpBuf)
		if err != nil {
			if err == io.EOF {
				return
			}
			// Log error but continue
			// TODO: Add proper logging
			continue
		}

		// Write RTP packet to local track
		if _, err := fwd.localTrack.Write(rtpBuf[:n]); err != nil {
			if err == io.ErrClosedPipe {
				return
			}
			// Log error but continue
			// TODO: Add proper logging
			continue
		}
	}
}

// Close closes the media service and stops all forwarders.
func (s *Service) Close() error {
	s.mu.Lock()
	forwarders := make([]*trackForwarder, 0, len(s.forwarders))
	for _, fwd := range s.forwarders {
		forwarders = append(forwarders, fwd)
	}
	s.forwarders = make(map[SubscriptionID]*trackForwarder)
	s.mu.Unlock()

	// Stop all forwarders
	for _, fwd := range forwarders {
		fwd.cancel()
		<-fwd.done
	}

	return nil
}
