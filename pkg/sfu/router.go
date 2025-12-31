package sfu

import (
	"log"
	"sync"

	"github.com/pion/rtp"
)

// Router manages media routing from a single publisher to multiple subscribers.
// It holds receivers for incoming tracks and tracks which subscribers are connected.
type Router struct {
	id                 string
	session            *Session
	receivers          map[string]*Receiver
	simulcastReceivers map[string]*SimulcastReceiver
	simulcastForwarders map[string]*SimulcastForwarder
	subscribers        map[*Subscriber]struct{}
	mu                 sync.RWMutex
	closed             bool
}

// NewRouter creates a new router for a publisher.
func NewRouter(id string, session *Session) *Router {
	return &Router{
		id:                  id,
		session:             session,
		receivers:           make(map[string]*Receiver),
		simulcastReceivers:  make(map[string]*SimulcastReceiver),
		simulcastForwarders: make(map[string]*SimulcastForwarder),
		subscribers:         make(map[*Subscriber]struct{}),
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
		"peerId":    r.id,
		"trackId":   receiver.TrackID(),
		"streamId":  receiver.StreamID(),
		"kind":      receiver.Kind().String(),
		"simulcast": false,
	})
}

// AddSimulcastReceiver adds a new simulcast receiver
func (r *Router) AddSimulcastReceiver(simulcastRecv *SimulcastReceiver) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}

	trackID := simulcastRecv.TrackID()
	r.simulcastReceivers[trackID] = simulcastRecv

	// Create forwarder for this simulcast track
	forwarder := NewSimulcastForwarder(simulcastRecv)
	r.simulcastForwarders[trackID] = forwarder

	// Copy subscribers to update outside the lock
	subscribers := make([]*Subscriber, 0, len(r.subscribers))
	for sub := range r.subscribers {
		subscribers = append(subscribers, sub)
	}
	r.mu.Unlock()

	// Add simulcast downtrack to all existing subscribers
	for _, sub := range subscribers {
		log.Printf("[Router] Adding simulcast track %s to existing subscriber", trackID)
		if err := sub.AddSimulcastDownTrack(simulcastRecv); err != nil {
			log.Printf("[Router] Error adding simulcast downtrack: %v", err)
			continue
		}
		if err := sub.Negotiate(); err != nil {
			log.Printf("[Router] Error negotiating: %v", err)
		}
	}

	// Notify other peers about the new simulcast track
	r.session.Broadcast(r.id, "trackAdded", map[string]interface{}{
		"peerId":    r.id,
		"trackId":   trackID,
		"streamId":  simulcastRecv.StreamID(),
		"kind":      simulcastRecv.Kind().String(),
		"simulcast": true,
	})
}

// GetSimulcastReceiver returns a simulcast receiver by track ID
func (r *Router) GetSimulcastReceiver(trackID string) (*SimulcastReceiver, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	recv, ok := r.simulcastReceivers[trackID]
	return recv, ok
}

// GetSimulcastReceivers returns all simulcast receivers
func (r *Router) GetSimulcastReceivers() map[string]*SimulcastReceiver {
	r.mu.RLock()
	defer r.mu.RUnlock()

	receivers := make(map[string]*SimulcastReceiver, len(r.simulcastReceivers))
	for k, v := range r.simulcastReceivers {
		receivers[k] = v
	}
	return receivers
}

// ForwardSimulcastRTP forwards an RTP packet from a simulcast layer
func (r *Router) ForwardSimulcastRTP(trackID string, packet *rtp.Packet, rid string) {
	r.mu.RLock()
	forwarder, ok := r.simulcastForwarders[trackID]
	r.mu.RUnlock()

	if !ok {
		return
	}

	forwarder.ForwardRTP(packet, rid)
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

	// Add regular receivers
	for _, receiver := range r.receivers {
		if err := subscriber.AddDownTrack(receiver); err != nil {
			return err
		}
	}

	// Add simulcast receivers
	for _, simulcastRecv := range r.simulcastReceivers {
		if err := subscriber.AddSimulcastDownTrack(simulcastRecv); err != nil {
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

// SetSubscriberLayer sets the target layer for a subscriber's simulcast track
func (r *Router) SetSubscriberLayer(subscriber *Subscriber, trackID, layer string) {
	r.mu.RLock()
	forwarder, ok := r.simulcastForwarders[trackID]
	r.mu.RUnlock()

	if !ok {
		return
	}

	// Find the subscriber's downtrack and set the layer
	_ = forwarder // Layer setting is done through the subscriber
	subscriber.SetSimulcastLayer(trackID, layer)
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

	simulcastReceivers := make([]*SimulcastReceiver, 0, len(r.simulcastReceivers))
	for _, recv := range r.simulcastReceivers {
		simulcastReceivers = append(simulcastReceivers, recv)
	}

	forwarders := make([]*SimulcastForwarder, 0, len(r.simulcastForwarders))
	for _, fwd := range r.simulcastForwarders {
		forwarders = append(forwarders, fwd)
	}
	r.mu.Unlock()

	for _, receiver := range receivers {
		receiver.Close()
	}

	for _, recv := range simulcastReceivers {
		recv.Close()
	}

	for _, fwd := range forwarders {
		fwd.Close()
	}

	return nil
}
