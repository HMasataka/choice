package handshake

import (
	"encoding/json"
	"time"

	"github.com/pion/webrtc/v4"
)

// MessageType represents the type of signaling message
type MessageType string

const (
	MessageTypeRegisterRequest  MessageType = "register_request"
	MessageTypeRegisterResponse MessageType = "register_response"
	MessageTypeSDP              MessageType = "sdp"
	MessageTypeICECandidate     MessageType = "ice_candidate"
	MessageTypeDataChannel      MessageType = "data_channel"
	MessageTypeJoinRoom         MessageType = "join_room"
	MessageTypeLeaveRoom        MessageType = "leave_room"
	MessageTypeRoomMessage      MessageType = "room_message"
	MessageTypeClientList       MessageType = "client_list"
)

// Message represents a generic signaling message
type Message struct {
	ID        string          `json:"id"`
	Type      MessageType     `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// RegisterRequest represents a client registration request
type RegisterRequest struct{}

// RegisterResponse represents a registration response
type RegisterResponse struct {
	ClientID string `json:"client_id"`
}

// SDPMessage represents an SDP exchange message
type SDPMessage struct {
	SenderID           string                    `json:"client_id"`
	SessionDescription webrtc.SessionDescription `json:"session_description"`
}

// ICECandidateMessage represents an ICE candidate message
type ICECandidateMessage struct {
	SenderID  string                  `json:"client_id"`
	Candidate webrtc.ICECandidateInit `json:"candidate"`
}

// JoinRoomRequest represents a request to join a room
type JoinRoomRequest struct {
	ClientID   string `json:"client_id"`
	ClientName string `json:"client_name"`
	RoomID     string `json:"room_id"`
}

// JoinRoomResponse represents a response to join room request
type JoinRoomResponse struct {
	Status     string       `json:"status"` // "success" or "error"
	Message    string       `json:"message,omitempty"`
	RoomID     string       `json:"room_id,omitempty"`
	ClientID   string       `json:"client_id,omitempty"`
	ClientList []ClientInfo `json:"client_list,omitempty"`
}

// LeaveRoomRequest represents a request to leave a room
type LeaveRoomRequest struct {
	ClientID string `json:"client_id"`
	RoomID   string `json:"room_id"`
}

// LeaveRoomResponse represents a response to leave room request
type LeaveRoomResponse struct {
	Status   string `json:"status"` // "success" or "error"
	Message  string `json:"message,omitempty"`
	ClientID string `json:"client_id,omitempty"`
}

// RoomMessage represents a message sent within a room
type RoomMessage struct {
	Type      string `json:"type"`      // "broadcast", "private", "system"
	From      string `json:"from"`      // sender client ID
	FromName  string `json:"from_name"` // sender display name
	To        string `json:"to"`        // target client ID (for private messages)
	Content   string `json:"content"`   // message content
	Timestamp string `json:"timestamp"`
	RoomID    string `json:"room_id"`
}

// ClientListMessage represents current clients in a room
type ClientListMessage struct {
	RoomID  string       `json:"room_id"`
	Clients []ClientInfo `json:"clients"`
}

// ClientInfo represents basic client information
type ClientInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Connected string `json:"connected"`
	Active    bool   `json:"active"`
}

func NewSDPMessage(senderID string, sd webrtc.SessionDescription) (Message, error) {
	sdpMsg := SDPMessage{
		SenderID:           senderID,
		SessionDescription: sd,
	}

	data, err := json.Marshal(sdpMsg)
	if err != nil {
		return Message{}, err
	}

	return Message{
		Type:      MessageTypeSDP,
		Timestamp: time.Now(),
		Data:      data,
	}, nil
}

func NewICECandidateMessage(senderID string, candidate webrtc.ICECandidateInit) (Message, error) {
	iceMsg := ICECandidateMessage{
		SenderID:  senderID,
		Candidate: candidate,
	}

	data, err := json.Marshal(iceMsg)
	if err != nil {
		return Message{}, err
	}

	return Message{
		Type:      MessageTypeICECandidate,
		Timestamp: time.Now(),
		Data:      data,
	}, nil
}

func NewJoinRoomRequestMessage(clientID, clientName, roomID string) (Message, error) {
	joinMsg := JoinRoomRequest{
		ClientID:   clientID,
		ClientName: clientName,
		RoomID:     roomID,
	}

	data, err := json.Marshal(joinMsg)
	if err != nil {
		return Message{}, err
	}

	return Message{
		Type:      MessageTypeJoinRoom,
		Timestamp: time.Now(),
		Data:      data,
	}, nil
}

func NewJoinRoomResponseMessage(status, message, roomID, clientID string, clientList []ClientInfo) (Message, error) {
	respMsg := JoinRoomResponse{
		Status:     status,
		Message:    message,
		RoomID:     roomID,
		ClientID:   clientID,
		ClientList: clientList,
	}

	data, err := json.Marshal(respMsg)
	if err != nil {
		return Message{}, err
	}

	return Message{
		Type:      MessageTypeJoinRoom,
		Timestamp: time.Now(),
		Data:      data,
	}, nil
}

func NewRoomMessage(msgType, from, fromName, to, content, roomID string) (Message, error) {
	roomMsg := RoomMessage{
		Type:      msgType,
		From:      from,
		FromName:  fromName,
		To:        to,
		Content:   content,
		Timestamp: time.Now().Format(time.RFC3339),
		RoomID:    roomID,
	}

	data, err := json.Marshal(roomMsg)
	if err != nil {
		return Message{}, err
	}

	return Message{
		Type:      MessageTypeRoomMessage,
		Timestamp: time.Now(),
		Data:      data,
	}, nil
}

func NewClientListMessage(roomID string, clients []ClientInfo) (Message, error) {
	listMsg := ClientListMessage{
		RoomID:  roomID,
		Clients: clients,
	}

	data, err := json.Marshal(listMsg)
	if err != nil {
		return Message{}, err
	}

	return Message{
		Type:      MessageTypeClientList,
		Timestamp: time.Now(),
		Data:      data,
	}, nil
}

func NewLeaveRoomResponseMessage(status, message, clientID string) (Message, error) {
	respMsg := LeaveRoomResponse{
		Status:   status,
		Message:  message,
		ClientID: clientID,
	}

	data, err := json.Marshal(respMsg)
	if err != nil {
		return Message{}, err
	}

	return Message{
		Type:      MessageTypeLeaveRoom,
		Timestamp: time.Now(),
		Data:      data,
	}, nil
}
