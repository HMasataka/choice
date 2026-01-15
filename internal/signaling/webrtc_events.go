package signaling

import (
	"encoding/json"
	"sync"

	pion "github.com/pion/webrtc/v4"

	"github.com/HMasataka/choice/internal/signaling/protocol"
	"github.com/HMasataka/choice/pkg/logger"
)

// WebRTCEventsBridge bridges WebRTC events to signaling notifications.
// Phase 1 implementation: Basic ICE candidate forwarding and state logging.
type WebRTCEventsBridge struct {
	notifier *Notifier
	log      logger.Logger

	// mu protects participantConnections map
	mu                     sync.RWMutex
	participantConnections map[string]*Connection
}

// NewWebRTCEventsBridge creates a new WebRTC events bridge.
func NewWebRTCEventsBridge(notifier *Notifier, log logger.Logger) *WebRTCEventsBridge {
	return &WebRTCEventsBridge{
		notifier:               notifier,
		log:                    log,
		participantConnections: make(map[string]*Connection),
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
}

// UnregisterParticipant unregisters a participant's connection.
func (b *WebRTCEventsBridge) UnregisterParticipant(participantID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.participantConnections, participantID)
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

// OnTrack handles received tracks.
// Phase 1: Logs track reception.
func (b *WebRTCEventsBridge) OnTrack(participantID string, track *pion.TrackRemote, receiver *pion.RTPReceiver) {
	b.log.Info("Track received",
		"participant_id", participantID,
		"track_id", track.ID(),
		"kind", track.Kind().String(),
		"ssrc", track.SSRC(),
	)

	// Phase 1: No media routing yet (Task 1.7.2)
	// Phase 2+: Add track to MediaRouter
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
