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

// PublishResponse contains the response data for a successful publish.
type PublishResponse struct {
	TrackID string
	Mid     string
}

// SubscribeResponse contains the response data for a successful subscribe.
type SubscribeResponse struct {
	SubscriptionID string
	TrackID        string
	PublisherID    string
}

// MediaService defines the interface for media operations.
// This interface is implemented by the media package and injected into handlers.
type MediaService interface {
	// Publish handles a participant publishing a track.
	Publish(ctx context.Context, participantID string, kind protocol.TrackKind, simulcast bool, metadata map[string]interface{}, label string) (*PublishResponse, error)
	// Unpublish handles a participant unpublishing a track.
	Unpublish(ctx context.Context, participantID string, trackID string) error
	// Subscribe handles a participant subscribing to a track.
	Subscribe(ctx context.Context, participantID string, publisherID string, trackID string, preferredLayer protocol.SimulcastLayer) (*SubscribeResponse, error)
	// Unsubscribe handles a participant unsubscribing from a track.
	Unsubscribe(ctx context.Context, participantID string, subscriptionID string) error
	// SetPreferredLayer sets the preferred simulcast layer for a track.
	SetPreferredLayer(ctx context.Context, participantID string, trackID string, layer protocol.SimulcastLayer) error
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
	dispatcher   *Dispatcher
	roomService  RoomService
	rtcService   WebRTCService
	mediaService MediaService
	iceServers   []protocol.IceServer

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
func NewHandlers(dispatcher *Dispatcher, roomService RoomService, rtcService WebRTCService, mediaService MediaService, cfg HandlersConfig) *Handlers {
	h := &Handlers{
		dispatcher:             dispatcher,
		roomService:            roomService,
		rtcService:             rtcService,
		mediaService:           mediaService,
		iceServers:             cfg.IceServers,
		participantConnections: make(map[string]string),
	}

	// Register method handlers
	h.registerMethods()

	return h
}

// registerMethods registers all JSON-RPC method handlers.
func (h *Handlers) registerMethods() {
	// Basic methods
	h.dispatcher.RegisterMethod(protocol.MethodJoin, h.handleJoin)
	h.dispatcher.RegisterMethod(protocol.MethodLeave, h.handleLeave)
	h.dispatcher.RegisterMethod(protocol.MethodOffer, h.handleOffer)
	h.dispatcher.RegisterMethod(protocol.MethodAnswer, h.handleAnswer)
	h.dispatcher.RegisterMethod(protocol.MethodCandidate, h.handleCandidate)

	// Media methods
	h.dispatcher.RegisterMethod(protocol.MethodPublish, h.handlePublish)
	h.dispatcher.RegisterMethod(protocol.MethodUnpublish, h.handleUnpublish)
	h.dispatcher.RegisterMethod(protocol.MethodSubscribe, h.handleSubscribe)
	h.dispatcher.RegisterMethod(protocol.MethodUnsubscribe, h.handleUnsubscribe)
	h.dispatcher.RegisterMethod(protocol.MethodSetPreferredLayer, h.handleSetPreferredLayer)
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

// handlePublish handles the "publish" method.
func (h *Handlers) handlePublish(ctx context.Context, conn *Connection, req *protocol.Request) (interface{}, *protocol.Error) {
	// Parse parameters
	var params protocol.PublishParams
	if err := req.UnmarshalParams(&params); err != nil {
		return nil, protocol.NewInvalidParamsError("failed to parse publish params")
	}

	// Validate parameters
	if validErr := protocol.ValidatePublishParams(&params); validErr != nil {
		return nil, validErr
	}

	// Get participant ID from connection
	participantID := h.getParticipantID(conn)
	if participantID == "" {
		return nil, protocol.NewNotInRoomError()
	}

	// Check if media service is available
	if h.mediaService == nil {
		// Return stub response
		return &protocol.PublishResult{
			TrackID: "stub-track-" + uuid.New().String(),
			Mid:     "0",
		}, nil
	}

	// Call media service
	resp, err := h.mediaService.Publish(ctx, participantID, params.Kind, params.Simulcast, params.Metadata, params.Label)
	if err != nil {
		return nil, h.convertServiceError(err)
	}

	// Guard against nil response (should not happen per interface contract)
	if resp == nil {
		return nil, protocol.NewInternalError("publish returned nil response")
	}

	return &protocol.PublishResult{
		TrackID: resp.TrackID,
		Mid:     resp.Mid,
	}, nil
}

// handleUnpublish handles the "unpublish" method.
func (h *Handlers) handleUnpublish(ctx context.Context, conn *Connection, req *protocol.Request) (interface{}, *protocol.Error) {
	// Parse parameters
	var params protocol.UnpublishParams
	if err := req.UnmarshalParams(&params); err != nil {
		return nil, protocol.NewInvalidParamsError("failed to parse unpublish params")
	}

	// Validate parameters
	if validErr := protocol.ValidateUnpublishParams(&params); validErr != nil {
		return nil, validErr
	}

	// Get participant ID from connection
	participantID := h.getParticipantID(conn)
	if participantID == "" {
		return nil, protocol.NewNotInRoomError()
	}

	// Check if media service is available
	if h.mediaService == nil {
		// Return stub response
		return &protocol.UnpublishResult{}, nil
	}

	// Call media service
	if err := h.mediaService.Unpublish(ctx, participantID, params.TrackID); err != nil {
		return nil, h.convertServiceError(err)
	}

	return &protocol.UnpublishResult{}, nil
}

// handleSubscribe handles the "subscribe" method.
func (h *Handlers) handleSubscribe(ctx context.Context, conn *Connection, req *protocol.Request) (interface{}, *protocol.Error) {
	// Parse parameters
	var params protocol.SubscribeParams
	if err := req.UnmarshalParams(&params); err != nil {
		return nil, protocol.NewInvalidParamsError("failed to parse subscribe params")
	}

	// Validate parameters
	if validErr := protocol.ValidateSubscribeParams(&params); validErr != nil {
		return nil, validErr
	}

	// Apply default preferredLayer per schema (default: "h")
	preferredLayer := params.PreferredLayer
	if preferredLayer == "" {
		preferredLayer = protocol.SimulcastLayerHigh
	}

	// Get participant ID from connection
	participantID := h.getParticipantID(conn)
	if participantID == "" {
		return nil, protocol.NewNotInRoomError()
	}

	// Check if media service is available
	if h.mediaService == nil {
		// Return stub response
		return &protocol.SubscribeResult{
			SubscriptionID: "stub-sub-" + uuid.New().String(),
			TrackID:        params.TrackID,
			PublisherID:    params.PublisherID,
		}, nil
	}

	// Call media service
	resp, err := h.mediaService.Subscribe(ctx, participantID, params.PublisherID, params.TrackID, preferredLayer)
	if err != nil {
		return nil, h.convertServiceError(err)
	}

	// Guard against nil response (should not happen per interface contract)
	if resp == nil {
		return nil, protocol.NewInternalError("subscribe returned nil response")
	}

	return &protocol.SubscribeResult{
		SubscriptionID: resp.SubscriptionID,
		TrackID:        resp.TrackID,
		PublisherID:    resp.PublisherID,
	}, nil
}

// handleUnsubscribe handles the "unsubscribe" method.
func (h *Handlers) handleUnsubscribe(ctx context.Context, conn *Connection, req *protocol.Request) (interface{}, *protocol.Error) {
	// Parse parameters
	var params protocol.UnsubscribeParams
	if err := req.UnmarshalParams(&params); err != nil {
		return nil, protocol.NewInvalidParamsError("failed to parse unsubscribe params")
	}

	// Validate parameters
	if validErr := protocol.ValidateUnsubscribeParams(&params); validErr != nil {
		return nil, validErr
	}

	// Get participant ID from connection
	participantID := h.getParticipantID(conn)
	if participantID == "" {
		return nil, protocol.NewNotInRoomError()
	}

	// Check if media service is available
	if h.mediaService == nil {
		// Return stub response
		return &protocol.UnsubscribeResult{}, nil
	}

	// Call media service
	if err := h.mediaService.Unsubscribe(ctx, participantID, params.SubscriptionID); err != nil {
		return nil, h.convertServiceError(err)
	}

	return &protocol.UnsubscribeResult{}, nil
}

// handleSetPreferredLayer handles the "setPreferredLayer" method.
func (h *Handlers) handleSetPreferredLayer(ctx context.Context, conn *Connection, req *protocol.Request) (interface{}, *protocol.Error) {
	// Parse parameters
	var params protocol.SetPreferredLayerParams
	if err := req.UnmarshalParams(&params); err != nil {
		return nil, protocol.NewInvalidParamsError("failed to parse setPreferredLayer params")
	}

	// Validate parameters
	if validErr := protocol.ValidateSetPreferredLayerParams(&params); validErr != nil {
		return nil, validErr
	}

	// Get participant ID from connection
	participantID := h.getParticipantID(conn)
	if participantID == "" {
		return nil, protocol.NewNotInRoomError()
	}

	// Check if media service is available
	if h.mediaService == nil {
		// Return stub response
		return &protocol.SetPreferredLayerResult{}, nil
	}

	// Call media service
	if err := h.mediaService.SetPreferredLayer(ctx, participantID, params.TrackID, params.Layer); err != nil {
		return nil, h.convertServiceError(err)
	}

	return &protocol.SetPreferredLayerResult{}, nil
}
