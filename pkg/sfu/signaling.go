package sfu

import (
	"encoding/json"
	"log"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

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
	case "setLayer":
		return h.handleSetLayer(req)
	case "getLayer":
		return h.handleGetLayer(req)
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

func (h *signalingHandler) handleSetLayer(req *rpcRequest) *rpcResponse {
	var params setLayerParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "Invalid params")
	}

	log.Printf("[Signaling] setLayer: session=%s, peer=%s, trackId=%s, layer=%s",
		params.SessionID, params.PeerID, params.TrackID, params.Layer)

	session, err := h.sfu.GetSession(params.SessionID)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	peer, err := session.GetPeer(params.PeerID)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	peer.SetLayer(params.TrackID, params.Layer)

	return successResponse(req.ID, map[string]bool{"success": true})
}

func (h *signalingHandler) handleGetLayer(req *rpcRequest) *rpcResponse {
	var params getLayerParams
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

	current, target, ok := peer.GetLayer(params.TrackID)
	if !ok {
		return errorResponse(req.ID, -32000, "Track not found")
	}

	return successResponse(req.ID, getLayerResult{
		CurrentLayer: current,
		TargetLayer:  target,
	})
}

func (h *signalingHandler) sendError(id interface{}, code int, message string) {
	response := errorResponse(id, code, message)
	data, _ := json.Marshal(response)
	h.conn.WriteMessage(websocket.TextMessage, data)
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

type rpcNotification struct {
	JSONRPC string                 `json:"jsonrpc"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params"`
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

type setLayerParams struct {
	SessionID string `json:"sessionId"`
	PeerID    string `json:"peerId"`
	TrackID   string `json:"trackId"`
	Layer     string `json:"layer"` // "high", "mid", or "low"
}

type getLayerParams struct {
	SessionID string `json:"sessionId"`
	PeerID    string `json:"peerId"`
	TrackID   string `json:"trackId"`
}

type getLayerResult struct {
	CurrentLayer string `json:"currentLayer"`
	TargetLayer  string `json:"targetLayer"`
}
