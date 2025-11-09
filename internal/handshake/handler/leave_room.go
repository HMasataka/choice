package handler

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/HMasataka/choice/internal/room"
	"github.com/HMasataka/choice/payload/handshake"
)

// LeaveRoomHandler handles leave room requests
type LeaveRoomHandler struct {
	roomManager *room.RoomManager
}

// NewLeaveRoomHandler creates a new leave room handler
func NewLeaveRoomHandler(roomManager *room.RoomManager) *LeaveRoomHandler {
	return &LeaveRoomHandler{
		roomManager: roomManager,
	}
}

// CanHandle checks if this handler can handle the message type
func (h *LeaveRoomHandler) CanHandle(messageType handshake.MessageType) bool {
	return messageType == handshake.MessageTypeLeaveRoom
}

// Handle processes leave room requests
func (h *LeaveRoomHandler) Handle(ctx context.Context, msg *handshake.Message) (*handshake.Message, error) {
	var leaveReq handshake.LeaveRoomRequest
	if err := json.Unmarshal(msg.Data, &leaveReq); err != nil {
		log.Printf("Failed to unmarshal leave room request: %v", err)
		return h.createErrorResponse("Invalid request format"), nil
	}

	log.Printf("Processing leave room request: client=%s, room=%s", leaveReq.ClientID, leaveReq.RoomID)

	// Validate request
	if leaveReq.ClientID == "" {
		return h.createErrorResponse("Client ID is required"), nil
	}

	// Check if client exists
	client, exists := h.roomManager.GetClient(leaveReq.ClientID)
	if !exists {
		log.Printf("Client %s not found", leaveReq.ClientID)
		return h.createErrorResponse("Client not found"), nil
	}

	// Get the client's current room (use the client's room ID, not the request room ID)
	roomID := client.RoomID
	if roomID == "" {
		log.Printf("Client %s is not in any room", leaveReq.ClientID)
		return h.createErrorResponse("Client is not in any room"), nil
	}

	// Remove client from the room (this is handled by the room manager)
	if err := h.roomManager.RemoveClient(leaveReq.ClientID); err != nil {
		log.Printf("Failed to remove client %s from room: %v", leaveReq.ClientID, err)
		return h.createErrorResponse("Failed to leave room"), nil
	}

	log.Printf("Client %s successfully left room %s", leaveReq.ClientID, roomID)

	// Create success response
	return h.createLeaveResponse(leaveReq.ClientID, "Successfully left room"), nil
}

// createLeaveResponse creates a successful leave room response
func (h *LeaveRoomHandler) createLeaveResponse(clientID, message string) *handshake.Message {
	response, err := handshake.NewLeaveRoomResponseMessage("success", message, clientID)
	if err != nil {
		log.Printf("Failed to create leave room response: %v", err)
		return h.createErrorResponse("Internal server error")
	}

	return &response
}

// createErrorResponse creates an error response
func (h *LeaveRoomHandler) createErrorResponse(message string) *handshake.Message {
	response, err := handshake.NewLeaveRoomResponseMessage("error", message, "")
	if err != nil {
		log.Printf("Failed to create error response: %v", err)
		// Return a basic error message if we can't even create the error response
		basicResponse := &handshake.Message{
			Type:      handshake.MessageTypeLeaveRoom,
			Timestamp: time.Now(),
			Data:      []byte(`{"status":"error","message":"Internal server error"}`),
		}
		return basicResponse
	}

	return &response
}

