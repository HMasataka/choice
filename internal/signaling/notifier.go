package signaling

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/HMasataka/choice/internal/signaling/protocol"
)

// Notifier sends notifications to connections.
// It provides type-safe methods for sending all supported notification types.
type Notifier struct {
	// mu protects rooms and connRooms maps
	mu sync.RWMutex
	// rooms maps room IDs to sets of connections
	rooms map[string]map[*Connection]struct{}
	// connRooms maps connections to sets of room IDs for efficient cleanup
	connRooms map[*Connection]map[string]struct{}
}

// NewNotifier creates a new Notifier instance.
func NewNotifier() *Notifier {
	return &Notifier{
		rooms:     make(map[string]map[*Connection]struct{}),
		connRooms: make(map[*Connection]map[string]struct{}),
	}
}

// AddToRoom adds a connection to a room for broadcasting.
func (n *Notifier) AddToRoom(roomID string, conn *Connection) {
	if conn == nil || roomID == "" {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	// Add to rooms map
	if n.rooms[roomID] == nil {
		n.rooms[roomID] = make(map[*Connection]struct{})
	}
	n.rooms[roomID][conn] = struct{}{}

	// Track room membership for this connection
	if n.connRooms[conn] == nil {
		n.connRooms[conn] = make(map[string]struct{})
	}
	n.connRooms[conn][roomID] = struct{}{}
}

// RemoveFromRoom removes a connection from a room.
func (n *Notifier) RemoveFromRoom(roomID string, conn *Connection) {
	if conn == nil || roomID == "" {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	// Remove from rooms map
	if room := n.rooms[roomID]; room != nil {
		delete(room, conn)
		if len(room) == 0 {
			delete(n.rooms, roomID)
		}
	}

	// Remove from connection's room set
	if rooms := n.connRooms[conn]; rooms != nil {
		delete(rooms, roomID)
		if len(rooms) == 0 {
			delete(n.connRooms, conn)
		}
	}
}

// RemoveConnection removes a connection from all rooms.
// This should be called when a connection is closed to prevent stale references.
func (n *Notifier) RemoveConnection(conn *Connection) {
	if conn == nil {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	// Get all rooms this connection is in
	rooms := n.connRooms[conn]
	if rooms == nil {
		return
	}

	// Remove from each room
	for roomID := range rooms {
		if room := n.rooms[roomID]; room != nil {
			delete(room, conn)
			if len(room) == 0 {
				delete(n.rooms, roomID)
			}
		}
	}

	// Remove connection's room tracking
	delete(n.connRooms, conn)
}

// GetRoomConnections returns all connections in a room.
func (n *Notifier) GetRoomConnections(roomID string) []*Connection {
	n.mu.RLock()
	defer n.mu.RUnlock()

	room := n.rooms[roomID]
	if room == nil {
		return nil
	}

	conns := make([]*Connection, 0, len(room))
	for conn := range room {
		conns = append(conns, conn)
	}
	return conns
}

// sendToConnection sends a notification to a single connection.
func (n *Notifier) sendToConnection(conn *Connection, notif *protocol.Notification) bool {
	if conn == nil || notif == nil {
		return false
	}

	data, err := json.Marshal(notif)
	if err != nil {
		return false
	}

	return conn.Send(data)
}

// broadcastToRoom sends a notification to all connections in a room.
func (n *Notifier) broadcastToRoom(roomID string, notif *protocol.Notification, exclude *Connection) int {
	if notif == nil {
		return 0
	}

	data, err := json.Marshal(notif)
	if err != nil {
		return 0
	}

	n.mu.RLock()
	room := n.rooms[roomID]
	if room == nil {
		n.mu.RUnlock()
		fmt.Printf("[DEBUG] broadcastToRoom: room %s not found in notifier.rooms\n", roomID)
		return 0
	}

	// Copy connections to avoid holding lock during send
	conns := make([]*Connection, 0, len(room))
	for conn := range room {
		if conn != exclude {
			conns = append(conns, conn)
		}
	}
	n.mu.RUnlock()
	fmt.Printf("[DEBUG] broadcastToRoom: room=%s, conns=%d, method=%s\n", roomID, len(conns), notif.Method)

	sent := 0
	for _, conn := range conns {
		if conn.Send(data) {
			sent++
		}
	}
	return sent
}

// NotifyJoined sends a joined notification to the joining participant.
func (n *Notifier) NotifyJoined(conn *Connection, participantID, roomID string) bool {
	notif, err := protocol.NewJoinedNotification(participantID, roomID)
	if err != nil {
		return false
	}
	return n.sendToConnection(conn, notif)
}

// NotifyLeft sends a left notification to the leaving participant.
func (n *Notifier) NotifyLeft(conn *Connection, reason protocol.LeftReason) bool {
	notif, err := protocol.NewLeftNotification(reason)
	if err != nil {
		return false
	}
	return n.sendToConnection(conn, notif)
}

// NotifyParticipantJoined broadcasts a participantJoined notification to all participants in the room.
func (n *Notifier) NotifyParticipantJoined(roomID, participantID string, metadata map[string]interface{}, exclude *Connection) int {
	notif, err := protocol.NewParticipantJoinedNotification(participantID, metadata)
	if err != nil {
		return 0
	}
	return n.broadcastToRoom(roomID, notif, exclude)
}

// NotifyParticipantLeft broadcasts a participantLeft notification to all participants in the room.
func (n *Notifier) NotifyParticipantLeft(roomID, participantID string, reason protocol.LeaveReason, exclude *Connection) int {
	notif, err := protocol.NewParticipantLeftNotification(participantID, reason)
	if err != nil {
		return 0
	}
	return n.broadcastToRoom(roomID, notif, exclude)
}

// NotifyParticipantReconnected broadcasts a participantReconnected notification to all participants in the room.
func (n *Notifier) NotifyParticipantReconnected(roomID, participantID string, metadata map[string]interface{}, exclude *Connection) int {
	notif, err := protocol.NewParticipantReconnectedNotification(participantID, metadata)
	if err != nil {
		return 0
	}
	return n.broadcastToRoom(roomID, notif, exclude)
}

// NotifyTrackPublished broadcasts a trackPublished notification to all participants in the room.
func (n *Notifier) NotifyTrackPublished(roomID, publisherID, trackID string, kind protocol.TrackKind, simulcast bool, metadata map[string]interface{}, exclude *Connection) int {
	notif, err := protocol.NewTrackPublishedNotification(publisherID, trackID, kind, simulcast, metadata)
	if err != nil {
		return 0
	}
	return n.broadcastToRoom(roomID, notif, exclude)
}

// NotifyTrackUnpublished broadcasts a trackUnpublished notification to all participants in the room.
func (n *Notifier) NotifyTrackUnpublished(roomID, publisherID, trackID string, exclude *Connection) int {
	notif, err := protocol.NewTrackUnpublishedNotification(publisherID, trackID)
	if err != nil {
		return 0
	}
	return n.broadcastToRoom(roomID, notif, exclude)
}

// NotifyOffer sends an offer notification to a connection for server-initiated renegotiation.
func (n *Notifier) NotifyOffer(conn *Connection, sdp string, reason protocol.OfferReason) bool {
	notif, err := protocol.NewOfferNotification(sdp, reason)
	if err != nil {
		return false
	}
	return n.sendToConnection(conn, notif)
}

// NotifyCandidate sends an ICE candidate notification to a connection.
func (n *Notifier) NotifyCandidate(conn *Connection, candidate, sdpMid string, sdpMLineIndex *int) bool {
	notif, err := protocol.NewCandidateNotification(candidate, sdpMid, sdpMLineIndex)
	if err != nil {
		return false
	}
	return n.sendToConnection(conn, notif)
}

// NotifyAnswer sends an answer notification to a connection.
func (n *Notifier) NotifyAnswer(conn *Connection, sdp string) bool {
	notif, err := protocol.NewAnswerNotification(sdp)
	if err != nil {
		return false
	}
	return n.sendToConnection(conn, notif)
}

// NotifyLayerChanged sends a layerChanged notification to a connection.
func (n *Notifier) NotifyLayerChanged(conn *Connection, trackID string, requestedLayer, actualLayer protocol.SimulcastLayer, reason protocol.LayerChangeReason) bool {
	notif, err := protocol.NewLayerChangedNotification(trackID, requestedLayer, actualLayer, reason)
	if err != nil {
		return false
	}
	return n.sendToConnection(conn, notif)
}

// NotifyError sends an error notification to a connection.
func (n *Notifier) NotifyError(conn *Connection, code int, message string, fatal bool) bool {
	notif, err := protocol.NewErrorNotification(code, message, fatal)
	if err != nil {
		return false
	}
	return n.sendToConnection(conn, notif)
}

// NotifyReconnect sends a reconnect notification to a connection.
func (n *Notifier) NotifyReconnect(conn *Connection, reason protocol.ReconnectReason, retryAfterMs int) bool {
	notif, err := protocol.NewReconnectNotification(reason, retryAfterMs)
	if err != nil {
		return false
	}
	return n.sendToConnection(conn, notif)
}

// BroadcastError sends an error notification to all connections in a room.
func (n *Notifier) BroadcastError(roomID string, code int, message string, fatal bool, exclude *Connection) int {
	notif, err := protocol.NewErrorNotification(code, message, fatal)
	if err != nil {
		return 0
	}
	return n.broadcastToRoom(roomID, notif, exclude)
}

// BroadcastReconnect sends a reconnect notification to all connections in a room.
func (n *Notifier) BroadcastReconnect(roomID string, reason protocol.ReconnectReason, retryAfterMs int, exclude *Connection) int {
	notif, err := protocol.NewReconnectNotification(reason, retryAfterMs)
	if err != nil {
		return 0
	}
	return n.broadcastToRoom(roomID, notif, exclude)
}

// NotifyTrackSubscribed sends a trackSubscribed notification to a connection.
func (n *Notifier) NotifyTrackSubscribed(conn *Connection, subscriptionID, publisherID, trackID string, kind protocol.TrackKind) bool {
	notif, err := protocol.NewTrackSubscribedNotification(subscriptionID, publisherID, trackID, kind)
	if err != nil {
		return false
	}
	return n.sendToConnection(conn, notif)
}

// NotifyTrackSubscriptionFailed sends a trackSubscriptionFailed notification to a connection.
func (n *Notifier) NotifyTrackSubscriptionFailed(conn *Connection, publisherID, trackID string, errorCode int, errorMsg string) bool {
	notif, err := protocol.NewTrackSubscriptionFailedNotification(publisherID, trackID, errorCode, errorMsg)
	if err != nil {
		return false
	}
	return n.sendToConnection(conn, notif)
}

// NotifyConnectionQualityChanged sends a connectionQualityChanged notification to a connection.
func (n *Notifier) NotifyConnectionQualityChanged(conn *Connection, participantID string, quality protocol.ConnectionQuality, score float64) bool {
	notif, err := protocol.NewConnectionQualityChangedNotification(participantID, quality, score)
	if err != nil {
		return false
	}
	return n.sendToConnection(conn, notif)
}

// BroadcastConnectionQualityChanged broadcasts connectionQualityChanged notification to all participants in a room.
func (n *Notifier) BroadcastConnectionQualityChanged(roomID, participantID string, quality protocol.ConnectionQuality, score float64, exclude *Connection) int {
	notif, err := protocol.NewConnectionQualityChangedNotification(participantID, quality, score)
	if err != nil {
		return 0
	}
	return n.broadcastToRoom(roomID, notif, exclude)
}

// NotifyServerStateChanged sends a serverStateChanged notification to a connection.
func (n *Notifier) NotifyServerStateChanged(conn *Connection, roomID string, state protocol.ServerState, message string) bool {
	notif, err := protocol.NewServerStateChangedNotification(roomID, state, message)
	if err != nil {
		return false
	}
	return n.sendToConnection(conn, notif)
}

// BroadcastServerStateChanged broadcasts serverStateChanged notification to all participants in a room.
func (n *Notifier) BroadcastServerStateChanged(roomID string, state protocol.ServerState, message string, exclude *Connection) int {
	notif, err := protocol.NewServerStateChangedNotification(roomID, state, message)
	if err != nil {
		return 0
	}
	return n.broadcastToRoom(roomID, notif, exclude)
}

// GetRoomParticipants returns participant info for all connections in a room except the excluded one.
// participantConnections maps connection IDs to participant IDs.
func (n *Notifier) GetRoomParticipants(roomID string, exclude *Connection, participantConnections map[string]string) []protocol.ParticipantInfo {
	n.mu.RLock()
	defer n.mu.RUnlock()

	room := n.rooms[roomID]
	if room == nil {
		return []protocol.ParticipantInfo{}
	}

	participants := make([]protocol.ParticipantInfo, 0, len(room))
	for conn := range room {
		if conn == exclude {
			continue
		}
		participantID, ok := participantConnections[conn.ID()]
		if !ok {
			continue
		}
		participants = append(participants, protocol.ParticipantInfo{
			ID:       participantID,
			Metadata: nil,
			Tracks:   nil,
		})
	}
	return participants
}

// GetRoomParticipantsWithMetadata returns participant info with metadata for all connections in a room.
// participantConnections maps connection IDs to participant IDs.
// participantMetadata maps participant IDs to their metadata.
func (n *Notifier) GetRoomParticipantsWithMetadata(roomID string, exclude *Connection, participantConnections map[string]string, participantMetadata map[string]map[string]interface{}) []protocol.ParticipantInfo {
	n.mu.RLock()
	defer n.mu.RUnlock()

	room := n.rooms[roomID]
	if room == nil {
		return []protocol.ParticipantInfo{}
	}

	participants := make([]protocol.ParticipantInfo, 0, len(room))
	for conn := range room {
		if conn == exclude {
			continue
		}
		participantID, ok := participantConnections[conn.ID()]
		if !ok {
			continue
		}
		metadata := participantMetadata[participantID]
		participants = append(participants, protocol.ParticipantInfo{
			ID:       participantID,
			Metadata: metadata,
			Tracks:   nil,
		})
	}
	return participants
}
