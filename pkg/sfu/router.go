package sfu

import (
	"log"
	"sync"

	"github.com/pion/rtp"
)

// Router manages media routing from a single publisher to multiple subscribers.
type Router struct {
	id                  string
	session             *Session
	simulcastReceivers  map[string]*SimulcastReceiver
	simulcastForwarders map[string]*SimulcastForwarder
	subscribers         map[*Subscriber]struct{}
	mu                  sync.RWMutex
	closed              bool
}

// NewRouter creates a new router for a publisher.
func NewRouter(id string, session *Session) *Router {
	return &Router{
		id:                  id,
		session:             session,
		simulcastReceivers:  make(map[string]*SimulcastReceiver),
		simulcastForwarders: make(map[string]*SimulcastForwarder),
		subscribers:         make(map[*Subscriber]struct{}),
	}
}

// ID returns the router identifier (same as the publisher's peer ID).
func (r *Router) ID() string {
	return r.id
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

		// Register with forwarder
		if dt := sub.GetSimulcastDownTrack(trackID); dt != nil {
			forwarder.AddDownTrack(dt)
			log.Printf("[Router] Registered downtrack with forwarder for track %s", trackID)
		}

		if err := sub.Negotiate(); err != nil {
			log.Printf("[Router] Error negotiating: %v", err)
		}
	}

	// Notify other peers about the new simulcast track
	r.session.Broadcast(r.id, "trackAdded", map[string]interface{}{
		"peerId":   r.id,
		"trackId":  trackID,
		"streamId": simulcastRecv.StreamID(),
		"kind":     simulcastRecv.Kind().String(),
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

// Subscribe adds a subscriber and connects all current receivers to it.
func (r *Router) Subscribe(subscriber *Subscriber) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.subscribers[subscriber] = struct{}{}

	// Add simulcast receivers and register with forwarders
	for trackID, simulcastRecv := range r.simulcastReceivers {
		if err := subscriber.AddSimulcastDownTrack(simulcastRecv); err != nil {
			return err
		}

		// Register the downtrack with the forwarder
		if forwarder, ok := r.simulcastForwarders[trackID]; ok {
			if dt := subscriber.GetSimulcastDownTrack(trackID); dt != nil {
				forwarder.AddDownTrack(dt)
				log.Printf("[Router] Registered downtrack with forwarder for track %s", trackID)
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

// SetSubscriberLayer sets the target layer for a subscriber's simulcast track
func (r *Router) SetSubscriberLayer(subscriber *Subscriber, trackID, layer string) {
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

	simulcastReceivers := make([]*SimulcastReceiver, 0, len(r.simulcastReceivers))
	for _, recv := range r.simulcastReceivers {
		simulcastReceivers = append(simulcastReceivers, recv)
	}

	forwarders := make([]*SimulcastForwarder, 0, len(r.simulcastForwarders))
	for _, fwd := range r.simulcastForwarders {
		forwarders = append(forwarders, fwd)
	}
	r.mu.Unlock()

	for _, recv := range simulcastReceivers {
		recv.Close()
	}

	for _, fwd := range forwarders {
		fwd.Close()
	}

	return nil
}
