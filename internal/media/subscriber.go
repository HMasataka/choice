package media

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Subscriber manages subscriptions for a participant.
// Per tasks.md 3.4.3: Subscriber with layer selection.
type Subscriber struct {
	mu sync.RWMutex

	// ID is the participant ID of the subscriber.
	ID string

	// subscriptions are the active subscriptions for this subscriber.
	// Key is SubscriptionID.
	subscriptions map[SubscriptionID]*Subscription

	// subscriptionsByTrack maps TrackID to SubscriptionID for quick lookup.
	subscriptionsByTrack map[TrackID]SubscriptionID

	// createdAt is when the subscriber was created.
	createdAt time.Time

	// updatedAt is when the subscriber was last updated.
	updatedAt time.Time

	// onSubscriptionAdded is called when a subscription is added.
	onSubscriptionAdded func(sub *Subscription)

	// onSubscriptionRemoved is called when a subscription is removed.
	onSubscriptionRemoved func(subID SubscriptionID)

	// onLayerChanged is called when a subscription's layer changes.
	onLayerChanged func(subID SubscriptionID, previousLayer, newLayer SimulcastLayer)
}

// NewSubscriber creates a new Subscriber.
func NewSubscriber(id string) *Subscriber {
	now := time.Now()
	return &Subscriber{
		ID:                   id,
		subscriptions:        make(map[SubscriptionID]*Subscription),
		subscriptionsByTrack: make(map[TrackID]SubscriptionID),
		createdAt:            now,
		updatedAt:            now,
	}
}

// Subscribe creates a subscription to a track.
func (s *Subscriber) Subscribe(ctx context.Context, publisherID string, trackID TrackID, preferredLayer SimulcastLayer) (*Subscription, error) {
	if publisherID == "" {
		return nil, fmt.Errorf("publisher ID cannot be empty")
	}
	if err := trackID.Validate(); err != nil {
		return nil, fmt.Errorf("invalid track ID: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already subscribed to this track
	if existingSubID, exists := s.subscriptionsByTrack[trackID]; exists {
		return nil, fmt.Errorf("already subscribed to track %s with subscription %s", trackID, existingSubID)
	}

	// Create new subscription
	sub := NewSubscription(s.ID, publisherID, trackID, preferredLayer)
	if err := sub.Validate(); err != nil {
		return nil, fmt.Errorf("invalid subscription: %w", err)
	}

	// Store subscription
	s.subscriptions[sub.ID] = sub
	s.subscriptionsByTrack[trackID] = sub.ID
	s.updatedAt = time.Now()

	// Capture callback before releasing lock
	onSubscriptionAdded := s.onSubscriptionAdded
	if onSubscriptionAdded != nil {
		// Return a copy to the callback
		subCopy := &Subscription{
			ID:             sub.ID,
			SubscriberID:   sub.SubscriberID,
			PublisherID:    sub.PublisherID,
			TrackID:        sub.TrackID,
			PreferredLayer: sub.PreferredLayer,
			CreatedAt:      sub.CreatedAt,
			UpdatedAt:      sub.UpdatedAt,
		}
		go onSubscriptionAdded(subCopy)
	}

	// Return a copy
	return &Subscription{
		ID:             sub.ID,
		SubscriberID:   sub.SubscriberID,
		PublisherID:    sub.PublisherID,
		TrackID:        sub.TrackID,
		PreferredLayer: sub.PreferredLayer,
		CreatedAt:      sub.CreatedAt,
		UpdatedAt:      sub.UpdatedAt,
	}, nil
}

// Unsubscribe removes a subscription.
func (s *Subscriber) Unsubscribe(ctx context.Context, subscriptionID SubscriptionID) error {
	if err := subscriptionID.Validate(); err != nil {
		return fmt.Errorf("invalid subscription ID: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sub, exists := s.subscriptions[subscriptionID]
	if !exists {
		return fmt.Errorf("subscription %s not found", subscriptionID)
	}

	// Remove from both maps
	delete(s.subscriptions, subscriptionID)
	delete(s.subscriptionsByTrack, sub.TrackID)
	s.updatedAt = time.Now()

	// Capture callback before releasing lock
	onSubscriptionRemoved := s.onSubscriptionRemoved
	if onSubscriptionRemoved != nil {
		go onSubscriptionRemoved(subscriptionID)
	}

	return nil
}

// UnsubscribeByTrack removes the subscription for a specific track.
func (s *Subscriber) UnsubscribeByTrack(ctx context.Context, trackID TrackID) error {
	if err := trackID.Validate(); err != nil {
		return fmt.Errorf("invalid track ID: %w", err)
	}

	s.mu.Lock()

	subID, exists := s.subscriptionsByTrack[trackID]
	if !exists {
		s.mu.Unlock()
		return fmt.Errorf("no subscription for track %s", trackID)
	}

	// Remove from both maps
	delete(s.subscriptions, subID)
	delete(s.subscriptionsByTrack, trackID)
	s.updatedAt = time.Now()

	// Capture callback before releasing lock
	onSubscriptionRemoved := s.onSubscriptionRemoved
	s.mu.Unlock()

	if onSubscriptionRemoved != nil {
		go onSubscriptionRemoved(subID)
	}

	return nil
}

// SetPreferredLayer sets the preferred layer for a subscription.
func (s *Subscriber) SetPreferredLayer(ctx context.Context, subscriptionID SubscriptionID, layer SimulcastLayer) error {
	if err := subscriptionID.Validate(); err != nil {
		return fmt.Errorf("invalid subscription ID: %w", err)
	}
	if err := layer.Validate(); err != nil {
		return fmt.Errorf("invalid layer: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sub, exists := s.subscriptions[subscriptionID]
	if !exists {
		return fmt.Errorf("subscription %s not found", subscriptionID)
	}

	previousLayer := sub.PreferredLayer
	sub.PreferredLayer = layer
	sub.UpdatedAt = time.Now()
	s.updatedAt = time.Now()

	// Notify layer change
	if previousLayer != layer {
		onLayerChanged := s.onLayerChanged
		if onLayerChanged != nil {
			go onLayerChanged(subscriptionID, previousLayer, layer)
		}
	}

	return nil
}

// GetSubscription retrieves a subscription by ID.
func (s *Subscriber) GetSubscription(subscriptionID SubscriptionID) (*Subscription, error) {
	if err := subscriptionID.Validate(); err != nil {
		return nil, fmt.Errorf("invalid subscription ID: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	sub, exists := s.subscriptions[subscriptionID]
	if !exists {
		return nil, fmt.Errorf("subscription %s not found", subscriptionID)
	}

	// Return a copy
	return &Subscription{
		ID:             sub.ID,
		SubscriberID:   sub.SubscriberID,
		PublisherID:    sub.PublisherID,
		TrackID:        sub.TrackID,
		PreferredLayer: sub.PreferredLayer,
		CreatedAt:      sub.CreatedAt,
		UpdatedAt:      sub.UpdatedAt,
	}, nil
}

// GetSubscriptionByTrack retrieves the subscription for a specific track.
func (s *Subscriber) GetSubscriptionByTrack(trackID TrackID) (*Subscription, error) {
	if err := trackID.Validate(); err != nil {
		return nil, fmt.Errorf("invalid track ID: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	subID, exists := s.subscriptionsByTrack[trackID]
	if !exists {
		return nil, fmt.Errorf("no subscription for track %s", trackID)
	}

	sub := s.subscriptions[subID]
	// Return a copy
	return &Subscription{
		ID:             sub.ID,
		SubscriberID:   sub.SubscriberID,
		PublisherID:    sub.PublisherID,
		TrackID:        sub.TrackID,
		PreferredLayer: sub.PreferredLayer,
		CreatedAt:      sub.CreatedAt,
		UpdatedAt:      sub.UpdatedAt,
	}, nil
}

// ListSubscriptions returns all subscriptions for this subscriber.
func (s *Subscriber) ListSubscriptions() []*Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()

	subs := make([]*Subscription, 0, len(s.subscriptions))
	for _, sub := range s.subscriptions {
		subs = append(subs, &Subscription{
			ID:             sub.ID,
			SubscriberID:   sub.SubscriberID,
			PublisherID:    sub.PublisherID,
			TrackID:        sub.TrackID,
			PreferredLayer: sub.PreferredLayer,
			CreatedAt:      sub.CreatedAt,
			UpdatedAt:      sub.UpdatedAt,
		})
	}
	return subs
}

// IsSubscribedTo checks if subscribed to a specific track.
func (s *Subscriber) IsSubscribedTo(trackID TrackID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.subscriptionsByTrack[trackID]
	return exists
}

// SubscriptionCount returns the number of subscriptions.
func (s *Subscriber) SubscriptionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.subscriptions)
}

// SetOnSubscriptionAdded sets the callback for subscription added events.
func (s *Subscriber) SetOnSubscriptionAdded(cb func(sub *Subscription)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onSubscriptionAdded = cb
}

// SetOnSubscriptionRemoved sets the callback for subscription removed events.
func (s *Subscriber) SetOnSubscriptionRemoved(cb func(subID SubscriptionID)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onSubscriptionRemoved = cb
}

// SetOnLayerChanged sets the callback for layer change events.
func (s *Subscriber) SetOnLayerChanged(cb func(subID SubscriptionID, previousLayer, newLayer SimulcastLayer)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onLayerChanged = cb
}

// GetCreatedAt returns when the subscriber was created.
func (s *Subscriber) GetCreatedAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.createdAt
}

// GetUpdatedAt returns when the subscriber was last updated.
func (s *Subscriber) GetUpdatedAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.updatedAt
}

// SubscriberManager manages multiple subscribers.
type SubscriberManager struct {
	mu          sync.RWMutex
	subscribers map[string]*Subscriber
}

// NewSubscriberManager creates a new SubscriberManager.
func NewSubscriberManager() *SubscriberManager {
	return &SubscriberManager{
		subscribers: make(map[string]*Subscriber),
	}
}

// GetOrCreateSubscriber gets an existing subscriber or creates a new one.
func (m *SubscriberManager) GetOrCreateSubscriber(subscriberID string) *Subscriber {
	m.mu.Lock()
	defer m.mu.Unlock()

	if subscriber, ok := m.subscribers[subscriberID]; ok {
		return subscriber
	}

	subscriber := NewSubscriber(subscriberID)
	m.subscribers[subscriberID] = subscriber
	return subscriber
}

// GetSubscriber gets a subscriber by ID.
func (m *SubscriberManager) GetSubscriber(subscriberID string) (*Subscriber, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	subscriber, ok := m.subscribers[subscriberID]
	return subscriber, ok
}

// RemoveSubscriber removes a subscriber.
func (m *SubscriberManager) RemoveSubscriber(subscriberID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.subscribers, subscriberID)
}

// ListSubscribers returns all subscriber IDs.
func (m *SubscriberManager) ListSubscribers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.subscribers))
	for id := range m.subscribers {
		ids = append(ids, id)
	}
	return ids
}

// SubscriberCount returns the number of subscribers.
func (m *SubscriberManager) SubscriberCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.subscribers)
}

// GetSubscribersToTrack returns all subscribers subscribed to a track.
func (m *SubscriberManager) GetSubscribersToTrack(trackID TrackID) []*Subscriber {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Subscriber
	for _, sub := range m.subscribers {
		if sub.IsSubscribedTo(trackID) {
			result = append(result, sub)
		}
	}
	return result
}
