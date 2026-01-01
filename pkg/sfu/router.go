package sfu

import (
	"log/slog"
	"sync"

	"github.com/pion/rtp"
)

// Router manages media routing from a publisher to multiple subscribers.
type Router struct {
	id          string
	session     *Session
	tracks      map[string]*TrackReceiver
	forwarders  map[string]*Forwarder
	subscribers map[*Subscriber]struct{}
	mu          sync.RWMutex
	closed      bool
}

// NewRouter creates a new router.
func NewRouter(id string, session *Session) *Router {
	return &Router{
		id:          id,
		session:     session,
		tracks:      make(map[string]*TrackReceiver),
		forwarders:  make(map[string]*Forwarder),
		subscribers: make(map[*Subscriber]struct{}),
	}
}

// ID returns the router identifier (same as the publisher's peer ID).
func (r *Router) ID() string {
	return r.id
}

// AddTrack adds a new track receiver and notifies existing subscribers.
func (r *Router) AddTrack(track *TrackReceiver) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}

	trackID := track.TrackID()
	r.tracks[trackID] = track

	forwarder := NewForwarder(track)
	r.forwarders[trackID] = forwarder

	subscribers := make([]*Subscriber, 0, len(r.subscribers))
	for sub := range r.subscribers {
		subscribers = append(subscribers, sub)
	}
	r.mu.Unlock()

	// Add downtrack to existing subscribers
	for _, sub := range subscribers {
		slog.Info("[Router] Adding track to existing subscriber", "trackID", trackID)
		if err := sub.AddDownTrack(track); err != nil {
			slog.Warn("[Router] Error adding downtrack to subscriber", "error", err, "trackID", trackID)
			continue
		}

		if dt := sub.GetDownTrack(trackID); dt != nil {
			forwarder.AddDownTrack(dt)
		}

		if err := sub.Negotiate(); err != nil {
			slog.Warn("[Router] Error negotiating with subscriber", "error", err, "trackID", trackID)
		}
	}

	// Notify other peers
	r.session.Broadcast(r.id, "trackAdded", map[string]any{
		"peerId":   r.id,
		"trackId":  trackID,
		"streamId": track.StreamID(),
		"kind":     track.Kind().String(),
	})
}

// GetTrack returns a track receiver by ID.
func (r *Router) GetTrack(trackID string) (*TrackReceiver, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	track, ok := r.tracks[trackID]
	return track, ok
}

// GetTracks returns all track receivers.
func (r *Router) GetTracks() map[string]*TrackReceiver {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tracks := make(map[string]*TrackReceiver, len(r.tracks))
	for k, v := range r.tracks {
		tracks[k] = v
	}
	return tracks
}

// Forward forwards an RTP packet to all subscribers.
func (r *Router) Forward(trackID string, packet *rtp.Packet, layerName string) {
	r.mu.RLock()
	forwarder, ok := r.forwarders[trackID]
	r.mu.RUnlock()

	if !ok {
		return
	}

	forwarder.Forward(packet, layerName)
}

// Subscribe adds a subscriber and connects all current tracks to it.
func (r *Router) Subscribe(subscriber *Subscriber) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.subscribers[subscriber] = struct{}{}

	for trackID, track := range r.tracks {
		if err := subscriber.AddDownTrack(track); err != nil {
			return err
		}

		if forwarder, ok := r.forwarders[trackID]; ok {
			if dt := subscriber.GetDownTrack(trackID); dt != nil {
				forwarder.AddDownTrack(dt)
				slog.Info("[Router] Added downtrack to forwarder", "trackID", trackID)
			}
		}
	}

	return nil
}

// Unsubscribe removes a subscriber from this router.
func (r *Router) Unsubscribe(subscriber *Subscriber) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.subscribers, subscriber)
}

// Close closes the router and all its tracks.
func (r *Router) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true

	tracks := make([]*TrackReceiver, 0, len(r.tracks))
	for _, track := range r.tracks {
		tracks = append(tracks, track)
	}

	forwarders := make([]*Forwarder, 0, len(r.forwarders))
	for _, fwd := range r.forwarders {
		forwarders = append(forwarders, fwd)
	}
	r.mu.Unlock()

	for _, track := range tracks {
		if err := track.Close(); err != nil {
			slog.Warn("track close error", slog.String("error", err.Error()))
		}
	}

	for _, fwd := range forwarders {
		fwd.Close()
	}

	return nil
}
