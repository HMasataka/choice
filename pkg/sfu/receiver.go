package sfu

import (
	"io"
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

const (
	rtpReadTimeout = 30 * time.Second
	rtpBufferSize  = 100
)

// Receiver receives RTP packets from a remote track and forwards them to DownTracks.
type Receiver struct {
	track       *webrtc.TrackRemote
	rtpReceiver *webrtc.RTPReceiver
	codec       webrtc.RTPCodecParameters
	downTracks  []*DownTrack
	closeCh     chan struct{}
	mu          sync.RWMutex
	closed      bool
}

// NewReceiver creates a new receiver for a remote track.
func NewReceiver(track *webrtc.TrackRemote, rtpReceiver *webrtc.RTPReceiver) *Receiver {
	return &Receiver{
		track:       track,
		rtpReceiver: rtpReceiver,
		codec:       track.Codec(),
		downTracks:  make([]*DownTrack, 0),
		closeCh:     make(chan struct{}),
	}
}

// Track Information

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

// DownTrack Management

// AddDownTrack adds a downtrack to receive RTP packets.
func (r *Receiver) AddDownTrack(dt *DownTrack) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return
	}
	r.downTracks = append(r.downTracks, dt)
}

// RemoveDownTrack removes a downtrack.
func (r *Receiver) RemoveDownTrack(dt *DownTrack) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, d := range r.downTracks {
		if d == dt {
			r.downTracks = append(r.downTracks[:i], r.downTracks[i+1:]...)
			return
		}
	}
}

// RTP Processing

// ReadRTP continuously reads RTP packets and forwards them to all downtracks.
func (r *Receiver) ReadRTP() {
	defer r.Close()

	for {
		select {
		case <-r.closeCh:
			return
		default:
		}

		r.track.SetReadDeadline(time.Now().Add(rtpReadTimeout))

		packet, _, err := r.track.ReadRTP()
		if err != nil {
			if err == io.EOF {
				return
			}
			continue
		}

		r.forwardRTP(packet)
	}
}

func (r *Receiver) forwardRTP(packet *rtp.Packet) {
	r.mu.RLock()
	downTracks := make([]*DownTrack, len(r.downTracks))
	copy(downTracks, r.downTracks)
	r.mu.RUnlock()

	for _, dt := range downTracks {
		if err := dt.WriteRTP(packet); err != nil {
			r.RemoveDownTrack(dt)
		}
	}
}

// Close closes the receiver and all its downtracks.
func (r *Receiver) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	close(r.closeCh)

	downTracks := make([]*DownTrack, len(r.downTracks))
	copy(downTracks, r.downTracks)
	r.mu.Unlock()

	for _, dt := range downTracks {
		dt.Close()
	}
	return nil
}
