package sfu

import (
	"io"
	"log"
	"sync"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

const (
	rtpReadTimeout = 30 * time.Second
)

// Receiver receives RTP packets from a remote track.
type Receiver struct {
	track       *webrtc.TrackRemote
	rtpReceiver *webrtc.RTPReceiver
	pc          *webrtc.PeerConnection
	codec       webrtc.RTPCodecParameters
	rid         string // Layer name (simulcast RID or "default")
	closeCh     chan struct{}
	mu          sync.RWMutex
	closed      bool
}

// NewReceiverWithLayer creates a new receiver for a layer.
func NewReceiverWithLayer(track *webrtc.TrackRemote, rtpReceiver *webrtc.RTPReceiver, rid string) *Receiver {
	return &Receiver{
		track:       track,
		rtpReceiver: rtpReceiver,
		codec:       track.Codec(),
		rid:         rid,
		closeCh:     make(chan struct{}),
	}
}

// SetPeerConnection sets the peer connection for sending RTCP.
func (r *Receiver) SetPeerConnection(pc *webrtc.PeerConnection) {
	r.pc = pc
}

// TrackID returns the track identifier.
func (r *Receiver) TrackID() string {
	return r.track.ID()
}

// StreamID returns the stream identifier.
func (r *Receiver) StreamID() string {
	return r.track.StreamID()
}

// Kind returns the track kind (audio/video).
func (r *Receiver) Kind() webrtc.RTPCodecType {
	return r.track.Kind()
}

// Codec returns the RTP codec parameters.
func (r *Receiver) Codec() webrtc.RTPCodecParameters {
	return r.codec
}

// SSRC returns the synchronization source identifier.
func (r *Receiver) SSRC() webrtc.SSRC {
	return r.track.SSRC()
}

// RID returns the layer name.
func (r *Receiver) RID() string {
	return r.rid
}

// SendPLI sends a Picture Loss Indication to request a keyframe.
func (r *Receiver) SendPLI() {
	if r.pc == nil || r.track == nil {
		return
	}

	ssrc := uint32(r.track.SSRC())
	pli := &rtcp.PictureLossIndication{
		MediaSSRC: ssrc,
	}

	if err := r.pc.WriteRTCP([]rtcp.Packet{pli}); err != nil {
		log.Printf("[Receiver] Failed to send PLI: %v", err)
		return
	}

	log.Printf("[Receiver] PLI sent for SSRC %d (track: %s)", ssrc, r.track.ID())
}

// ReadRTPPacket reads a single RTP packet.
func (r *Receiver) ReadRTPPacket() (*rtp.Packet, error) {
	select {
	case <-r.closeCh:
		return nil, io.EOF
	default:
	}

	r.track.SetReadDeadline(time.Now().Add(rtpReadTimeout))

	packet, _, err := r.track.ReadRTP()
	if err != nil {
		return nil, err
	}

	return packet, nil
}

// Close closes the receiver.
func (r *Receiver) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}
	r.closed = true
	close(r.closeCh)

	return nil
}
