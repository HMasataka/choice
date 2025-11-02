package webrtc

import (
	"errors"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// PeerConnectionOptions represents options for peer connection
type PeerConnectionOptions struct {
	ICEServers          []webrtc.ICEServer
	ICECandidateTimeout time.Duration
}

// DefaultPeerConnectionOptions returns default options
func DefaultPeerConnectionOptions() PeerConnectionOptions {
	return PeerConnectionOptions{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
		ICECandidateTimeout: 30 * time.Second,
	}
}

// PeerConnection wraps a WebRTC peer connection
type PeerConnection struct {
	pc      *webrtc.PeerConnection
	options PeerConnectionOptions

	pendingCandidates []webrtc.ICECandidateInit
	candidatesMu      sync.Mutex
}

// NewPeerConnection creates a new peer connection
func NewPeerConnection(id string, options PeerConnectionOptions) (*PeerConnection, error) {
	return nil, nil
}

// Close closes the peer connection
func (p *PeerConnection) Close() error {
	return p.pc.Close()
}

// CreateOffer creates an SDP offer
func (p *PeerConnection) CreateOffer(options *webrtc.OfferOptions) (webrtc.SessionDescription, error) {
	offer, err := p.pc.CreateOffer(options)
	if err != nil {
		return webrtc.SessionDescription{}, errors.New("failed to create offer: " + err.Error())
	}

	if err := p.pc.SetLocalDescription(offer); err != nil {
		return webrtc.SessionDescription{}, errors.New("failed to set local description: " + err.Error())
	}

	<-webrtc.GatheringCompletePromise(p.pc)

	return offer, nil
}

// CreateAnswer creates an SDP answer
func (p *PeerConnection) CreateAnswer(options *webrtc.AnswerOptions) (webrtc.SessionDescription, error) {
	answer, err := p.pc.CreateAnswer(options)
	if err != nil {
		return webrtc.SessionDescription{}, errors.New("failed to create answer: " + err.Error())
	}

	if err := p.pc.SetLocalDescription(answer); err != nil {
		return webrtc.SessionDescription{}, errors.New("failed to set local description: " + err.Error())
	}

	return answer, nil
}

// SetRemoteDescription sets the remote SDP
func (p *PeerConnection) SetRemoteDescription(sdp webrtc.SessionDescription) error {
	if err := p.pc.SetRemoteDescription(sdp); err != nil {
		return errors.New("failed to set remote description: " + err.Error())
	}

	// Process pending ICE candidates if any
	p.processPendingCandidates()

	return nil
}

// AddICECandidate adds an ICE candidate
func (p *PeerConnection) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	// If remote description is not set yet, queue the candidate
	if p.pc.RemoteDescription() == nil {
		p.candidatesMu.Lock()
		p.pendingCandidates = append(p.pendingCandidates, candidate)
		p.candidatesMu.Unlock()
		return nil
	}

	if err := p.pc.AddICECandidate(candidate); err != nil {
		return errors.New("failed to add ICE candidate: " + err.Error())
	}

	return nil
}

// processPendingCandidates processes queued ICE candidates
func (p *PeerConnection) processPendingCandidates() {
	p.candidatesMu.Lock()
	candidates := p.pendingCandidates
	p.pendingCandidates = nil
	p.candidatesMu.Unlock()

	for _, candidate := range candidates {
		if err := p.pc.AddICECandidate(candidate); err != nil {
		}
	}
}
