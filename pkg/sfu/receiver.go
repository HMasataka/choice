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

// LayerReceiver receives RTP packets from a single quality layer.
type LayerReceiver struct {
	track       *webrtc.TrackRemote
	rtpReceiver *webrtc.RTPReceiver
	pc          *webrtc.PeerConnection
	codec       webrtc.RTPCodecParameters
	layerName   string
	closeCh     chan struct{}
	mu          sync.RWMutex
	closed      bool
}

// NewLayerReceiver creates a new layer receiver.
func NewLayerReceiver(track *webrtc.TrackRemote, rtpReceiver *webrtc.RTPReceiver, layerName string) *LayerReceiver {
	return &LayerReceiver{
		track:       track,
		rtpReceiver: rtpReceiver,
		codec:       track.Codec(),
		layerName:   layerName,
		closeCh:     make(chan struct{}),
	}
}

// SetPeerConnection sets the peer connection for sending RTCP.
func (r *LayerReceiver) SetPeerConnection(pc *webrtc.PeerConnection) {
	r.pc = pc
}

// TrackID returns the track identifier.
func (r *LayerReceiver) TrackID() string {
	return r.track.ID()
}

// StreamID returns the stream identifier.
func (r *LayerReceiver) StreamID() string {
	return r.track.StreamID()
}

// Kind returns the track kind (audio/video).
func (r *LayerReceiver) Kind() webrtc.RTPCodecType {
	return r.track.Kind()
}

// Codec returns the RTP codec parameters.
func (r *LayerReceiver) Codec() webrtc.RTPCodecParameters {
	return r.codec
}

// SSRC returns the synchronization source identifier.
func (r *LayerReceiver) SSRC() webrtc.SSRC {
	return r.track.SSRC()
}

// LayerName returns the layer name.
func (r *LayerReceiver) LayerName() string {
	return r.layerName
}

// SendPLI sends a Picture Loss Indication to request a keyframe.
func (r *LayerReceiver) SendPLI() {
	if r.pc == nil || r.track == nil {
		return
	}

	ssrc := uint32(r.track.SSRC())
	pli := &rtcp.PictureLossIndication{
		MediaSSRC: ssrc,
	}

	if err := r.pc.WriteRTCP([]rtcp.Packet{pli}); err != nil {
		log.Printf("[LayerReceiver] Failed to send PLI: %v", err)
		return
	}

	log.Printf("[LayerReceiver] PLI sent for SSRC %d (track: %s, layer: %s)", ssrc, r.track.ID(), r.layerName)
}

// ReadRTP reads a single RTP packet.
func (r *LayerReceiver) ReadRTP() (*rtp.Packet, error) {
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
func (r *LayerReceiver) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}
	r.closed = true
	close(r.closeCh)

	return nil
}
