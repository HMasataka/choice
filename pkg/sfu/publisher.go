package sfu

import (
	"log"
	"sync"

	"github.com/pion/webrtc/v4"
)

// Publisher handles the publishing (upstream) connection from a client.
// It receives media tracks from the client and forwards them through a Router.
type Publisher struct {
	peer   *Peer
	pc     *webrtc.PeerConnection
	router *Router
	mu     sync.RWMutex
	closed bool
}

func newPublisher(peer *Peer) (*Publisher, error) {
	pc, err := peer.session.sfu.NewPeerConnection()
	if err != nil {
		return nil, err
	}

	p := &Publisher{
		peer:   peer,
		pc:     pc,
		router: NewRouter(peer.id, peer.session),
	}

	pc.OnTrack(p.onTrack)
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		peer.SendCandidate(c, "publisher")
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			p.Close()
		}
	})

	return p, nil
}

// onTrack handles incoming tracks from the client.
func (p *Publisher) onTrack(track *webrtc.TrackRemote, rtpReceiver *webrtc.RTPReceiver) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()

	log.Printf("[Publisher] Received track: %s (%s)", track.ID(), track.Kind())

	receiver := NewReceiver(track, rtpReceiver)
	p.router.AddReceiver(receiver)
	p.peer.session.AddRouter(p.peer.id, p.router)

	go receiver.ReadRTP()
}

// HandleOffer processes an SDP offer and returns an answer.
func (p *Publisher) HandleOffer(offer webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	if err := p.pc.SetRemoteDescription(offer); err != nil {
		return nil, err
	}

	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		return nil, err
	}

	if err := p.pc.SetLocalDescription(answer); err != nil {
		return nil, err
	}

	return &answer, nil
}

// AddICECandidate adds an ICE candidate to the publisher connection.
func (p *Publisher) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	return p.pc.AddICECandidate(candidate)
}

// Router returns the router for this publisher.
func (p *Publisher) Router() *Router {
	return p.router
}

// Close closes the publisher and its router.
func (p *Publisher) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	if p.router != nil {
		p.router.Close()
	}
	return p.pc.Close()
}
