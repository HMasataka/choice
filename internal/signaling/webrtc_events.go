package signaling

import (
	"context"
	"encoding/json"
	"sync"

	pion "github.com/pion/webrtc/v4"

	"github.com/HMasataka/choice/internal/media"
	"github.com/HMasataka/choice/internal/signaling/protocol"
	"github.com/HMasataka/choice/pkg/logger"
)

// ParticipantTracks tracks the number of tracks per participant.
type ParticipantTracks struct {
	VideoCount int
	AudioCount int
}

// WebRTCEventsBridge bridges WebRTC events to signaling notifications.
// Phase 1 implementation: Basic ICE candidate forwarding and state logging.
type WebRTCEventsBridge struct {
	notifier    *Notifier
	mediaRouter media.MediaRouter
	log         logger.Logger

	// mu protects participantConnections, participantTracks, and participantRooms maps
	mu                     sync.RWMutex
	participantConnections map[string]*Connection
	participantTracks      map[string]*ParticipantTracks
	participantRooms       map[string]string // participantID -> roomID
}

// NewWebRTCEventsBridge creates a new WebRTC events bridge.
func NewWebRTCEventsBridge(notifier *Notifier, mediaRouter media.MediaRouter, log logger.Logger) *WebRTCEventsBridge {
	return &WebRTCEventsBridge{
		notifier:               notifier,
		mediaRouter:            mediaRouter,
		log:                    log,
		participantConnections: make(map[string]*Connection),
		participantTracks:      make(map[string]*ParticipantTracks),
		participantRooms:       make(map[string]string),
	}
}

// RegisterParticipant registers a participant's connection for event routing.
func (b *WebRTCEventsBridge) RegisterParticipant(participantID string, conn *Connection) {
	if conn == nil || participantID == "" {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.participantConnections[participantID] = conn

	// Store room ID for later use in OnTrack
	if roomIDData, ok := conn.GetData("room_id"); ok {
		if roomID, ok := roomIDData.(string); ok {
			b.participantRooms[participantID] = roomID
		}
	}
}

// UnregisterParticipant unregisters a participant's connection.
func (b *WebRTCEventsBridge) UnregisterParticipant(participantID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.participantConnections, participantID)
	delete(b.participantTracks, participantID)
	delete(b.participantRooms, participantID)
}

// OnICEConnectionStateChange handles ICE connection state changes.
// Phase 1: Logs state changes for monitoring.
func (b *WebRTCEventsBridge) OnICEConnectionStateChange(participantID string, state pion.ICEConnectionState) {
	b.log.Info("ICE connection state changed",
		"participant_id", participantID,
		"state", state.String(),
	)

	// Phase 1: No notifications sent to clients for state changes
	// Phase 2+: Can add connectionQualityChanged or error notifications
}

// OnPeerConnectionStateChange handles peer connection state changes.
// Phase 1: Logs state changes for monitoring.
func (b *WebRTCEventsBridge) OnPeerConnectionStateChange(participantID string, state pion.PeerConnectionState) {
	b.log.Info("Peer connection state changed",
		"participant_id", participantID,
		"state", state.String(),
	)

	// Handle connection failures
	if state == pion.PeerConnectionStateFailed {
		b.log.Warn("Peer connection failed",
			"participant_id", participantID,
		)
		// Phase 2+: Send error notification to client
	}

	// Phase 1: No notifications sent to clients for state changes
	// Phase 2+: Can add serverStateChanged notifications
}

// OnICECandidate handles new ICE candidates (Trickle ICE).
// Phase 1: Forwards candidates to the participant via signaling.
func (b *WebRTCEventsBridge) OnICECandidate(participantID string, candidate pion.ICECandidateInit) {
	b.mu.RLock()
	conn := b.participantConnections[participantID]
	b.mu.RUnlock()

	if conn == nil {
		b.log.Debug("No connection for participant, dropping ICE candidate",
			"participant_id", participantID,
		)
		return
	}

	// Guard against nil SDPMid (Pion can emit candidates without SDPMid)
	if candidate.SDPMid == nil {
		b.log.Debug("ICE candidate has nil SDPMid, dropping",
			"participant_id", participantID,
		)
		return
	}

	// Convert *uint16 to *int for protocol struct
	var mLineIndex *int
	if candidate.SDPMLineIndex != nil {
		idx := int(*candidate.SDPMLineIndex)
		mLineIndex = &idx
	}

	// Create candidate notification
	notification, err := protocol.NewCandidateNotification(
		candidate.Candidate,
		*candidate.SDPMid,
		mLineIndex,
	)
	if err != nil {
		b.log.Error("Failed to create ICE candidate notification",
			"participant_id", participantID,
			"error", err,
		)
		return
	}

	// Marshal and send notification
	data, err := json.Marshal(notification)
	if err != nil {
		b.log.Error("Failed to marshal ICE candidate notification",
			"participant_id", participantID,
			"error", err,
		)
		return
	}

	if !conn.Send(data) {
		b.log.Error("Failed to send ICE candidate notification",
			"participant_id", participantID,
		)
	}
}

// getOrCreateParticipantTracks gets or creates track counts for a participant.
func (b *WebRTCEventsBridge) getOrCreateParticipantTracks(participantID string) *ParticipantTracks {
	if tracks, ok := b.participantTracks[participantID]; ok {
		return tracks
	}
	tracks := &ParticipantTracks{}
	b.participantTracks[participantID] = tracks
	return tracks
}

// checkTrackLimit checks if adding a track would exceed limits.
// Returns an error if the limit is exceeded.
// TODO: Make check + increment atomic to prevent race conditions when multiple tracks arrive concurrently
// TODO: Implement track count decrement on track end/unpublish (Task 1.7.3+)
func (b *WebRTCEventsBridge) checkTrackLimit(participantID string, kind pion.RTPCodecType) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	tracks := b.participantTracks[participantID]
	if tracks == nil {
		// No tracks yet, allow
		return nil
	}

	if kind == pion.RTPCodecTypeVideo && tracks.VideoCount >= 3 {
		return protocol.NewTrackLimitExceededError("video", 3)
	}

	if kind == pion.RTPCodecTypeAudio && tracks.AudioCount >= 2 {
		return protocol.NewTrackLimitExceededError("audio", 2)
	}

	return nil
}

// incrementTrackCount increments the track count for a participant.
func (b *WebRTCEventsBridge) incrementTrackCount(participantID string, kind pion.RTPCodecType) {
	b.mu.Lock()
	defer b.mu.Unlock()

	tracks := b.getOrCreateParticipantTracks(participantID)
	if kind == pion.RTPCodecTypeVideo {
		tracks.VideoCount++
	} else if kind == pion.RTPCodecTypeAudio {
		tracks.AudioCount++
	}
}

// OnTrack handles received tracks.
// Task 1.7.2: Implements track reception with limit checking and media routing.
func (b *WebRTCEventsBridge) OnTrack(participantID string, track *pion.TrackRemote, receiver *pion.RTPReceiver) {
	b.log.Info("Track received",
		"participant_id", participantID,
		"track_id", track.ID(),
		"kind", track.Kind().String(),
		"ssrc", track.SSRC(),
	)

	// Check track limits before accepting the track
	if err := b.checkTrackLimit(participantID, track.Kind()); err != nil {
		b.log.Warn("Track limit exceeded",
			"participant_id", participantID,
			"kind", track.Kind().String(),
			"error", err,
		)
		// TODO(Task 1.7.3+): Send error notification to participant for client visibility
		//   Use notifier to send protocol.NewTrackLimitExceededError as notification
		return
	}

	// Get room ID for this participant
	b.mu.RLock()
	roomID, hasRoom := b.participantRooms[participantID]
	b.mu.RUnlock()

	if !hasRoom || roomID == "" {
		b.log.Error("No room ID found for participant",
			"participant_id", participantID,
		)
		return
	}

	// Convert pion RTPCodecType to media.TrackKind
	var trackKind media.TrackKind
	switch track.Kind() {
	case pion.RTPCodecTypeVideo:
		trackKind = media.TrackKindVideo
	case pion.RTPCodecTypeAudio:
		trackKind = media.TrackKindAudio
	default:
		b.log.Error("Unknown track kind",
			"participant_id", participantID,
			"kind", track.Kind(),
		)
		return
	}

	// Build track metadata
	// TODO: Extract MID and RID from RTP extensions in Phase 1.6.4
	metadata := &media.TrackMetadata{
		SSRC: uint32(track.SSRC()),
		// MID and simulcast info will be populated by RTP processor
	}

	// Create LocalTrack
	localTrack := media.NewLocalTrack(participantID, roomID, trackKind, track, metadata)

	// Add track to MediaRouter
	ctx := context.Background()
	if err := b.mediaRouter.AddTrack(ctx, localTrack); err != nil {
		b.log.Error("Failed to add track to router",
			"participant_id", participantID,
			"track_id", localTrack.ID,
			"error", err,
		)
		return
	}

	// Increment track count
	b.incrementTrackCount(participantID, track.Kind())

	b.log.Info("Track added to router",
		"participant_id", participantID,
		"track_id", localTrack.ID,
		"kind", trackKind,
	)

	// Send trackPublished notification to all participants in the room
	b.notifier.NotifyTrackPublished(
		roomID,
		participantID,
		localTrack.ID.String(),
		protocol.TrackKind(trackKind.String()),
		localTrack.IsSimulcast(),
		nil, // metadata
		nil, // exclude nobody
	)
}

// OnNegotiationNeeded handles negotiation needed events.
// Phase 1: Logs the event.
func (b *WebRTCEventsBridge) OnNegotiationNeeded(participantID string) {
	b.log.Debug("Negotiation needed",
		"participant_id", participantID,
	)

	// Phase 1: No server-initiated renegotiation
	// Phase 2+: Create and send offer notification
}
