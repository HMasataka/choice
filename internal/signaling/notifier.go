package signaling

import (
	"encoding/json"
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
