package sfu

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

type PeerConnection struct {
	id         string
	session    *Session
	publisher  *Publisher
	subscriber *Subscriber
	conn       *websocket.Conn
	mu         sync.RWMutex
	closed     bool
}

func NewPeerConnection(id string, session *Session, conn *websocket.Conn) (*PeerConnection, error) {
	pc := &PeerConnection{
		id:      id,
		session: session,
		conn:    conn,
	}

	publisher, err := NewPublisher(pc)
	if err != nil {
		return nil, err
	}
	pc.publisher = publisher

	subscriber, err := NewSubscriber(pc)
	if err != nil {
		publisher.Close()
		return nil, err
	}
	pc.subscriber = subscriber

	return pc, nil
}

func (p *PeerConnection) ID() string {
	return p.id
}

func (p *PeerConnection) Session() *Session {
	return p.session
}

func (p *PeerConnection) Publisher() *Publisher {
	return p.publisher
}

func (p *PeerConnection) Subscriber() *Subscriber {
	return p.subscriber
}

func (p *PeerConnection) HandleOffer(offer webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	return p.publisher.HandleOffer(offer)
}

func (p *PeerConnection) AddICECandidate(candidate webrtc.ICECandidateInit, target string) error {
	if target == "subscriber" {
		return p.subscriber.AddICECandidate(candidate)
	}
	return p.publisher.AddICECandidate(candidate)
}

func (p *PeerConnection) Subscribe(router *Router) error {
	return p.subscriber.Subscribe(router)
}

func (p *PeerConnection) SendMessage(message interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return p.conn.WriteMessage(websocket.TextMessage, data)
}

func (p *PeerConnection) SendOffer(offer webrtc.SessionDescription) error {
	notification := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "offer",
		Params:  mustMarshal(map[string]interface{}{"offer": offer}),
	}
	return p.SendMessage(notification)
}

func (p *PeerConnection) SendCandidate(candidate *webrtc.ICECandidate, target string) error {
	if candidate == nil {
		return nil
	}

	notification := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "candidate",
		Params: mustMarshal(map[string]interface{}{
			"candidate": candidate.ToJSON(),
			"target":    target,
		}),
	}
	return p.SendMessage(notification)
}

func (p *PeerConnection) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	if p.publisher != nil {
		p.publisher.Close()
	}

	if p.subscriber != nil {
		p.subscriber.Close()
	}

	if p.conn != nil {
		p.conn.Close()
	}

	return nil
}

func mustMarshal(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
