package sfu

import (
	"log"
	"sync"
)

// Router manages media routing from a single publisher to multiple subscribers.
// It holds receivers for incoming tracks and tracks which subscribers are connected.
type Router struct {
	id          string
	session     *Session
	receivers   map[string]*Receiver
	subscribers map[*Subscriber]struct{}
	mu          sync.RWMutex
	closed      bool
}

// NewRouter creates a new router for a publisher.
func NewRouter(id string, session *Session) *Router {
	return &Router{
		id:          id,
		session:     session,
		receivers:   make(map[string]*Receiver),
		subscribers: make(map[*Subscriber]struct{}),
	}
}

// ID returns the router identifier (same as the publisher's peer ID).
func (r *Router) ID() string {
	return r.id
}

// Receiver Management

// AddReceiver adds a new receiver and notifies existing subscribers about the new track.
func (r *Router) AddReceiver(receiver *Receiver) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}

	r.receivers[receiver.TrackID()] = receiver

	// Copy subscribers to update outside the lock
	subscribers := make([]*Subscriber, 0, len(r.subscribers))
	for sub := range r.subscribers {
		subscribers = append(subscribers, sub)
	}
	r.mu.Unlock()

	// Add the new track to all existing subscribers
	for _, sub := range subscribers {
		log.Printf("[Router] Adding track %s to existing subscriber", receiver.TrackID())
		if err := sub.AddDownTrack(receiver); err != nil {
			log.Printf("[Router] Error adding downtrack: %v", err)
			continue
		}
		if err := sub.Negotiate(); err != nil {
			log.Printf("[Router] Error negotiating: %v", err)
		}
	}

	// Notify other peers about the new track
	r.session.Broadcast(r.id, "trackAdded", map[string]interface{}{
		"peerId":   r.id,
		"trackId":  receiver.TrackID(),
		"streamId": receiver.StreamID(),
		"kind":     receiver.Kind().String(),
	})
}

// GetReceivers returns all receivers.
func (r *Router) GetReceivers() []*Receiver {
	r.mu.RLock()
	defer r.mu.RUnlock()

	receivers := make([]*Receiver, 0, len(r.receivers))
	for _, receiver := range r.receivers {
		receivers = append(receivers, receiver)
	}
	return receivers
}

// Subscription Management

// Subscribe adds a subscriber and connects all current receivers to it.
func (r *Router) Subscribe(subscriber *Subscriber) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.subscribers[subscriber] = struct{}{}

	for _, receiver := range r.receivers {
		if err := subscriber.AddDownTrack(receiver); err != nil {
			return err
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

// Close closes the router and all its receivers.
func (r *Router) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true

	receivers := make([]*Receiver, 0, len(r.receivers))
	for _, receiver := range r.receivers {
		receivers = append(receivers, receiver)
	}
	r.mu.Unlock()

	for _, receiver := range receivers {
		receiver.Close()
	}
	return nil
}
