package signaling

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/HMasataka/choice/internal/signaling/protocol"
)

// RoomService defines the interface for room operations.
// This interface is implemented by the room package and injected into handlers.
type RoomService interface {
	// Join handles a participant joining a room.
	Join(ctx context.Context, token string, sessionID string, metadata map[string]interface{}) (*JoinResponse, error)
	// Leave handles a participant leaving a room.
	Leave(ctx context.Context, participantID string) error
}

// WebRTCService defines the interface for WebRTC operations.
// This interface is implemented by the webrtc package and injected into handlers.
type WebRTCService interface {
	// HandleOffer processes an SDP offer from a client.
	HandleOffer(ctx context.Context, participantID string, sdp string) (answerSDP string, err error)
	// HandleAnswer processes an SDP answer from a client.
	HandleAnswer(ctx context.Context, participantID string, sdp string) error
	// HandleCandidate processes an ICE candidate from a client.
	HandleCandidate(ctx context.Context, participantID string, candidate string, sdpMid string, sdpMLineIndex *int) error
}

// JoinResponse contains the response data for a successful join.
type JoinResponse struct {
	SessionID     string                    `json:"sessionId"`
	RoomID        string                    `json:"roomId"`
	ParticipantID string                    `json:"participantId"`
	Participants  []protocol.ParticipantInfo `json:"participants"`
	IceServers    []protocol.IceServer       `json:"iceServers"`
}

// Handlers contains all the method handlers for the signaling server.
type Handlers struct {
	dispatcher  *Dispatcher
	roomService RoomService
	rtcService  WebRTCService
	iceServers  []protocol.IceServer

	// mu protects participantConnections map
	mu sync.RWMutex
	// participantConnections maps connection IDs to participant IDs
	// This is used to track which participant is associated with which connection
	participantConnections map[string]string
}

// HandlersConfig contains configuration for handlers.
type HandlersConfig struct {
	IceServers []protocol.IceServer
}

// DefaultHandlersConfig returns the default handlers configuration.
func DefaultHandlersConfig() HandlersConfig {
	return HandlersConfig{
		IceServers: []protocol.IceServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}
}

// NewHandlers creates a new Handlers instance and registers all method handlers.
func NewHandlers(dispatcher *Dispatcher, roomService RoomService, rtcService WebRTCService, cfg HandlersConfig) *Handlers {
	h := &Handlers{
		dispatcher:             dispatcher,
		roomService:            roomService,
		rtcService:             rtcService,
		iceServers:             cfg.IceServers,
		participantConnections: make(map[string]string),
	}

	// Register method handlers
	h.registerMethods()

	return h
}

// registerMethods registers all JSON-RPC method handlers.
func (h *Handlers) registerMethods() {
	h.dispatcher.RegisterMethod(protocol.MethodJoin, h.handleJoin)
	h.dispatcher.RegisterMethod(protocol.MethodLeave, h.handleLeave)
	h.dispatcher.RegisterMethod(protocol.MethodOffer, h.handleOffer)
	h.dispatcher.RegisterMethod(protocol.MethodAnswer, h.handleAnswer)
	h.dispatcher.RegisterMethod(protocol.MethodCandidate, h.handleCandidate)
}

// handleJoin handles the "join" method.
func (h *Handlers) handleJoin(ctx context.Context, conn *Connection, req *protocol.Request) (interface{}, *protocol.Error) {
	// Parse parameters
	var params protocol.JoinParams
	if err := req.UnmarshalParams(&params); err != nil {
		return nil, protocol.NewInvalidParamsError("failed to parse join params")
	}

	// Validate parameters
	if validErr := protocol.ValidateJoinParams(&params); validErr != nil {
		return nil, validErr
	}

	// Check if room service is available
	if h.roomService == nil {
		// Return a stub response for now (room service not yet implemented)
		return h.stubJoinResponse(conn, &params), nil
	}

	// Call room service
	resp, err := h.roomService.Join(ctx, params.Token, params.SessionID, params.Metadata)
	if err != nil {
		return nil, h.convertServiceError(err)
	}

	// Track participant connection
	if conn != nil {
		h.mu.Lock()
		h.participantConnections[conn.ID()] = resp.ParticipantID
		h.mu.Unlock()
		conn.SetData("participant_id", resp.ParticipantID)
		conn.SetData("room_id", resp.RoomID)
	}

	// Build response with ICE servers
	// Prefer room-specific ICE servers if provided, otherwise use default
	iceServers := h.iceServers
	if len(resp.IceServers) > 0 {
		iceServers = resp.IceServers
	}

	result := &protocol.JoinResult{
		SessionID:     resp.SessionID,
		RoomID:        resp.RoomID,
		ParticipantID: resp.ParticipantID,
		Participants:  resp.Participants,
		IceServers:    iceServers,
	}

	return result, nil
}

// stubJoinResponse returns a stub response when room service is not available.
// This is useful for testing the signaling layer in isolation.
func (h *Handlers) stubJoinResponse(conn *Connection, params *protocol.JoinParams) *protocol.JoinResult {
	// Generate unique IDs for stub mode to avoid collisions
	participantID := "stub-" + uuid.New().String()
	sessionID := params.SessionID
	if sessionID == "" {
		sessionID = "stub-" + uuid.New().String()
	}
	roomID := "stub-room-" + uuid.New().String()

	if conn != nil {
		h.mu.Lock()
		h.participantConnections[conn.ID()] = participantID
		h.mu.Unlock()
		conn.SetData("participant_id", participantID)
		conn.SetData("room_id", roomID)
	}

	return &protocol.JoinResult{
		SessionID:     sessionID,
		RoomID:        roomID,
		ParticipantID: participantID,
		Participants:  []protocol.ParticipantInfo{},
		IceServers:    h.iceServers,
	}
}

// handleLeave handles the "leave" method.
func (h *Handlers) handleLeave(ctx context.Context, conn *Connection, req *protocol.Request) (interface{}, *protocol.Error) {
	// Get participant ID from connection
	participantID := h.getParticipantID(conn)
	if participantID == "" {
		return nil, protocol.NewNotInRoomError()
	}

	// Check if room service is available
	if h.roomService == nil {
		// Return stub response
		h.cleanupConnection(conn)
		return &protocol.LeaveResult{}, nil
	}

	// Call room service
	if err := h.roomService.Leave(ctx, participantID); err != nil {
		return nil, h.convertServiceError(err)
	}

	// Clean up connection data
	h.cleanupConnection(conn)

	return &protocol.LeaveResult{}, nil
}

// handleOffer handles the "offer" method.
func (h *Handlers) handleOffer(ctx context.Context, conn *Connection, req *protocol.Request) (interface{}, *protocol.Error) {
	// Parse parameters
	var params protocol.OfferParams
	if err := req.UnmarshalParams(&params); err != nil {
		return nil, protocol.NewInvalidParamsError("failed to parse offer params")
	}

	// Validate parameters
	if validErr := protocol.ValidateOfferParams(&params); validErr != nil {
		return nil, validErr
	}

	// Get participant ID from connection
	participantID := h.getParticipantID(conn)
	if participantID == "" {
		return nil, protocol.NewNotInRoomError()
	}

	// Check if WebRTC service is available
	if h.rtcService == nil {
		// Return stub response
		return &protocol.OfferResult{
			SDP: "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n",
		}, nil
	}

	// Call WebRTC service
	answerSDP, err := h.rtcService.HandleOffer(ctx, participantID, params.SDP)
	if err != nil {
		return nil, h.convertServiceError(err)
	}

	return &protocol.OfferResult{SDP: answerSDP}, nil
}

// handleAnswer handles the "answer" method.
func (h *Handlers) handleAnswer(ctx context.Context, conn *Connection, req *protocol.Request) (interface{}, *protocol.Error) {
	// Parse parameters
	var params protocol.AnswerParams
	if err := req.UnmarshalParams(&params); err != nil {
		return nil, protocol.NewInvalidParamsError("failed to parse answer params")
	}

	// Validate parameters
	if validErr := protocol.ValidateAnswerParams(&params); validErr != nil {
		return nil, validErr
	}

	// Get participant ID from connection
	participantID := h.getParticipantID(conn)
	if participantID == "" {
		return nil, protocol.NewNotInRoomError()
	}

	// Check if WebRTC service is available
	if h.rtcService == nil {
		// Return stub response
		return &protocol.AnswerResult{}, nil
	}

	// Call WebRTC service
	if err := h.rtcService.HandleAnswer(ctx, participantID, params.SDP); err != nil {
		return nil, h.convertServiceError(err)
	}

	return &protocol.AnswerResult{}, nil
}

// handleCandidate handles the "candidate" method.
func (h *Handlers) handleCandidate(ctx context.Context, conn *Connection, req *protocol.Request) (interface{}, *protocol.Error) {
	// Parse parameters
	var params protocol.CandidateParams
	if err := req.UnmarshalParams(&params); err != nil {
		return nil, protocol.NewInvalidParamsError("failed to parse candidate params")
	}

	// Validate parameters
	if validErr := protocol.ValidateCandidateParams(&params); validErr != nil {
		return nil, validErr
	}

	// Get participant ID from connection
	participantID := h.getParticipantID(conn)
	if participantID == "" {
		return nil, protocol.NewNotInRoomError()
	}

	// Check if WebRTC service is available
	if h.rtcService == nil {
		// Return stub response
		return &protocol.CandidateResult{}, nil
	}

	// Call WebRTC service
	if err := h.rtcService.HandleCandidate(ctx, participantID, params.Candidate, params.SDPMid, params.SDPMLineIndex); err != nil {
		return nil, h.convertServiceError(err)
	}

	return &protocol.CandidateResult{}, nil
}

// getParticipantID retrieves the participant ID from the connection.
func (h *Handlers) getParticipantID(conn *Connection) string {
	if conn == nil {
		return ""
	}

	data, ok := conn.GetData("participant_id")
	if !ok {
		return ""
	}

	participantID, ok := data.(string)
	if !ok {
		return ""
	}

	return participantID
}

// cleanupConnection removes participant data from the connection.
func (h *Handlers) cleanupConnection(conn *Connection) {
	if conn == nil {
		return
	}

	h.mu.Lock()
	delete(h.participantConnections, conn.ID())
	h.mu.Unlock()
	conn.DeleteData("participant_id")
	conn.DeleteData("room_id")
}

// convertServiceError converts a service error to a protocol error.
func (h *Handlers) convertServiceError(err error) *protocol.Error {
	// Check for specific error types and convert accordingly
	// For now, return a generic internal error
	// TODO: Add specific error type handling when room/webrtc services are implemented
	return protocol.NewInternalError(err.Error())
}

// OnConnectionClosed should be called when a connection is closed.
// This cleans up the participant's state.
func (h *Handlers) OnConnectionClosed(conn *Connection) {
	if conn == nil {
		return
	}

	participantID := h.getParticipantID(conn)
	if participantID == "" {
		return
	}

	// Leave room if in one
	if h.roomService != nil {
		// Use background context since the connection is closing
		_ = h.roomService.Leave(context.Background(), participantID)
	}

	// Clean up connection data
	h.cleanupConnection(conn)
}
