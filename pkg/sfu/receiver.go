package sfu

import (
	"io"
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

type Receiver struct {
	track       *webrtc.TrackRemote
	rtpReceiver *webrtc.RTPReceiver
	downTracks  []*DownTrack
	mu          sync.RWMutex
	closed      bool
	closeCh     chan struct{}
	rtpCh       chan *rtp.Packet
	codec       webrtc.RTPCodecParameters
}

func NewReceiver(track *webrtc.TrackRemote, rtpReceiver *webrtc.RTPReceiver) *Receiver {
	return &Receiver{
		track:       track,
		rtpReceiver: rtpReceiver,
		downTracks:  make([]*DownTrack, 0),
		closeCh:     make(chan struct{}),
		rtpCh:       make(chan *rtp.Packet, 100),
		codec:       track.Codec(),
	}
}

func (r *Receiver) Track() *webrtc.TrackRemote {
	return r.track
}

func (r *Receiver) Codec() webrtc.RTPCodecParameters {
	return r.codec
}

func (r *Receiver) Kind() webrtc.RTPCodecType {
	return r.track.Kind()
}

func (r *Receiver) SSRC() webrtc.SSRC {
	return r.track.SSRC()
}

func (r *Receiver) TrackID() string {
	return r.track.ID()
}

func (r *Receiver) StreamID() string {
	return r.track.StreamID()
}

func (r *Receiver) AddDownTrack(dt *DownTrack) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return
	}

	r.downTracks = append(r.downTracks, dt)
}

func (r *Receiver) RemoveDownTrack(dt *DownTrack) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, d := range r.downTracks {
		if d == dt {
			r.downTracks = append(r.downTracks[:i], r.downTracks[i+1:]...)
			break
		}
	}
}

func (r *Receiver) ReadRTP() {
	defer r.Close()

	for {
		select {
		case <-r.closeCh:
			return
		default:
		}

		r.track.SetReadDeadline(time.Now().Add(time.Second * 30))

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

func (r *Receiver) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	close(r.closeCh)
	r.mu.Unlock()

	for _, dt := range r.downTracks {
		dt.Close()
	}

	return nil
}
