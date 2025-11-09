package handler

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/HMasataka/choice/internal/room"
	"github.com/HMasataka/choice/payload/handshake"
)

// JoinRoomHandler handles join room requests
type JoinRoomHandler struct {
	roomManager *room.RoomManager
}

// NewJoinRoomHandler creates a new join room handler
func NewJoinRoomHandler(roomManager *room.RoomManager) *JoinRoomHandler {
	return &JoinRoomHandler{
		roomManager: roomManager,
	}
}

// CanHandle checks if this handler can handle the message type
func (h *JoinRoomHandler) CanHandle(messageType handshake.MessageType) bool {
	return messageType == handshake.MessageTypeJoinRoom
}

// Handle processes join room requests
func (h *JoinRoomHandler) Handle(ctx context.Context, msg *handshake.Message) (*handshake.Message, error) {
	var joinReq handshake.JoinRoomRequest
	if err := json.Unmarshal(msg.Data, &joinReq); err != nil {
		log.Printf("Failed to unmarshal join room request: %v", err)
		return h.createErrorResponse("Invalid request format"), nil
	}

	log.Printf("Processing join room request: client=%s, room=%s", joinReq.ClientID, joinReq.RoomID)

	// Validate request
	if joinReq.ClientID == "" || joinReq.RoomID == "" {
		return h.createErrorResponse("Client ID and Room ID are required"), nil
	}

	// Check if client already exists
	if _, exists := h.roomManager.GetClient(joinReq.ClientID); exists {
		log.Printf("Client %s is already connected", joinReq.ClientID)
		return h.createJoinResponse(joinReq.ClientID, joinReq.RoomID, "Already connected"), nil
	}

	// Get or create room
	room, err := h.roomManager.GetOrCreateRoom(joinReq.RoomID)
	if err != nil {
		log.Printf("Failed to get or create room %s: %v", joinReq.RoomID, err)
		return h.createErrorResponse("Failed to access room"), nil
	}

	// Check if room is full
	if room.IsFull() {
		log.Printf("Room %s is full", joinReq.RoomID)
		return h.createErrorResponse("Room is full"), nil
	}

	// Create success response with current client list
	return h.createJoinResponse(joinReq.ClientID, joinReq.RoomID, "Room information"), nil
}

// createJoinResponse creates a successful join room response
func (h *JoinRoomHandler) createJoinResponse(clientID, roomID, message string) *handshake.Message {
	room, exists := h.roomManager.GetRoom(roomID)
	var clientList []handshake.ClientInfo

	if exists {
		clients := room.GetClientInfoList()
		clientList = make([]handshake.ClientInfo, len(clients))
		for i, client := range clients {
			clientList[i] = handshake.ClientInfo{
				ID:        client.ID,
				Name:      client.Name,
				Connected: client.Connected.Format(time.RFC3339),
				Active:    client.Active,
			}
		}
	}

	response, err := handshake.NewJoinRoomResponseMessage("success", message, roomID, clientID, clientList)
	if err != nil {
		log.Printf("Failed to create join room response: %v", err)
		return h.createErrorResponse("Internal server error")
	}

	return &response
}

// createErrorResponse creates an error response
func (h *JoinRoomHandler) createErrorResponse(message string) *handshake.Message {
	response, err := handshake.NewJoinRoomResponseMessage("error", message, "", "", nil)
	if err != nil {
		log.Printf("Failed to create error response: %v", err)
		// Return a basic error message if we can't even create the error response
		basicResponse := &handshake.Message{
			Type:      handshake.MessageTypeJoinRoom,
			Timestamp: time.Now(),
			Data:      []byte(`{"status":"error","message":"Internal server error"}`),
		}
		return basicResponse
	}

	return &response
}

// NotifyClientJoined notifies other clients in the room that a new client joined
func (h *JoinRoomHandler) NotifyClientJoined(roomID, clientID, clientName string) error {
	room, exists := h.roomManager.GetRoom(roomID)
	if !exists {
		return nil // Room doesn't exist, nothing to notify
	}

	// Create system message
	systemMsg, err := handshake.NewRoomMessage("system", "server", "System", "",
		clientName+" joined the room", roomID)
	if err != nil {
		log.Printf("Failed to create system message: %v", err)
		return err
	}

	systemMsgBytes, err := json.Marshal(systemMsg)
	if err != nil {
		log.Printf("Failed to marshal system message: %v", err)
		return err
	}

	// Broadcast to other clients (exclude the newly joined client)
	if err := room.BroadcastToOthers(clientID, systemMsgBytes); err != nil {
		log.Printf("Failed to broadcast join notification: %v", err)
		return err
	}

	// Also send updated client list to all clients
	h.broadcastClientList(roomID)

	log.Printf("Notified room %s about client %s joining", roomID, clientID)
	return nil
}

// NotifyClientLeft notifies other clients in the room that a client left
func (h *JoinRoomHandler) NotifyClientLeft(roomID, clientID, clientName string) error {
	room, exists := h.roomManager.GetRoom(roomID)
	if !exists {
		return nil // Room doesn't exist, nothing to notify
	}

	// Create system message
	systemMsg, err := handshake.NewRoomMessage("system", "server", "System", "",
		clientName+" left the room", roomID)
	if err != nil {
		log.Printf("Failed to create system message: %v", err)
		return err
	}

	systemMsgBytes, err := json.Marshal(systemMsg)
	if err != nil {
		log.Printf("Failed to marshal system message: %v", err)
		return err
	}

	// Broadcast to remaining clients
	if err := room.BroadcastMessage(systemMsgBytes); err != nil {
		log.Printf("Failed to broadcast leave notification: %v", err)
		return err
	}

	// Send updated client list to all remaining clients
	h.broadcastClientList(roomID)

	log.Printf("Notified room %s about client %s leaving", roomID, clientID)
	return nil
}

// broadcastClientList sends the current client list to all clients in the room
func (h *JoinRoomHandler) broadcastClientList(roomID string) {
	room, exists := h.roomManager.GetRoom(roomID)
	if !exists {
		return
	}

	clients := room.GetClientInfoList()
	clientList := make([]handshake.ClientInfo, len(clients))
	for i, client := range clients {
		clientList[i] = handshake.ClientInfo{
			ID:        client.ID,
			Name:      client.Name,
			Connected: client.Connected.Format(time.RFC3339),
			Active:    client.Active,
		}
	}

	listMsg, err := handshake.NewClientListMessage(roomID, clientList)
	if err != nil {
		log.Printf("Failed to create client list message: %v", err)
		return
	}

	listMsgBytes, err := json.Marshal(listMsg)
	if err != nil {
		log.Printf("Failed to marshal client list message: %v", err)
		return
	}

	if err := room.BroadcastMessage(listMsgBytes); err != nil {
		log.Printf("Failed to broadcast client list: %v", err)
	}
}