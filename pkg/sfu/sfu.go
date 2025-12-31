package sfu

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

// Errors
var (
	ErrSessionNotFound = errors.New("session not found")
	ErrPeerNotFound    = errors.New("peer not found")
)

// Config holds the SFU configuration.
type Config struct {
	ICEServers []webrtc.ICEServer
}

// SFU is the main Selective Forwarding Unit that manages sessions and WebRTC connections.
type SFU struct {
	config   Config
	api      *webrtc.API
	sessions map[string]*Session
	mu       sync.RWMutex
	upgrader websocket.Upgrader
}

// NewSFU creates a new SFU instance.
func NewSFU(config Config) *SFU {
	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		panic(err)
	}

	return &SFU{
		config:   config,
		api:      webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine)),
		sessions: make(map[string]*Session),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// NewPeerConnection creates a new WebRTC peer connection with the configured ICE servers.
func (s *SFU) NewPeerConnection() (*webrtc.PeerConnection, error) {
	return s.api.NewPeerConnection(webrtc.Configuration{
		ICEServers: s.config.ICEServers,
	})
}

// Session Management

// GetOrCreateSession returns an existing session or creates a new one.
func (s *SFU) GetOrCreateSession(id string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session, ok := s.sessions[id]; ok {
		return session
	}

	session := newSession(id, s)
	s.sessions[id] = session
	return session
}

// GetSession returns a session by ID.
func (s *SFU) GetSession(id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return session, nil
}

// DeleteSession removes and closes a session.
func (s *SFU) DeleteSession(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session, ok := s.sessions[id]; ok {
		session.Close()
		delete(s.sessions, id)
	}
}

// WebSocket Handling

// HandleWebSocket handles incoming WebSocket connections for signaling.
func (s *SFU) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	rawConn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	conn := newWSConn(rawConn)
	defer conn.Close()

	handler := newSignalingHandler(s, conn)
	handler.run()
}

// signalingHandler handles JSON-RPC signaling for a single WebSocket connection.
type signalingHandler struct {
	sfu  *SFU
	conn *wsConn
}

func newSignalingHandler(sfu *SFU, conn *wsConn) *signalingHandler {
	return &signalingHandler{sfu: sfu, conn: conn}
}

func (h *signalingHandler) run() {
	for {
		_, message, err := h.conn.ReadMessage()
		if err != nil {
			break
		}

		var request rpcRequest
		if err := json.Unmarshal(message, &request); err != nil {
			h.sendError(nil, -32700, "Parse error")
			continue
		}

		response := h.handleRequest(&request)
		if response != nil {
			data, _ := json.Marshal(response)
			h.conn.WriteMessage(websocket.TextMessage, data)
		}
	}
}

func (h *signalingHandler) handleRequest(req *rpcRequest) *rpcResponse {
	switch req.Method {
	case "join":
		return h.handleJoin(req)
	case "subscribe":
		return h.handleSubscribe(req)
	case "candidate":
		return h.handleCandidate(req)
	case "answer":
		return h.handleAnswer(req)
	case "leave":
		return h.handleLeave(req)
	default:
		return errorResponse(req.ID, -32601, "Method not found")
	}
}

func (h *signalingHandler) handleJoin(req *rpcRequest) *rpcResponse {
	var params joinParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "Invalid params")
	}

	session := h.sfu.GetOrCreateSession(params.SessionID)
	peer, err := session.AddPeer(params.PeerID, h.conn)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	answer, err := peer.HandleOffer(params.Offer)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	go session.NotifyExistingTracks(peer)

	return successResponse(req.ID, joinResult{Answer: *answer})
}

func (h *signalingHandler) handleSubscribe(req *rpcRequest) *rpcResponse {
	var params subscribeParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "Invalid params")
	}

	session, err := h.sfu.GetSession(params.SessionID)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	if err := session.Subscribe(params.PeerID, params.TargetPeerID); err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	return successResponse(req.ID, map[string]bool{"success": true})
}

func (h *signalingHandler) handleCandidate(req *rpcRequest) *rpcResponse {
	var params candidateParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "Invalid params")
	}

	session, err := h.sfu.GetSession(params.SessionID)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	peer, err := session.GetPeer(params.PeerID)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	if err := peer.AddICECandidate(params.Candidate, params.Target); err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	return successResponse(req.ID, map[string]bool{"success": true})
}

func (h *signalingHandler) handleAnswer(req *rpcRequest) *rpcResponse {
	var params answerParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "Invalid params")
	}

	session, err := h.sfu.GetSession(params.SessionID)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	peer, err := session.GetPeer(params.PeerID)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	if err := peer.HandleAnswer(params.Answer); err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	return successResponse(req.ID, map[string]bool{"success": true})
}

func (h *signalingHandler) handleLeave(req *rpcRequest) *rpcResponse {
	var params leaveParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "Invalid params")
	}

	session, err := h.sfu.GetSession(params.SessionID)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	session.RemovePeer(params.PeerID)
	return successResponse(req.ID, map[string]bool{"success": true})
}

func (h *signalingHandler) sendError(id interface{}, code int, message string) {
	response := errorResponse(id, code, message)
	data, _ := json.Marshal(response)
	h.conn.WriteMessage(websocket.TextMessage, data)
}

// wsConn wraps a WebSocket connection with thread-safe write operations.
type wsConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func newWSConn(conn *websocket.Conn) *wsConn {
	return &wsConn{conn: conn}
}

func (w *wsConn) WriteMessage(messageType int, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteMessage(messageType, data)
}

func (w *wsConn) ReadMessage() (int, []byte, error) {
	return w.conn.ReadMessage()
}

func (w *wsConn) Close() error {
	return w.conn.Close()
}

// JSON-RPC Types

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func successResponse(id interface{}, result interface{}) *rpcResponse {
	return &rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func errorResponse(id interface{}, code int, message string) *rpcResponse {
	return &rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
}

// RPC Params and Results

type joinParams struct {
	SessionID string                    `json:"sessionId"`
	PeerID    string                    `json:"peerId"`
	Offer     webrtc.SessionDescription `json:"offer"`
}

type joinResult struct {
	Answer webrtc.SessionDescription `json:"answer"`
}

type subscribeParams struct {
	SessionID    string `json:"sessionId"`
	PeerID       string `json:"peerId"`
	TargetPeerID string `json:"targetPeerId"`
}

type candidateParams struct {
	SessionID string                  `json:"sessionId"`
	PeerID    string                  `json:"peerId"`
	Candidate webrtc.ICECandidateInit `json:"candidate"`
	Target    string                  `json:"target"`
}

type answerParams struct {
	SessionID string                    `json:"sessionId"`
	PeerID    string                    `json:"peerId"`
	Answer    webrtc.SessionDescription `json:"answer"`
}

type leaveParams struct {
	SessionID string `json:"sessionId"`
	PeerID    string `json:"peerId"`
}
