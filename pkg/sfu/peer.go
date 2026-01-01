package sfu

import (
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

// Peer represents a client connected to the SFU.
// It manages both publishing (sending media) and subscribing (receiving media) connections.
type Peer struct {
	id         string
	session    *Session
	publisher  *Publisher
	subscriber *Subscriber
	conn       *wsConn
	mu         sync.RWMutex
	closed     bool
}

func newPeer(id string, session *Session, conn *wsConn) (*Peer, error) {
	p := &Peer{
		id:      id,
		session: session,
		conn:    conn,
	}

	publisher, err := newPublisher(p)
	if err != nil {
		return nil, err
	}
	p.publisher = publisher

	subscriber, err := newSubscriber(p)
	if err != nil {
		if err := publisher.Close(); err != nil {
			slog.Warn("publisher close error", slog.String("error", err.Error()))
		}
		return nil, err
	}
	p.subscriber = subscriber

	return p, nil
}

// ID returns the peer identifier.
func (p *Peer) ID() string {
	return p.id
}

// Session returns the session this peer belongs to.
func (p *Peer) Session() *Session {
	return p.session
}

// WebRTC Operations

// HandleOffer processes an SDP offer from the client and returns an answer.
func (p *Peer) HandleOffer(offer webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	return p.publisher.HandleOffer(offer)
}

// HandleAnswer processes an SDP answer from the client for the subscriber connection.
func (p *Peer) HandleAnswer(answer webrtc.SessionDescription) error {
	return p.subscriber.HandleAnswer(answer)
}

// AddICECandidate adds an ICE candidate to the appropriate connection.
func (p *Peer) AddICECandidate(candidate webrtc.ICECandidateInit, target string) error {
	if target == "subscriber" {
		return p.subscriber.AddICECandidate(candidate)
	}
	return p.publisher.AddICECandidate(candidate)
}

// Subscribe subscribes to a router to receive its media tracks.
func (p *Peer) Subscribe(router *Router) error {
	return p.subscriber.Subscribe(router)
}

// SetLayer sets the target layer for a track.
func (p *Peer) SetLayer(trackID, layer string) {
	p.subscriber.SetLayer(trackID, layer)
}

// GetLayer returns the current and target layer for a track.
func (p *Peer) GetLayer(trackID string) (current, target string, ok bool) {
	return p.subscriber.GetLayer(trackID)
}

// Signaling

// SendNotification sends a JSON-RPC notification to the client.
func (p *Peer) SendNotification(method string, params map[string]any) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil
	}
	p.mu.RUnlock()

	notification := rpcNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(notification)
	if err != nil {
		return err
	}
	return p.conn.WriteMessage(websocket.TextMessage, data)
}

// SendOffer sends an SDP offer to the client for the subscriber connection.
func (p *Peer) SendOffer(offer webrtc.SessionDescription) error {
	return p.SendNotification("offer", map[string]any{"offer": offer})
}

// SendCandidate sends an ICE candidate to the client.
func (p *Peer) SendCandidate(candidate *webrtc.ICECandidate, target string) error {
	if candidate == nil {
		return nil
	}
	return p.SendNotification("candidate", map[string]any{
		"candidate": candidate.ToJSON(),
		"target":    target,
	})
}

// SendData sends data to the peer via data channel.
func (p *Peer) SendData(data []byte) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil
	}
	p.mu.RUnlock()

	return p.subscriber.SendData(data)
}

// Close closes the peer and all its connections.
func (p *Peer) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	if p.publisher != nil {
		if err := p.publisher.Close(); err != nil {
			slog.Warn("publisher close error", slog.String("error", err.Error()))
		}
	}
	if p.subscriber != nil {
		if err := p.subscriber.Close(); err != nil {
			slog.Warn("subscriber close error", slog.String("error", err.Error()))
		}
	}
	if p.conn != nil {
		if err := p.conn.Close(); err != nil {
			slog.Warn("ws conn close error", slog.String("error", err.Error()))
		}
	}
	return nil
}
