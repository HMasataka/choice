package sfu

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrPeerNotFound    = errors.New("peer not found")
)

type Config struct {
	ICEServers []webrtc.ICEServer
}

type SFU struct {
	config   Config
	sessions map[string]*Session
	mu       sync.RWMutex
	api      *webrtc.API
	upgrader websocket.Upgrader
}

func NewSFU(config Config) *SFU {
	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		panic(err)
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))

	return &SFU{
		config:   config,
		sessions: make(map[string]*Session),
		api:      api,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (s *SFU) CreateSession(id string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	session := NewSession(id, s)
	s.sessions[id] = session

	return session
}

func (s *SFU) GetSession(id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}

	return session, nil
}

func (s *SFU) DeleteSession(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session, ok := s.sessions[id]; ok {
		session.Close()
		delete(s.sessions, id)
	}
}

func (s *SFU) GetOrCreateSession(id string) *Session {
	session, err := s.GetSession(id)
	if err != nil {
		return s.CreateSession(id)
	}

	return session
}

// JSON-RPC Types
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type JoinParams struct {
	SessionID string                    `json:"sessionId"`
	PeerID    string                    `json:"peerId"`
	Offer     webrtc.SessionDescription `json:"offer"`
}

type JoinResult struct {
	Answer webrtc.SessionDescription `json:"answer"`
}

type SubscribeParams struct {
	SessionID    string `json:"sessionId"`
	PeerID       string `json:"peerId"`
	TargetPeerID string `json:"targetPeerId"`
}

type CandidateParams struct {
	SessionID string                  `json:"sessionId"`
	PeerID    string                  `json:"peerId"`
	Candidate webrtc.ICECandidateInit `json:"candidate"`
	Target    string                  `json:"target"` // "publisher" or "subscriber"
}

type AnswerParams struct {
	SessionID string                    `json:"sessionId"`
	PeerID    string                    `json:"peerId"`
	Answer    webrtc.SessionDescription `json:"answer"`
}

type wsConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *wsConn) WriteMessage(messageType int, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteMessage(messageType, data)
}

func (w *wsConn) ReadMessage() (messageType int, p []byte, err error) {
	return w.conn.ReadMessage()
}

func (w *wsConn) Close() error {
	return w.conn.Close()
}

func (s *SFU) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	rawConn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	conn := &wsConn{conn: rawConn}
	defer conn.Close()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var request JSONRPCRequest
		if err := json.Unmarshal(message, &request); err != nil {
			s.sendError(conn, nil, -32700, "Parse error", nil)
			continue
		}

		response := s.handleRequest(&request, conn)
		if response != nil {
			data, _ := json.Marshal(response)
			conn.WriteMessage(websocket.TextMessage, data)
		}
	}
}

func (s *SFU) handleRequest(request *JSONRPCRequest, conn *wsConn) *JSONRPCResponse {
	switch request.Method {
	case "join":
		return s.handleJoin(request, conn)
	case "subscribe":
		return s.handleSubscribe(request)
	case "candidate":
		return s.handleCandidate(request)
	case "answer":
		return s.handleAnswer(request)
	case "leave":
		return s.handleLeave(request)
	default:
		return s.newErrorResponse(request.ID, -32601, "Method not found", nil)
	}
}

func (s *SFU) handleJoin(request *JSONRPCRequest, conn *wsConn) *JSONRPCResponse {
	var params JoinParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return s.newErrorResponse(request.ID, -32602, "Invalid params", nil)
	}

	session := s.GetOrCreateSession(params.SessionID)
	peer, err := session.AddPeer(params.PeerID, conn)
	if err != nil {
		return s.newErrorResponse(request.ID, -32000, err.Error(), nil)
	}

	answer, err := peer.HandleOffer(params.Offer)
	if err != nil {
		return s.newErrorResponse(request.ID, -32000, err.Error(), nil)
	}

	// Notify new peer about existing tracks
	go session.NotifyExistingTracks(peer)

	return s.newSuccessResponse(request.ID, JoinResult{Answer: *answer})
}

func (s *SFU) handleSubscribe(request *JSONRPCRequest) *JSONRPCResponse {
	var params SubscribeParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return s.newErrorResponse(request.ID, -32602, "Invalid params", nil)
	}

	session, err := s.GetSession(params.SessionID)
	if err != nil {
		return s.newErrorResponse(request.ID, -32000, err.Error(), nil)
	}

	if err := session.Subscribe(params.PeerID, params.TargetPeerID); err != nil {
		return s.newErrorResponse(request.ID, -32000, err.Error(), nil)
	}

	return s.newSuccessResponse(request.ID, map[string]bool{"success": true})
}

func (s *SFU) handleCandidate(request *JSONRPCRequest) *JSONRPCResponse {
	var params CandidateParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return s.newErrorResponse(request.ID, -32602, "Invalid params", nil)
	}

	session, err := s.GetSession(params.SessionID)
	if err != nil {
		return s.newErrorResponse(request.ID, -32000, err.Error(), nil)
	}

	peer, err := session.GetPeer(params.PeerID)
	if err != nil {
		return s.newErrorResponse(request.ID, -32000, err.Error(), nil)
	}

	if err := peer.AddICECandidate(params.Candidate, params.Target); err != nil {
		return s.newErrorResponse(request.ID, -32000, err.Error(), nil)
	}

	return s.newSuccessResponse(request.ID, map[string]bool{"success": true})
}

func (s *SFU) handleAnswer(request *JSONRPCRequest) *JSONRPCResponse {
	var params AnswerParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return s.newErrorResponse(request.ID, -32602, "Invalid params", nil)
	}

	session, err := s.GetSession(params.SessionID)
	if err != nil {
		return s.newErrorResponse(request.ID, -32000, err.Error(), nil)
	}

	peer, err := session.GetPeer(params.PeerID)
	if err != nil {
		return s.newErrorResponse(request.ID, -32000, err.Error(), nil)
	}

	if err := peer.Subscriber().HandleAnswer(params.Answer); err != nil {
		return s.newErrorResponse(request.ID, -32000, err.Error(), nil)
	}

	return s.newSuccessResponse(request.ID, map[string]bool{"success": true})
}

func (s *SFU) handleLeave(request *JSONRPCRequest) *JSONRPCResponse {
	var params struct {
		SessionID string `json:"sessionId"`
		PeerID    string `json:"peerId"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return s.newErrorResponse(request.ID, -32602, "Invalid params", nil)
	}

	session, err := s.GetSession(params.SessionID)
	if err != nil {
		return s.newErrorResponse(request.ID, -32000, err.Error(), nil)
	}

	session.RemovePeer(params.PeerID)

	return s.newSuccessResponse(request.ID, map[string]bool{"success": true})
}

func (s *SFU) sendError(conn *wsConn, id interface{}, code int, message string, data interface{}) {
	response := s.newErrorResponse(id, code, message, data)
	responseData, _ := json.Marshal(response)
	conn.WriteMessage(websocket.TextMessage, responseData)
}

func (s *SFU) newSuccessResponse(id interface{}, result interface{}) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

func (s *SFU) newErrorResponse(id interface{}, code int, message string, data interface{}) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

func (s *SFU) NewPeerConnection() (*webrtc.PeerConnection, error) {
	config := webrtc.Configuration{
		ICEServers: s.config.ICEServers,
	}

	return s.api.NewPeerConnection(config)
}
