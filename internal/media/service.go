package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtcp"
	pion "github.com/pion/webrtc/v4"

	"github.com/HMasataka/choice/internal/signaling/protocol"
)

// trackSequence is used to generate unique track IDs
var trackSequence uint64

// generateTrackSequence returns a unique sequence number for track IDs
func generateTrackSequence() uint64 {
	return atomic.AddUint64(&trackSequence, 1)
}

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
// The client sends this request to signal intent to publish, but actual track handling
// happens via WebRTC negotiation (OnTrack callback).
func (s *Service) Publish(ctx context.Context, participantID string, kind protocol.TrackKind, simulcast bool, metadata map[string]interface{}, label string) (*PublishResponse, error) {
	// Generate a track ID for the client
	// In Phase 1, we just return a stub response. The actual track will be registered
	// when WebRTCEventsBridge.OnTrack is called after SDP negotiation.
	trackID := fmt.Sprintf("%s-%s-%d", participantID, kind, generateTrackSequence())

	return &PublishResponse{
		TrackID: trackID,
		Mid:     "0", // Mid will be determined during SDP negotiation
	}, nil
}

// Unpublish handles a participant unpublishing a track.
// Phase 1: Stub implementation.
func (s *Service) Unpublish(ctx context.Context, participantID string, trackID string) error {
	// TODO(Task 1.7.3+): Implement unpublish logic
	// - Remove track from MediaRouter
	// - Stop all subscriptions to this track
	// - Send trackUnpublished notification
	// For now, just return success
	return nil
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

	// Request a keyframe from the publisher BEFORE starting the forwarder
	// This ensures the subscriber can start decoding immediately
	if localTrack.Kind == TrackKindVideo {
		s.requestKeyframe(localTrack.PublisherID, uint32(localTrack.Track.SSRC()))
		// Wait a bit for the keyframe to be generated
		// This increases latency slightly but ensures video can be decoded
		time.Sleep(50 * time.Millisecond)
	}

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
	// For now, just return success
	return nil
}

// GetTracksForParticipant returns all tracks published by a participant.
func (s *Service) GetTracksForParticipant(ctx context.Context, participantID string) []protocol.TrackInfo {
	tracks, err := s.mediaRouter.ListTracks(ctx)
	if err != nil {
		return nil
	}

	var result []protocol.TrackInfo
	for _, track := range tracks {
		if track.PublisherID == participantID {
			kind := protocol.TrackKindVideo
			if track.Kind == TrackKindAudio {
				kind = protocol.TrackKindAudio
			}
			// Convert TrackMetadata to map[string]interface{} if present
			var metadata map[string]interface{}
			if meta := track.GetMetadata(); meta != nil {
				metadata = map[string]interface{}{
					"label":     meta.Label,
					"simulcast": meta.Simulcast,
				}
			}
			result = append(result, protocol.TrackInfo{
				TrackID:   track.ID.String(),
				Kind:      kind,
				Simulcast: track.IsSimulcast(),
				Metadata:  metadata,
			})
		}
	}
	return result
}

// forwardRTP forwards RTP packets from the remote track to the local track.
// This runs in a goroutine for each subscription.
// For video tracks, it waits for a keyframe before starting to forward.
func (s *Service) forwardRTP(ctx context.Context, fwd *trackForwarder) {
	defer close(fwd.done)

	isVideo := fwd.remoteTrack.Kind().String() == "video"
	fmt.Printf("[DEBUG] forwardRTP started: subscriptionID=%s, track=%s, kind=%s, isVideo=%v\n",
		fwd.subscriptionID, fwd.localTrack.ID(), fwd.remoteTrack.Kind().String(), isVideo)

	// Read RTP packets from the remote track and write to the local track
	rtpBuf := make([]byte, 1500) // MTU size
	packetCount := 0
	waitingForKeyframe := isVideo // Only wait for keyframe if video track
	keyframeWaitStarted := time.Now()
	const maxKeyframeWait = 5 * time.Second

	for {
		select {
		case <-ctx.Done():
			fmt.Printf("[DEBUG] forwardRTP stopped (context done): subscriptionID=%s, packets=%d\n",
				fwd.subscriptionID, packetCount)
			return
		default:
		}

		// Read RTP packet from remote track
		n, _, err := fwd.remoteTrack.Read(rtpBuf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Printf("[DEBUG] forwardRTP: EOF on read, stopping: subscriptionID=%s, packets=%d\n",
					fwd.subscriptionID, packetCount)
				return
			}
			// Log error but continue
			fmt.Printf("[DEBUG] forwardRTP: read error: %v\n", err)
			continue
		}

		// For video, wait for a keyframe before starting to forward
		if waitingForKeyframe {
			// Check if this is a keyframe
			if isVP8Keyframe(rtpBuf[:n]) {
				waitingForKeyframe = false
				fmt.Printf("[DEBUG] forwardRTP: VP8 keyframe detected, starting forwarding: subscription=%s, waitTime=%v\n",
					fwd.subscriptionID, time.Since(keyframeWaitStarted))
			} else {
				// Check timeout
				if time.Since(keyframeWaitStarted) > maxKeyframeWait {
					waitingForKeyframe = false
					fmt.Printf("[DEBUG] forwardRTP: keyframe wait timeout, starting forwarding anyway: subscription=%s\n",
						fwd.subscriptionID)
				} else {
					// Discard this packet and continue waiting
					continue
				}
			}
		}

		// Write RTP packet to local track
		if _, err := fwd.localTrack.Write(rtpBuf[:n]); err != nil {
			if errors.Is(err, io.ErrClosedPipe) {
				fmt.Printf("[DEBUG] forwardRTP: pipe closed, stopping: subscriptionID=%s, packets=%d\n",
					fwd.subscriptionID, packetCount)
				return
			}
			// Log error but continue
			fmt.Printf("[DEBUG] forwardRTP: write error: %v\n", err)
			continue
		}

		packetCount++
		if packetCount == 1 || packetCount%100 == 0 {
			fmt.Printf("[DEBUG] forwardRTP: forwarded %d packets, subscription=%s, kind=%s, lastSize=%d\n",
				packetCount, fwd.subscriptionID, fwd.remoteTrack.Kind().String(), n)
		}
	}
}

// isVP8Keyframe checks if an RTP packet contains a VP8 keyframe.
// VP8 keyframes have a specific bit pattern in the RTP payload.
func isVP8Keyframe(rtpPacket []byte) bool {
	// RTP header is at least 12 bytes
	if len(rtpPacket) < 13 {
		return false
	}

	// Get RTP header length (12 bytes + CSRC count * 4 + extension)
	headerLen := 12
	csrcCount := int(rtpPacket[0] & 0x0F)
	headerLen += csrcCount * 4

	// Check for extension
	if rtpPacket[0]&0x10 != 0 {
		if len(rtpPacket) < headerLen+4 {
			return false
		}
		extLen := int(rtpPacket[headerLen+2])<<8 | int(rtpPacket[headerLen+3])
		headerLen += 4 + extLen*4
	}

	if len(rtpPacket) <= headerLen {
		return false
	}

	// VP8 payload descriptor (first byte of payload)
	// X bit (0x80): Extended control bits present
	// R bit (0x40): Reserved
	// N bit (0x20): Non-reference frame
	// S bit (0x10): Start of VP8 partition
	// PID bits (0x0F): Partition index
	payloadDescriptor := rtpPacket[headerLen]

	// Check S bit - must be 1 for start of partition
	if payloadDescriptor&0x10 == 0 {
		return false
	}

	// Skip VP8 payload descriptor (variable length)
	payloadOffset := headerLen + 1

	// X bit - extended control bits
	if payloadDescriptor&0x80 != 0 {
		if len(rtpPacket) <= payloadOffset {
			return false
		}
		extByte := rtpPacket[payloadOffset]
		payloadOffset++

		// I bit - PictureID present
		if extByte&0x80 != 0 {
			if len(rtpPacket) <= payloadOffset {
				return false
			}
			// Check M bit for extended PictureID
			if rtpPacket[payloadOffset]&0x80 != 0 {
				payloadOffset++ // Skip extended PictureID
			}
			payloadOffset++
		}

		// L bit - TL0PICIDX present
		if extByte&0x40 != 0 {
			payloadOffset++
		}

		// T or K bit - TID/KEYIDX present
		if extByte&0x20 != 0 || extByte&0x10 != 0 {
			payloadOffset++
		}
	}

	if len(rtpPacket) <= payloadOffset {
		return false
	}

	// VP8 payload header (first byte after descriptor)
	// For keyframes, the P bit (bit 0) should be 0
	payloadHeader := rtpPacket[payloadOffset]
	isKeyframe := (payloadHeader & 0x01) == 0

	return isKeyframe
}

// requestKeyframe sends PLI (Picture Loss Indication) packets to the publisher to request a keyframe.
// This is called when a new subscriber joins to ensure they can start decoding immediately.
// Multiple PLIs are sent because browsers may not respond to a single PLI immediately.
func (s *Service) requestKeyframe(publisherID string, ssrc uint32) {
	publisherPeer := s.webrtcService.GetPeer(publisherID)
	if publisherPeer == nil {
		fmt.Printf("[DEBUG] requestKeyframe: publisher peer not found: %s\n", publisherID)
		return
	}

	pc := publisherPeer.PeerConnection()
	if pc == nil {
		fmt.Printf("[DEBUG] requestKeyframe: peer connection is nil for publisher: %s\n", publisherID)
		return
	}

	// Send multiple PLIs to ensure the publisher responds with a keyframe
	// Browsers may not respond to a single PLI immediately
	sendPLI := func() {
		pli := &rtcp.PictureLossIndication{
			SenderSSRC: 0, // SFU's SSRC (not relevant for PLI)
			MediaSSRC:  ssrc,
		}
		if err := pc.WriteRTCP([]rtcp.Packet{pli}); err != nil {
			fmt.Printf("[DEBUG] requestKeyframe: failed to send PLI to publisher %s: %v\n", publisherID, err)
			return
		}
		fmt.Printf("[DEBUG] requestKeyframe: sent PLI to publisher %s for SSRC %d\n", publisherID, ssrc)
	}

	// Send PLI immediately
	sendPLI()

	// Send additional PLIs after delays to ensure keyframe delivery
	go func() {
		time.Sleep(100 * time.Millisecond)
		sendPLI()
		time.Sleep(200 * time.Millisecond)
		sendPLI()
		time.Sleep(500 * time.Millisecond)
		sendPLI()
	}()
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
