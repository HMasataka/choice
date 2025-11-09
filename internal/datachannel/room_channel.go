package datachannel

import (
	"encoding/json"
	"log"
	"time"

	"github.com/HMasataka/choice/internal/room"
	"github.com/HMasataka/choice/payload/handshake"
)

// RoomChannelManager manages DataChannel messaging for rooms
type RoomChannelManager struct {
	roomManager *room.RoomManager
}

// NewRoomChannelManager creates a new room channel manager
func NewRoomChannelManager(roomManager *room.RoomManager) *RoomChannelManager {
	return &RoomChannelManager{
		roomManager: roomManager,
	}
}

// SetupRoomDataChannel sets up DataChannel for room messaging
func (rcm *RoomChannelManager) SetupRoomDataChannel(client *room.Client) {
	dc := client.GetDataChannel()
	if dc == nil {
		log.Printf("No DataChannel available for client %s", client.ID)
		return
	}

	// Set up message handler
	dc.OnMessage(func(data []byte) {
		rcm.handleDataChannelMessage(client, data)
	})

	// Send welcome message
	welcomeMsg := map[string]interface{}{
		"type":      "system",
		"from":      "server",
		"from_name": "System",
		"content":   "Welcome to room " + client.RoomID,
		"timestamp": time.Now().Format(time.RFC3339),
		"room_id":   client.RoomID,
	}

	welcomeData, err := json.Marshal(welcomeMsg)
	if err != nil {
		log.Printf("Failed to marshal welcome message: %v", err)
		return
	}

	if err := client.SendDataChannelMessage(welcomeData); err != nil {
		log.Printf("Failed to send welcome message to client %s: %v", client.ID, err)
	}

	log.Printf("Set up room DataChannel for client %s in room %s", client.ID, client.RoomID)
}

// handleDataChannelMessage processes incoming DataChannel messages
func (rcm *RoomChannelManager) handleDataChannelMessage(sender *room.Client, data []byte) {
	log.Printf("Received DataChannel message from client %s: %s", sender.ID, string(data))

	// Try to parse as room message
	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("Failed to parse DataChannel message from client %s: %v", sender.ID, err)
		rcm.sendErrorMessage(sender, "Invalid message format")
		return
	}

	// Extract message type
	msgType, ok := msg["type"].(string)
	if !ok {
		log.Printf("Missing or invalid message type from client %s", sender.ID)
		rcm.sendErrorMessage(sender, "Missing message type")
		return
	}

	// Handle different message types
	switch msgType {
	case "broadcast":
		rcm.handleBroadcastMessage(sender, msg)
	case "private":
		rcm.handlePrivateMessage(sender, msg)
	case "ping":
		rcm.handlePingMessage(sender)
	default:
		log.Printf("Unknown message type '%s' from client %s", msgType, sender.ID)
		rcm.sendErrorMessage(sender, "Unknown message type: "+msgType)
	}
}

// handleBroadcastMessage handles broadcast messages to all clients in the room
func (rcm *RoomChannelManager) handleBroadcastMessage(sender *room.Client, msg map[string]interface{}) {
	content, ok := msg["content"].(string)
	if !ok {
		rcm.sendErrorMessage(sender, "Missing or invalid content")
		return
	}

	// Get the room
	room, exists := rcm.roomManager.GetRoom(sender.RoomID)
	if !exists {
		rcm.sendErrorMessage(sender, "Room not found")
		return
	}

	// Create room message
	roomMsg := map[string]interface{}{
		"type":      "broadcast",
		"from":      sender.ID,
		"from_name": sender.Name,
		"content":   content,
		"timestamp": time.Now().Format(time.RFC3339),
		"room_id":   sender.RoomID,
	}

	roomMsgData, err := json.Marshal(roomMsg)
	if err != nil {
		log.Printf("Failed to marshal room message: %v", err)
		rcm.sendErrorMessage(sender, "Failed to process message")
		return
	}

	// Broadcast to other clients in the room (exclude sender)
	if err := room.BroadcastDataChannelToOthers(sender.ID, roomMsgData); err != nil {
		log.Printf("Failed to broadcast message in room %s: %v", sender.RoomID, err)
		rcm.sendErrorMessage(sender, "Failed to broadcast message")
		return
	}

	log.Printf("Broadcasted message from client %s to room %s", sender.ID, sender.RoomID)
}

// handlePrivateMessage handles private messages to specific clients
func (rcm *RoomChannelManager) handlePrivateMessage(sender *room.Client, msg map[string]interface{}) {
	content, ok := msg["content"].(string)
	if !ok {
		rcm.sendErrorMessage(sender, "Missing or invalid content")
		return
	}

	targetID, ok := msg["to"].(string)
	if !ok {
		rcm.sendErrorMessage(sender, "Missing or invalid target client ID")
		return
	}

	// Check if target client exists and is in the same room
	targetClient, exists := rcm.roomManager.GetClient(targetID)
	if !exists {
		rcm.sendErrorMessage(sender, "Target client not found")
		return
	}

	if targetClient.RoomID != sender.RoomID {
		rcm.sendErrorMessage(sender, "Target client is not in the same room")
		return
	}

	// Create private message
	privateMsg := map[string]interface{}{
		"type":      "private",
		"from":      sender.ID,
		"from_name": sender.Name,
		"to":        targetID,
		"content":   content,
		"timestamp": time.Now().Format(time.RFC3339),
		"room_id":   sender.RoomID,
	}

	privateMsgData, err := json.Marshal(privateMsg)
	if err != nil {
		log.Printf("Failed to marshal private message: %v", err)
		rcm.sendErrorMessage(sender, "Failed to process message")
		return
	}

	// Send to target client
	if err := targetClient.SendDataChannelMessage(privateMsgData); err != nil {
		log.Printf("Failed to send private message to client %s: %v", targetID, err)
		rcm.sendErrorMessage(sender, "Failed to send private message")
		return
	}

	log.Printf("Sent private message from client %s to client %s", sender.ID, targetID)
}

// handlePingMessage handles ping messages (keep-alive)
func (rcm *RoomChannelManager) handlePingMessage(sender *room.Client) {
	sender.UpdateLastPing()

	// Send pong response
	pongMsg := map[string]interface{}{
		"type":      "pong",
		"timestamp": time.Now().Format(time.RFC3339),
	}

	pongData, err := json.Marshal(pongMsg)
	if err != nil {
		log.Printf("Failed to marshal pong message: %v", err)
		return
	}

	if err := sender.SendDataChannelMessage(pongData); err != nil {
		log.Printf("Failed to send pong to client %s: %v", sender.ID, err)
	}
}

// sendErrorMessage sends an error message to a client
func (rcm *RoomChannelManager) sendErrorMessage(client *room.Client, errorMsg string) {
	errMsg := map[string]interface{}{
		"type":      "error",
		"message":   errorMsg,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	errData, err := json.Marshal(errMsg)
	if err != nil {
		log.Printf("Failed to marshal error message: %v", err)
		return
	}

	if err := client.SendDataChannelMessage(errData); err != nil {
		log.Printf("Failed to send error message to client %s: %v", client.ID, err)
	}
}

// BroadcastSystemMessage sends a system message to all clients in a room
func (rcm *RoomChannelManager) BroadcastSystemMessage(roomID, content string) error {
	roomObj, exists := rcm.roomManager.GetRoom(roomID)
	if !exists {
		return room.ErrRoomNotFound
	}

	systemMsg := map[string]interface{}{
		"type":      "system",
		"from":      "server",
		"from_name": "System",
		"content":   content,
		"timestamp": time.Now().Format(time.RFC3339),
		"room_id":   roomID,
	}

	systemMsgData, err := json.Marshal(systemMsg)
	if err != nil {
		return err
	}

	return roomObj.BroadcastDataChannelMessage(systemMsgData)
}

// SendClientListUpdate sends updated client list to all clients in a room
func (rcm *RoomChannelManager) SendClientListUpdate(roomID string) error {
	roomObj, exists := rcm.roomManager.GetRoom(roomID)
	if !exists {
		return room.ErrRoomNotFound
	}

	clients := roomObj.GetClientInfoList()
	clientList := make([]handshake.ClientInfo, len(clients))
	for i, client := range clients {
		clientList[i] = handshake.ClientInfo{
			ID:        client.ID,
			Name:      client.Name,
			Connected: client.Connected.Format(time.RFC3339),
			Active:    client.Active,
		}
	}

	listMsg := map[string]interface{}{
		"type":     "client_list",
		"room_id":  roomID,
		"clients":  clientList,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	listMsgData, err := json.Marshal(listMsg)
	if err != nil {
		return err
	}

	return roomObj.BroadcastDataChannelMessage(listMsgData)
}