package sfu

import (
	"log/slog"
	"sync"

	"github.com/pion/webrtc/v4"
)

// Publisher handles the publishing (upstream) connection from a client.
type Publisher struct {
	peer   *Peer
	pc     *webrtc.PeerConnection
	router *Router
	tracks map[string]*TrackReceiver
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
		tracks: make(map[string]*TrackReceiver),
	}

	pc.OnTrack(p.onTrack)
	pc.OnDataChannel(p.onDataChannel)
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if err := peer.SendCandidate(c, "publisher"); err != nil {
			slog.Warn("send candidate (publisher) failed", slog.String("error", err.Error()))
		}
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			if err := p.Close(); err != nil {
				slog.Warn("publisher close error", slog.String("error", err.Error()))
			}
		}
	})

	return p, nil
}

// onDataChannel handles incoming data channels from the client.
func (p *Publisher) onDataChannel(dc *webrtc.DataChannel) {
	slog.Info("[Publisher] Data channel received",
		slog.String("label", dc.Label()),
		slog.String("peerID", p.peer.id),
	)

	dc.OnOpen(func() {
		slog.Info("[Publisher] Data channel opened",
			slog.String("label", dc.Label()),
			slog.String("peerID", p.peer.id),
		)
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		slog.Debug("[Publisher] Data channel message received",
			slog.String("peerID", p.peer.id),
			slog.Int("size", len(msg.Data)),
		)
		p.peer.session.BroadcastData(p.peer.id, msg.Data)
	})

	dc.OnClose(func() {
		slog.Info("[Publisher] Data channel closed",
			slog.String("label", dc.Label()),
			slog.String("peerID", p.peer.id),
		)
	})
}

// onTrack handles incoming tracks from the client.
func (p *Publisher) onTrack(remoteTrack *webrtc.TrackRemote, rtpReceiver *webrtc.RTPReceiver) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}

	trackID := remoteTrack.ID()
	rid := remoteTrack.RID()

	// Determine layer name
	layerName := rid
	if layerName == "" {
		layerName = LayerDefault
	}

	slog.Info("[Publisher] onTrack", "trackID", trackID, "kind", remoteTrack.Kind().String(), "layer", layerName)

	// Get or create TrackReceiver
	track, exists := p.tracks[trackID]
	isNewTrack := !exists
	if isNewTrack {
		track = NewTrackReceiver(trackID, remoteTrack.StreamID(), remoteTrack.Kind())
		p.tracks[trackID] = track
	}
	p.mu.Unlock()

	// Create layer receiver
	receiver := NewLayerReceiver(remoteTrack, rtpReceiver, layerName)
	receiver.SetPeerConnection(p.pc)
	track.AddLayer(layerName, receiver)

	// Register track with router (only for new tracks)
	if isNewTrack {
		p.router.AddTrack(track)
		p.peer.session.AddRouter(p.peer.id, p.router)
	}

	// Start reading RTP
	go p.readRTP(receiver, track, layerName)
}

// readRTP reads RTP packets from a layer and forwards them.
func (p *Publisher) readRTP(receiver *LayerReceiver, track *TrackReceiver, layerName string) {
	defer func() {
		if err := receiver.Close(); err != nil {
			slog.Warn("receiver close error", slog.String("error", err.Error()))
		}
	}()

	for {
		packet, err := receiver.ReadRTP()
		if err != nil {
			return
		}

		p.router.Forward(track.TrackID(), packet, layerName)
	}
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

	for _, track := range p.tracks {
		if err := track.Close(); err != nil {
			slog.Warn("track close error", slog.String("error", err.Error()))
		}
	}
	p.mu.Unlock()

	if p.router != nil {
		if err := p.router.Close(); err != nil {
			slog.Warn("router close error", slog.String("error", err.Error()))
		}
	}
	return p.pc.Close()
}
