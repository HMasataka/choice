package sfu

import (
	"log"
	"sync"

	"github.com/pion/webrtc/v4"
)

// Publisher handles the publishing (upstream) connection from a client.
// It receives media tracks from the client and forwards them through a Router.
type Publisher struct {
	peer             *Peer
	pc               *webrtc.PeerConnection
	router           *Router
	simulcastReceivers map[string]*SimulcastReceiver // key: track ID (without RID)
	mu               sync.RWMutex
	closed           bool
}

func newPublisher(peer *Peer) (*Publisher, error) {
	pc, err := peer.session.sfu.NewPeerConnection()
	if err != nil {
		return nil, err
	}

	p := &Publisher{
		peer:               peer,
		pc:                 pc,
		router:             NewRouter(peer.id, peer.session),
		simulcastReceivers: make(map[string]*SimulcastReceiver),
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

	rid := track.RID()
	isSimulcast := rid != ""

	log.Printf("[Publisher] onTrack: id=%s, kind=%s, rid=%s, isSimulcast=%v",
		track.ID(), track.Kind(), rid, isSimulcast)

	if isSimulcast {
		p.handleSimulcastTrack(track, rtpReceiver)
	} else {
		p.handleRegularTrack(track, rtpReceiver)
	}
}

// handleRegularTrack handles non-simulcast tracks
func (p *Publisher) handleRegularTrack(track *webrtc.TrackRemote, rtpReceiver *webrtc.RTPReceiver) {
	p.mu.Unlock()

	log.Printf("[Publisher] Received regular track: %s (%s)", track.ID(), track.Kind())

	receiver := NewReceiver(track, rtpReceiver)
	p.router.AddReceiver(receiver)
	p.peer.session.AddRouter(p.peer.id, p.router)

	go receiver.ReadRTP()
}

// handleSimulcastTrack handles simulcast tracks
func (p *Publisher) handleSimulcastTrack(track *webrtc.TrackRemote, rtpReceiver *webrtc.RTPReceiver) {
	rid := track.RID()
	trackID := track.ID()

	log.Printf("[Publisher] Received simulcast track: %s, RID: %s (%s)", trackID, rid, track.Kind())

	// Get or create SimulcastReceiver for this track
	simulcastRecv, exists := p.simulcastReceivers[trackID]
	isNewReceiver := !exists
	if isNewReceiver {
		simulcastRecv = NewSimulcastReceiver(trackID, track.StreamID(), track.Kind())
		p.simulcastReceivers[trackID] = simulcastRecv
	}
	p.mu.Unlock()

	// Create receiver for this layer and add it BEFORE notifying router
	receiver := NewReceiverWithLayer(track, rtpReceiver, rid)
	simulcastRecv.AddLayer(rid, receiver)

	// Add to router only after first layer is added
	if isNewReceiver {
		p.router.AddSimulcastReceiver(simulcastRecv)
		p.peer.session.AddRouter(p.peer.id, p.router)
	}

	// Start reading RTP with layer-aware forwarding
	go p.readSimulcastRTP(receiver, simulcastRecv, rid)
}

// readSimulcastRTP reads RTP from a simulcast layer and forwards through the SimulcastReceiver
func (p *Publisher) readSimulcastRTP(receiver *Receiver, simulcastRecv *SimulcastReceiver, rid string) {
	defer receiver.Close()

	for {
		packet, err := receiver.ReadRTPPacket()
		if err != nil {
			return
		}

		// Forward to all downtracks with layer information
		p.router.ForwardSimulcastRTP(simulcastRecv.TrackID(), packet, rid)
	}
}

// GetSimulcastReceiver returns the simulcast receiver for a track
func (p *Publisher) GetSimulcastReceiver(trackID string) (*SimulcastReceiver, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	recv, ok := p.simulcastReceivers[trackID]
	return recv, ok
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

// RequestKeyframe sends a PLI to request a keyframe for a specific layer
func (p *Publisher) RequestKeyframe(trackID string, rid string) error {
	// Find the appropriate receiver and send PLI
	p.mu.RLock()
	simulcastRecv, ok := p.simulcastReceivers[trackID]
	p.mu.RUnlock()

	if !ok {
		return nil
	}

	layer, ok := simulcastRecv.GetLayer(rid)
	if !ok {
		return nil
	}

	// Send PLI via RTCP
	log.Printf("[Publisher] Requesting keyframe for track %s, layer %s", trackID, rid)
	_ = layer // TODO: Implement actual PLI sending
	return nil
}

// Close closes the publisher and its router.
func (p *Publisher) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true

	// Close all simulcast receivers
	for _, recv := range p.simulcastReceivers {
		recv.Close()
	}
	p.mu.Unlock()

	if p.router != nil {
		p.router.Close()
	}
	return p.pc.Close()
}
