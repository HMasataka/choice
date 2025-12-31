package sfu

import (
	"sync"

	"github.com/pion/rtp"
)

// Forwarder forwards RTP packets from a track to all its downtracks.
type Forwarder struct {
	trackReceiver *TrackReceiver
	downTracks    map[*DownTrack]struct{}
	mu            sync.RWMutex
	closed        bool
	closeCh       chan struct{}
}

// NewForwarder creates a new forwarder.
func NewForwarder(trackReceiver *TrackReceiver) *Forwarder {
	return &Forwarder{
		trackReceiver: trackReceiver,
		downTracks:    make(map[*DownTrack]struct{}),
		closeCh:       make(chan struct{}),
	}
}

// AddDownTrack adds a downtrack to the forwarder.
func (f *Forwarder) AddDownTrack(dt *DownTrack) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return
	}

	f.downTracks[dt] = struct{}{}
}

// RemoveDownTrack removes a downtrack from the forwarder.
func (f *Forwarder) RemoveDownTrack(dt *DownTrack) {
	f.mu.Lock()
	defer f.mu.Unlock()

	delete(f.downTracks, dt)
}

// Forward forwards an RTP packet from a specific layer to all downtracks.
func (f *Forwarder) Forward(packet *rtp.Packet, fromLayer string) {
	f.mu.RLock()
	downTracks := make([]*DownTrack, 0, len(f.downTracks))
	for dt := range f.downTracks {
		downTracks = append(downTracks, dt)
	}
	f.mu.RUnlock()

	for _, dt := range downTracks {
		if err := dt.WriteRTP(packet, fromLayer); err != nil {
			f.RemoveDownTrack(dt)
		}
	}
}

// Close closes the forwarder and all its downtracks.
func (f *Forwarder) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return
	}
	f.closed = true
	close(f.closeCh)

	for dt := range f.downTracks {
		dt.Close()
	}
}
