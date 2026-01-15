package signaling

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/HMasataka/choice/internal/signaling/protocol"
)

// testConnection wraps a Connection and captures sent messages for testing.
type testConnection struct {
	*Connection
	mu       sync.Mutex
	messages [][]byte
}

func newTestConnection(id string) *testConnection {
	conn := &Connection{
		id:   id,
		data: make(map[string]interface{}),
		send: make(chan []byte, 100),
		done: make(chan struct{}),
	}
	// Set state to connected so Send() works
	conn.state.Store(int32(StateConnected))
	tc := &testConnection{
		Connection: conn,
		messages:   make([][]byte, 0),
	}
	// Start a goroutine to capture messages
	go func() {
		for {
			select {
			case msg, ok := <-conn.send:
				if !ok {
					return
				}
				tc.mu.Lock()
				tc.messages = append(tc.messages, msg)
				tc.mu.Unlock()
			case <-conn.done:
				return
			}
		}
	}()
	return tc
}

func (tc *testConnection) GetMessages() [][]byte {
	// Allow goroutine to process messages
	time.Sleep(10 * time.Millisecond)
	tc.mu.Lock()
	defer tc.mu.Unlock()
	result := make([][]byte, len(tc.messages))
	copy(result, tc.messages)
	return result
}

func (tc *testConnection) Close() {
	close(tc.done)
}

func TestNotifier_AddRemoveRoom(t *testing.T) {
	notifier := NewNotifier()

	conn1 := &Connection{id: "conn-1", data: make(map[string]interface{})}
	conn2 := &Connection{id: "conn-2", data: make(map[string]interface{})}

	// Add to room
	notifier.AddToRoom("room-1", conn1)
	notifier.AddToRoom("room-1", conn2)

	conns := notifier.GetRoomConnections("room-1")
	if len(conns) != 2 {
		t.Errorf("expected 2 connections, got %d", len(conns))
	}

	// Remove from room
	notifier.RemoveFromRoom("room-1", conn1)
	conns = notifier.GetRoomConnections("room-1")
	if len(conns) != 1 {
		t.Errorf("expected 1 connection after removal, got %d", len(conns))
	}

	// Remove last connection
	notifier.RemoveFromRoom("room-1", conn2)
	conns = notifier.GetRoomConnections("room-1")
	if len(conns) != 0 {
		t.Errorf("expected 0 connections after all removals, got %d", len(conns))
	}
}

func TestNotifier_AddToRoom_NilConnection(t *testing.T) {
	notifier := NewNotifier()

	// Should not panic
	notifier.AddToRoom("room-1", nil)

	conns := notifier.GetRoomConnections("room-1")
	if len(conns) != 0 {
		t.Errorf("expected 0 connections, got %d", len(conns))
	}
}

func TestNotifier_AddToRoom_EmptyRoomID(t *testing.T) {
	notifier := NewNotifier()

	conn := &Connection{id: "conn-1", data: make(map[string]interface{})}

	// Should not add with empty room ID
	notifier.AddToRoom("", conn)

	// Verify no rooms were created
	notifier.mu.RLock()
	roomCount := len(notifier.rooms)
	notifier.mu.RUnlock()

	if roomCount != 0 {
		t.Errorf("expected 0 rooms, got %d", roomCount)
	}
}

func TestNotifier_GetRoomConnections_NonExistentRoom(t *testing.T) {
	notifier := NewNotifier()

	conns := notifier.GetRoomConnections("non-existent-room")
	if conns != nil {
		t.Error("expected nil for non-existent room")
	}
}

func TestNotifier_NotifyJoined(t *testing.T) {
	notifier := NewNotifier()

	conn := newTestConnection("conn-1")

	// Test NotifyJoined
	result := notifier.NotifyJoined(conn.Connection, "participant-1", "room-1")
	if !result {
		t.Error("NotifyJoined should return true")
	}

	// Verify message
	messages := conn.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var notif protocol.Notification
	if err := json.Unmarshal(messages[0], &notif); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	if notif.Method != protocol.NotifyJoined {
		t.Errorf("expected method %s, got %s", protocol.NotifyJoined, notif.Method)
	}

	var params protocol.JoinedParams
	if err := notif.UnmarshalParams(&params); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}

	if params.ParticipantID != "participant-1" {
		t.Errorf("expected participantId participant-1, got %s", params.ParticipantID)
	}

	if params.RoomID != "room-1" {
		t.Errorf("expected roomId room-1, got %s", params.RoomID)
	}
}

func TestNotifier_NotifyJoined_NilConnection(t *testing.T) {
	notifier := NewNotifier()

	result := notifier.NotifyJoined(nil, "participant-1", "room-1")
	if result {
		t.Error("NotifyJoined should return false for nil connection")
	}
}

func TestNotifier_NotifyLeft(t *testing.T) {
	notifier := NewNotifier()

	conn := newTestConnection("conn-1")

	result := notifier.NotifyLeft(conn.Connection, protocol.LeftReasonVoluntary)
	if !result {
		t.Error("NotifyLeft should return true")
	}

	messages := conn.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var notif protocol.Notification
	if err := json.Unmarshal(messages[0], &notif); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	if notif.Method != protocol.NotifyLeft {
		t.Errorf("expected method %s, got %s", protocol.NotifyLeft, notif.Method)
	}

	var params protocol.LeftParams
	if err := notif.UnmarshalParams(&params); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}

	if params.Reason != protocol.LeftReasonVoluntary {
		t.Errorf("expected reason %s, got %s", protocol.LeftReasonVoluntary, params.Reason)
	}
}

func TestNotifier_NotifyParticipantJoined(t *testing.T) {
	notifier := NewNotifier()

	conn1 := newTestConnection("conn-1")
	conn2 := newTestConnection("conn-2")
	joinedConn := newTestConnection("conn-joined")

	// Add connections to room
	notifier.AddToRoom("room-1", conn1.Connection)
	notifier.AddToRoom("room-1", conn2.Connection)
	notifier.AddToRoom("room-1", joinedConn.Connection)

	// Broadcast participantJoined, excluding the joined connection
	metadata := map[string]interface{}{"name": "Test User"}
	sent := notifier.NotifyParticipantJoined("room-1", "new-participant", metadata, joinedConn.Connection)

	if sent != 2 {
		t.Errorf("expected 2 messages sent, got %d", sent)
	}

	// Verify conn1 received the notification
	messages := conn1.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("conn1 should have 1 message, got %d", len(messages))
	}

	var notif protocol.Notification
	if err := json.Unmarshal(messages[0], &notif); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	if notif.Method != protocol.NotifyParticipantJoined {
		t.Errorf("expected method %s, got %s", protocol.NotifyParticipantJoined, notif.Method)
	}

	// Verify joinedConn did NOT receive the notification
	joinedMessages := joinedConn.GetMessages()
	if len(joinedMessages) != 0 {
		t.Errorf("joinedConn should not receive notification, got %d messages", len(joinedMessages))
	}
}

func TestNotifier_NotifyParticipantLeft(t *testing.T) {
	notifier := NewNotifier()

	conn1 := newTestConnection("conn-1")
	conn2 := newTestConnection("conn-2")

	notifier.AddToRoom("room-1", conn1.Connection)
	notifier.AddToRoom("room-1", conn2.Connection)

	sent := notifier.NotifyParticipantLeft("room-1", "leaving-participant", protocol.LeaveReasonLeave, nil)

	if sent != 2 {
		t.Errorf("expected 2 messages sent, got %d", sent)
	}

	// Verify notification content
	messages := conn1.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var notif protocol.Notification
	if err := json.Unmarshal(messages[0], &notif); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	if notif.Method != protocol.NotifyParticipantLeft {
		t.Errorf("expected method %s, got %s", protocol.NotifyParticipantLeft, notif.Method)
	}

	var params protocol.ParticipantLeftParams
	if err := notif.UnmarshalParams(&params); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}

	if params.ParticipantID != "leaving-participant" {
		t.Errorf("expected participantId leaving-participant, got %s", params.ParticipantID)
	}

	if params.Reason != protocol.LeaveReasonLeave {
		t.Errorf("expected reason %s, got %s", protocol.LeaveReasonLeave, params.Reason)
	}
}

func TestNotifier_NotifyTrackPublished(t *testing.T) {
	notifier := NewNotifier()

	conn1 := newTestConnection("conn-1")
	publisherConn := newTestConnection("publisher-conn")

	notifier.AddToRoom("room-1", conn1.Connection)
	notifier.AddToRoom("room-1", publisherConn.Connection)

	metadata := map[string]interface{}{"label": "camera"}
	sent := notifier.NotifyTrackPublished("room-1", "publisher-1", "track-1", protocol.TrackKindVideo, true, metadata, publisherConn.Connection)

	if sent != 1 {
		t.Errorf("expected 1 message sent (excluding publisher), got %d", sent)
	}

	messages := conn1.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var notif protocol.Notification
	if err := json.Unmarshal(messages[0], &notif); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	if notif.Method != protocol.NotifyTrackPublished {
		t.Errorf("expected method %s, got %s", protocol.NotifyTrackPublished, notif.Method)
	}

	var params protocol.TrackPublishedParams
	if err := notif.UnmarshalParams(&params); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}

	if params.PublisherID != "publisher-1" {
		t.Errorf("expected publisherId publisher-1, got %s", params.PublisherID)
	}

	if params.TrackID != "track-1" {
		t.Errorf("expected trackId track-1, got %s", params.TrackID)
	}

	if params.Kind != protocol.TrackKindVideo {
		t.Errorf("expected kind video, got %s", params.Kind)
	}

	if !params.Simulcast {
		t.Error("expected simulcast true")
	}
}

func TestNotifier_NotifyTrackUnpublished(t *testing.T) {
	notifier := NewNotifier()

	conn1 := newTestConnection("conn-1")

	notifier.AddToRoom("room-1", conn1.Connection)

	sent := notifier.NotifyTrackUnpublished("room-1", "publisher-1", "track-1", nil)

	if sent != 1 {
		t.Errorf("expected 1 message sent, got %d", sent)
	}

	messages := conn1.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var notif protocol.Notification
	if err := json.Unmarshal(messages[0], &notif); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	if notif.Method != protocol.NotifyTrackUnpublished {
		t.Errorf("expected method %s, got %s", protocol.NotifyTrackUnpublished, notif.Method)
	}

	var params protocol.TrackUnpublishedParams
	if err := notif.UnmarshalParams(&params); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}

	if params.PublisherID != "publisher-1" {
		t.Errorf("expected publisherId publisher-1, got %s", params.PublisherID)
	}

	if params.TrackID != "track-1" {
		t.Errorf("expected trackId track-1, got %s", params.TrackID)
	}
}

func TestNotifier_BroadcastToEmptyRoom(t *testing.T) {
	notifier := NewNotifier()

	// Broadcast to non-existent room
	sent := notifier.NotifyParticipantJoined("non-existent-room", "participant-1", nil, nil)

	if sent != 0 {
		t.Errorf("expected 0 messages sent to empty room, got %d", sent)
	}
}

func TestNotifier_ConcurrentAccess(t *testing.T) {
	notifier := NewNotifier()

	var wg sync.WaitGroup

	// Concurrently add and remove connections
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			conn := &Connection{id: string(rune('a' + id%26)), data: make(map[string]interface{})}
			notifier.AddToRoom("room-1", conn)
			notifier.GetRoomConnections("room-1")
			notifier.RemoveFromRoom("room-1", conn)
		}(i)
	}

	wg.Wait()
}

func TestNotifier_RemoveConnection(t *testing.T) {
	notifier := NewNotifier()

	conn := &Connection{id: "conn-1", data: make(map[string]interface{})}

	// Add connection to multiple rooms
	notifier.AddToRoom("room-1", conn)
	notifier.AddToRoom("room-2", conn)
	notifier.AddToRoom("room-3", conn)

	// Verify connection is in all rooms
	if conns := notifier.GetRoomConnections("room-1"); len(conns) != 1 {
		t.Errorf("expected 1 connection in room-1, got %d", len(conns))
	}
	if conns := notifier.GetRoomConnections("room-2"); len(conns) != 1 {
		t.Errorf("expected 1 connection in room-2, got %d", len(conns))
	}
	if conns := notifier.GetRoomConnections("room-3"); len(conns) != 1 {
		t.Errorf("expected 1 connection in room-3, got %d", len(conns))
	}

	// Remove connection from all rooms
	notifier.RemoveConnection(conn)

	// Verify connection is removed from all rooms
	if conns := notifier.GetRoomConnections("room-1"); conns != nil {
		t.Errorf("expected nil for room-1, got %d connections", len(conns))
	}
	if conns := notifier.GetRoomConnections("room-2"); conns != nil {
		t.Errorf("expected nil for room-2, got %d connections", len(conns))
	}
	if conns := notifier.GetRoomConnections("room-3"); conns != nil {
		t.Errorf("expected nil for room-3, got %d connections", len(conns))
	}
}

func TestNotifier_RemoveConnection_NilConnection(t *testing.T) {
	notifier := NewNotifier()

	// Should not panic
	notifier.RemoveConnection(nil)
}

func TestNotifier_RemoveConnection_NotInAnyRoom(t *testing.T) {
	notifier := NewNotifier()

	conn := &Connection{id: "conn-1", data: make(map[string]interface{})}

	// Should not panic when removing connection that's not in any room
	notifier.RemoveConnection(conn)
}

func TestNotifier_NotifyOffer(t *testing.T) {
	notifier := NewNotifier()

	conn := newTestConnection("conn-1")

	result := notifier.NotifyOffer(conn.Connection, "v=0\r\no=...", protocol.OfferReasonTrackAdded)
	if !result {
		t.Error("NotifyOffer should return true")
	}

	messages := conn.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var notif protocol.Notification
	if err := json.Unmarshal(messages[0], &notif); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	if notif.Method != protocol.NotifyOffer {
		t.Errorf("expected method %s, got %s", protocol.NotifyOffer, notif.Method)
	}

	var params protocol.OfferNotificationParams
	if err := notif.UnmarshalParams(&params); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}

	if params.SDP != "v=0\r\no=..." {
		t.Errorf("expected sdp v=0\\r\\no=..., got %s", params.SDP)
	}

	if params.Reason != protocol.OfferReasonTrackAdded {
		t.Errorf("expected reason %s, got %s", protocol.OfferReasonTrackAdded, params.Reason)
	}
}

func TestNotifier_NotifyOffer_NilConnection(t *testing.T) {
	notifier := NewNotifier()

	result := notifier.NotifyOffer(nil, "sdp", protocol.OfferReasonTrackAdded)
	if result {
		t.Error("NotifyOffer should return false for nil connection")
	}
}

func TestNotifier_NotifyCandidate(t *testing.T) {
	notifier := NewNotifier()

	conn := newTestConnection("conn-1")

	sdpMLineIndex := 0
	result := notifier.NotifyCandidate(conn.Connection, "candidate:1234", "audio", &sdpMLineIndex)
	if !result {
		t.Error("NotifyCandidate should return true")
	}

	messages := conn.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var notif protocol.Notification
	if err := json.Unmarshal(messages[0], &notif); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	if notif.Method != protocol.NotifyCandidate {
		t.Errorf("expected method %s, got %s", protocol.NotifyCandidate, notif.Method)
	}

	var params protocol.CandidateNotificationParams
	if err := notif.UnmarshalParams(&params); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}

	if params.Candidate != "candidate:1234" {
		t.Errorf("expected candidate candidate:1234, got %s", params.Candidate)
	}

	if params.SDPMid != "audio" {
		t.Errorf("expected sdpMid audio, got %s", params.SDPMid)
	}

	if params.SDPMLineIndex == nil || *params.SDPMLineIndex != 0 {
		t.Errorf("expected sdpMLineIndex 0, got %v", params.SDPMLineIndex)
	}
}

func TestNotifier_NotifyAnswer(t *testing.T) {
	notifier := NewNotifier()

	conn := newTestConnection("conn-1")

	result := notifier.NotifyAnswer(conn.Connection, "v=0\r\no=answer...")
	if !result {
		t.Error("NotifyAnswer should return true")
	}

	messages := conn.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var notif protocol.Notification
	if err := json.Unmarshal(messages[0], &notif); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	if notif.Method != protocol.NotifyAnswer {
		t.Errorf("expected method %s, got %s", protocol.NotifyAnswer, notif.Method)
	}

	var params protocol.AnswerNotificationParams
	if err := notif.UnmarshalParams(&params); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}

	if params.SDP != "v=0\r\no=answer..." {
		t.Errorf("expected sdp v=0\\r\\no=answer..., got %s", params.SDP)
	}
}

func TestNotifier_NotifyLayerChanged(t *testing.T) {
	notifier := NewNotifier()

	conn := newTestConnection("conn-1")

	result := notifier.NotifyLayerChanged(conn.Connection, "track-1", protocol.SimulcastLayerHigh, protocol.SimulcastLayerMedium, protocol.LayerChangeReasonBandwidth)
	if !result {
		t.Error("NotifyLayerChanged should return true")
	}

	messages := conn.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var notif protocol.Notification
	if err := json.Unmarshal(messages[0], &notif); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	if notif.Method != protocol.NotifyLayerChanged {
		t.Errorf("expected method %s, got %s", protocol.NotifyLayerChanged, notif.Method)
	}

	var params protocol.LayerChangedParams
	if err := notif.UnmarshalParams(&params); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}

	if params.TrackID != "track-1" {
		t.Errorf("expected trackId track-1, got %s", params.TrackID)
	}

	if params.RequestedLayer != protocol.SimulcastLayerHigh {
		t.Errorf("expected requestedLayer h, got %s", params.RequestedLayer)
	}

	if params.ActualLayer != protocol.SimulcastLayerMedium {
		t.Errorf("expected actualLayer m, got %s", params.ActualLayer)
	}

	if params.Reason != protocol.LayerChangeReasonBandwidth {
		t.Errorf("expected reason bandwidth, got %s", params.Reason)
	}
}

func TestNotifier_NotifyError(t *testing.T) {
	notifier := NewNotifier()

	conn := newTestConnection("conn-1")

	result := notifier.NotifyError(conn.Connection, 1001, "Room not found", false)
	if !result {
		t.Error("NotifyError should return true")
	}

	messages := conn.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var notif protocol.Notification
	if err := json.Unmarshal(messages[0], &notif); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	if notif.Method != protocol.NotifyError {
		t.Errorf("expected method %s, got %s", protocol.NotifyError, notif.Method)
	}

	var params protocol.ErrorNotificationParams
	if err := notif.UnmarshalParams(&params); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}

	if params.Code != 1001 {
		t.Errorf("expected code 1001, got %d", params.Code)
	}

	if params.Message != "Room not found" {
		t.Errorf("expected message Room not found, got %s", params.Message)
	}

	if params.Fatal {
		t.Error("expected fatal false")
	}
}

func TestNotifier_NotifyError_Fatal(t *testing.T) {
	notifier := NewNotifier()

	conn := newTestConnection("conn-1")

	result := notifier.NotifyError(conn.Connection, 1000, "Server error", true)
	if !result {
		t.Error("NotifyError should return true")
	}

	messages := conn.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var notif protocol.Notification
	if err := json.Unmarshal(messages[0], &notif); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	var params protocol.ErrorNotificationParams
	if err := notif.UnmarshalParams(&params); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}

	if !params.Fatal {
		t.Error("expected fatal true")
	}
}

func TestNotifier_NotifyReconnect(t *testing.T) {
	notifier := NewNotifier()

	conn := newTestConnection("conn-1")

	result := notifier.NotifyReconnect(conn.Connection, protocol.ReconnectReasonICEDisconnected, 5000)
	if !result {
		t.Error("NotifyReconnect should return true")
	}

	messages := conn.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var notif protocol.Notification
	if err := json.Unmarshal(messages[0], &notif); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	if notif.Method != protocol.NotifyReconnect {
		t.Errorf("expected method %s, got %s", protocol.NotifyReconnect, notif.Method)
	}

	var params protocol.ReconnectParams
	if err := notif.UnmarshalParams(&params); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}

	if params.Reason != protocol.ReconnectReasonICEDisconnected {
		t.Errorf("expected reason ice_disconnected, got %s", params.Reason)
	}

	if params.RetryAfterMs != 5000 {
		t.Errorf("expected retryAfterMs 5000, got %d", params.RetryAfterMs)
	}
}

func TestNotifier_BroadcastError(t *testing.T) {
	notifier := NewNotifier()

	conn1 := newTestConnection("conn-1")
	conn2 := newTestConnection("conn-2")

	notifier.AddToRoom("room-1", conn1.Connection)
	notifier.AddToRoom("room-1", conn2.Connection)

	sent := notifier.BroadcastError("room-1", 1000, "Server maintenance", true, nil)

	if sent != 2 {
		t.Errorf("expected 2 messages sent, got %d", sent)
	}

	// Verify both connections received the error
	messages := conn1.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("conn1 expected 1 message, got %d", len(messages))
	}

	var notif protocol.Notification
	if err := json.Unmarshal(messages[0], &notif); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	if notif.Method != protocol.NotifyError {
		t.Errorf("expected method %s, got %s", protocol.NotifyError, notif.Method)
	}
}

func TestNotifier_BroadcastReconnect(t *testing.T) {
	notifier := NewNotifier()

	conn1 := newTestConnection("conn-1")
	conn2 := newTestConnection("conn-2")
	excludedConn := newTestConnection("conn-excluded")

	notifier.AddToRoom("room-1", conn1.Connection)
	notifier.AddToRoom("room-1", conn2.Connection)
	notifier.AddToRoom("room-1", excludedConn.Connection)

	sent := notifier.BroadcastReconnect("room-1", protocol.ReconnectReasonServerRestart, 10000, excludedConn.Connection)

	if sent != 2 {
		t.Errorf("expected 2 messages sent (excluding 1), got %d", sent)
	}

	// Verify conn1 received the notification
	messages := conn1.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("conn1 expected 1 message, got %d", len(messages))
	}

	var notif protocol.Notification
	if err := json.Unmarshal(messages[0], &notif); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	if notif.Method != protocol.NotifyReconnect {
		t.Errorf("expected method %s, got %s", protocol.NotifyReconnect, notif.Method)
	}

	// Verify excludedConn did NOT receive the notification
	excludedMessages := excludedConn.GetMessages()
	if len(excludedMessages) != 0 {
		t.Errorf("excludedConn should not receive notification, got %d messages", len(excludedMessages))
	}
}

func TestNotifier_NotifyTrackSubscribed(t *testing.T) {
	notifier := NewNotifier()

	conn := newTestConnection("conn-1")

	result := notifier.NotifyTrackSubscribed(conn.Connection, "sub-123", "publisher-1", "track-1", protocol.TrackKindVideo)
	if !result {
		t.Error("NotifyTrackSubscribed should return true")
	}

	messages := conn.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var notif protocol.Notification
	if err := json.Unmarshal(messages[0], &notif); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	if notif.Method != protocol.NotifyTrackSubscribed {
		t.Errorf("expected method %s, got %s", protocol.NotifyTrackSubscribed, notif.Method)
	}

	var params protocol.TrackSubscribedParams
	if err := notif.UnmarshalParams(&params); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}

	if params.SubscriptionID != "sub-123" {
		t.Errorf("expected subscriptionId sub-123, got %s", params.SubscriptionID)
	}

	if params.PublisherID != "publisher-1" {
		t.Errorf("expected publisherId publisher-1, got %s", params.PublisherID)
	}

	if params.TrackID != "track-1" {
		t.Errorf("expected trackId track-1, got %s", params.TrackID)
	}

	if params.Kind != protocol.TrackKindVideo {
		t.Errorf("expected kind video, got %s", params.Kind)
	}
}

func TestNotifier_NotifyTrackSubscribed_NilConnection(t *testing.T) {
	notifier := NewNotifier()

	result := notifier.NotifyTrackSubscribed(nil, "sub-123", "publisher-1", "track-1", protocol.TrackKindVideo)
	if result {
		t.Error("NotifyTrackSubscribed should return false for nil connection")
	}
}

func TestNotifier_NotifyTrackSubscriptionFailed(t *testing.T) {
	notifier := NewNotifier()

	conn := newTestConnection("conn-1")

	result := notifier.NotifyTrackSubscriptionFailed(conn.Connection, "publisher-1", "track-1", 1005, "Track not found")
	if !result {
		t.Error("NotifyTrackSubscriptionFailed should return true")
	}

	messages := conn.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var notif protocol.Notification
	if err := json.Unmarshal(messages[0], &notif); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	if notif.Method != protocol.NotifyTrackSubscriptionFail {
		t.Errorf("expected method %s, got %s", protocol.NotifyTrackSubscriptionFail, notif.Method)
	}

	var params protocol.TrackSubscriptionFailedParams
	if err := notif.UnmarshalParams(&params); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}

	if params.PublisherID != "publisher-1" {
		t.Errorf("expected publisherId publisher-1, got %s", params.PublisherID)
	}

	if params.TrackID != "track-1" {
		t.Errorf("expected trackId track-1, got %s", params.TrackID)
	}

	if params.ErrorCode != 1005 {
		t.Errorf("expected errorCode 1005, got %d", params.ErrorCode)
	}

	if params.ErrorMsg != "Track not found" {
		t.Errorf("expected errorMessage 'Track not found', got %s", params.ErrorMsg)
	}
}

func TestNotifier_NotifyConnectionQualityChanged(t *testing.T) {
	notifier := NewNotifier()

	conn := newTestConnection("conn-1")

	result := notifier.NotifyConnectionQualityChanged(conn.Connection, "participant-1", protocol.ConnectionQualityGood, 0.85)
	if !result {
		t.Error("NotifyConnectionQualityChanged should return true")
	}

	messages := conn.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var notif protocol.Notification
	if err := json.Unmarshal(messages[0], &notif); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	if notif.Method != protocol.NotifyConnectionQuality {
		t.Errorf("expected method %s, got %s", protocol.NotifyConnectionQuality, notif.Method)
	}

	var params protocol.ConnectionQualityChangedParams
	if err := notif.UnmarshalParams(&params); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}

	if params.ParticipantID != "participant-1" {
		t.Errorf("expected participantId participant-1, got %s", params.ParticipantID)
	}

	if params.Quality != protocol.ConnectionQualityGood {
		t.Errorf("expected quality good, got %s", params.Quality)
	}

	if params.Score != 0.85 {
		t.Errorf("expected score 0.85, got %f", params.Score)
	}
}

func TestNotifier_BroadcastConnectionQualityChanged(t *testing.T) {
	notifier := NewNotifier()

	conn1 := newTestConnection("conn-1")
	conn2 := newTestConnection("conn-2")

	notifier.AddToRoom("room-1", conn1.Connection)
	notifier.AddToRoom("room-1", conn2.Connection)

	sent := notifier.BroadcastConnectionQualityChanged("room-1", "participant-1", protocol.ConnectionQualityFair, 0.6, nil)

	if sent != 2 {
		t.Errorf("expected 2 messages sent, got %d", sent)
	}

	messages := conn1.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var notif protocol.Notification
	if err := json.Unmarshal(messages[0], &notif); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	if notif.Method != protocol.NotifyConnectionQuality {
		t.Errorf("expected method %s, got %s", protocol.NotifyConnectionQuality, notif.Method)
	}
}

func TestNotifier_NotifyServerStateChanged(t *testing.T) {
	notifier := NewNotifier()

	conn := newTestConnection("conn-1")

	result := notifier.NotifyServerStateChanged(conn.Connection, "room-1", protocol.ServerStateMaintenance, "Scheduled maintenance")
	if !result {
		t.Error("NotifyServerStateChanged should return true")
	}

	messages := conn.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var notif protocol.Notification
	if err := json.Unmarshal(messages[0], &notif); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	if notif.Method != protocol.NotifyServerState {
		t.Errorf("expected method %s, got %s", protocol.NotifyServerState, notif.Method)
	}

	var params protocol.ServerStateChangedParams
	if err := notif.UnmarshalParams(&params); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}

	if params.RoomID != "room-1" {
		t.Errorf("expected roomId room-1, got %s", params.RoomID)
	}

	if params.State != protocol.ServerStateMaintenance {
		t.Errorf("expected state maintenance, got %s", params.State)
	}

	if params.Message != "Scheduled maintenance" {
		t.Errorf("expected message 'Scheduled maintenance', got %s", params.Message)
	}
}

func TestNotifier_BroadcastServerStateChanged(t *testing.T) {
	notifier := NewNotifier()

	conn1 := newTestConnection("conn-1")
	conn2 := newTestConnection("conn-2")
	excludedConn := newTestConnection("conn-excluded")

	notifier.AddToRoom("room-1", conn1.Connection)
	notifier.AddToRoom("room-1", conn2.Connection)
	notifier.AddToRoom("room-1", excludedConn.Connection)

	sent := notifier.BroadcastServerStateChanged("room-1", protocol.ServerStateShuttingDown, "Server shutting down", excludedConn.Connection)

	if sent != 2 {
		t.Errorf("expected 2 messages sent (excluding 1), got %d", sent)
	}

	messages := conn1.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var notif protocol.Notification
	if err := json.Unmarshal(messages[0], &notif); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	if notif.Method != protocol.NotifyServerState {
		t.Errorf("expected method %s, got %s", protocol.NotifyServerState, notif.Method)
	}

	// Verify excludedConn did NOT receive the notification
	excludedMessages := excludedConn.GetMessages()
	if len(excludedMessages) != 0 {
		t.Errorf("excludedConn should not receive notification, got %d messages", len(excludedMessages))
	}
}
