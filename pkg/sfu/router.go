package sfu

import (
	"log"
	"sync"
)

type Router struct {
	id          string
	session     *Session
	receivers   map[string]*Receiver
	subscribers map[*Subscriber]struct{}
	mu          sync.RWMutex
	closed      bool
}

func NewRouter(id string, session *Session) *Router {
	return &Router{
		id:          id,
		session:     session,
		receivers:   make(map[string]*Receiver),
		subscribers: make(map[*Subscriber]struct{}),
	}
}

func (r *Router) ID() string {
	return r.id
}

func (r *Router) Session() *Session {
	return r.session
}

func (r *Router) AddReceiver(receiver *Receiver) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}

	r.receivers[receiver.TrackID()] = receiver

	// Add the new track to all existing subscribers
	subscribersToUpdate := make([]*Subscriber, 0, len(r.subscribers))
	for sub := range r.subscribers {
		subscribersToUpdate = append(subscribersToUpdate, sub)
	}
	r.mu.Unlock()

	// Add downtrack and trigger renegotiation for each subscriber
	for _, sub := range subscribersToUpdate {
		log.Printf("Router.AddReceiver: adding new track %s to existing subscriber", receiver.TrackID())
		if err := sub.AddDownTrack(receiver); err != nil {
			log.Printf("Router.AddReceiver: error adding downtrack: %v", err)
			continue
		}
		if err := sub.Negotiate(); err != nil {
			log.Printf("Router.AddReceiver: error negotiating: %v", err)
		}
	}

	r.notifyNewTrack(receiver)
}

func (r *Router) RemoveReceiver(trackID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if receiver, exists := r.receivers[trackID]; exists {
		receiver.Close()
		delete(r.receivers, trackID)
	}
}

func (r *Router) GetReceiver(trackID string) (*Receiver, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	receiver, ok := r.receivers[trackID]
	return receiver, ok
}

func (r *Router) GetReceivers() []*Receiver {
	r.mu.RLock()
	defer r.mu.RUnlock()

	receivers := make([]*Receiver, 0, len(r.receivers))
	for _, receiver := range r.receivers {
		receivers = append(receivers, receiver)
	}

	return receivers
}

func (r *Router) notifyNewTrack(receiver *Receiver) {
	notification := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "trackAdded",
		"params": map[string]interface{}{
			"peerId":   r.id,
			"trackId":  receiver.TrackID(),
			"streamId": receiver.StreamID(),
			"kind":     receiver.Kind().String(),
		},
	}

	r.session.Broadcast(r.id, notification)
}

func (r *Router) Subscribe(subscriber *Subscriber) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Track this subscriber so we can add new tracks to it later
	r.subscribers[subscriber] = struct{}{}

	for _, receiver := range r.receivers {
		if err := subscriber.AddDownTrack(receiver); err != nil {
			return err
		}
	}

	return nil
}

func (r *Router) Unsubscribe(subscriber *Subscriber) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.subscribers, subscriber)
}

func (r *Router) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	r.mu.Unlock()

	for _, receiver := range r.receivers {
		receiver.Close()
	}

	return nil
}
