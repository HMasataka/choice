package sfu

import (
	"sync"

	"github.com/pion/webrtc/v4"
)

type Publisher struct {
	peer      *PeerConnection
	pc        *webrtc.PeerConnection
	receivers map[string]*Receiver
	router    *Router
	mu        sync.RWMutex
	closed    bool
}

func NewPublisher(peer *PeerConnection) (*Publisher, error) {
	pc, err := peer.session.sfu.NewPeerConnection()
	if err != nil {
		return nil, err
	}

	p := &Publisher{
		peer:      peer,
		pc:        pc,
		receivers: make(map[string]*Receiver),
	}

	p.router = NewRouter(peer.id, peer.session)

	pc.OnTrack(func(track *webrtc.TrackRemote, rtpReceiver *webrtc.RTPReceiver) {
		p.handleTrack(track, rtpReceiver)
	})

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		peer.SendCandidate(candidate, "publisher")
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateFailed ||
			state == webrtc.PeerConnectionStateClosed {
			p.Close()
		}
	})

	return p, nil
}

func (p *Publisher) handleTrack(track *webrtc.TrackRemote, rtpReceiver *webrtc.RTPReceiver) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}

	receiver := NewReceiver(track, rtpReceiver)
	p.receivers[track.ID()] = receiver

	p.router.AddReceiver(receiver)

	p.peer.session.AddRouter(p.peer.id, p.router)

	go receiver.ReadRTP()
}

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

func (p *Publisher) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	return p.pc.AddICECandidate(candidate)
}

func (p *Publisher) GetReceivers() []*Receiver {
	p.mu.RLock()
	defer p.mu.RUnlock()

	receivers := make([]*Receiver, 0, len(p.receivers))
	for _, r := range p.receivers {
		receivers = append(receivers, r)
	}

	return receivers
}

func (p *Publisher) Router() *Router {
	return p.router
}

func (p *Publisher) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	for _, receiver := range p.receivers {
		receiver.Close()
	}

	if p.router != nil {
		p.router.Close()
	}

	return p.pc.Close()
}
