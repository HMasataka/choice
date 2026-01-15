package media

import (
	"context"
	"fmt"
	"sync"
)

// MediaRouter handles media stream routing between participants.
// Per design.md: MediaRouter interface defines the core methods.
// This implementation provides basic track registry and subscription management
// without actual RTP packet forwarding (handled in later tasks).
type MediaRouter interface {
	// AddTrack adds a new published track to the router.
	// Returns an error if the track is invalid or already exists.
	AddTrack(ctx context.Context, track *LocalTrack) error

	// RemoveTrack removes a published track from the router.
	// Also removes all subscriptions to this track.
	// Returns an error if the track doesn't exist.
	RemoveTrack(ctx context.Context, trackID TrackID) error

	// Subscribe creates a subscription to a track.
	// Returns the new Subscription or an error if the track doesn't exist.
	Subscribe(ctx context.Context, subscriberID string, trackID TrackID, opts *SubscribeOptions) (*Subscription, error)

	// Unsubscribe removes a subscription.
	// Returns an error if the subscription doesn't exist.
	Unsubscribe(ctx context.Context, subscriptionID SubscriptionID) error

	// GetTrack retrieves track information by ID.
	// Returns an error if the track doesn't exist.
	// Note: Returns a pointer to the internal LocalTrack. Treat as read-only.
	// Use LocalTrack methods for safe modifications.
	GetTrack(ctx context.Context, trackID TrackID) (*LocalTrack, error)

	// ListTracks lists all tracks in the router.
	// Note: Returns pointers to internal LocalTrack objects. Treat as read-only.
	// Use LocalTrack methods for safe modifications.
	ListTracks(ctx context.Context) ([]*LocalTrack, error)
}

// mediaRouter is the concrete implementation of MediaRouter.
type mediaRouter struct {
	// mu protects all maps for concurrent access.
	// Per Codex recommendation: single RWMutex for entire router.
	mu sync.RWMutex

	// tracks is the main track registry: trackID → LocalTrack
	tracks map[TrackID]*LocalTrack

	// tracksByPublisher is a helper index: publisherID → set of trackIDs
	// Makes it easy to list tracks by publisher.
	tracksByPublisher map[string]map[TrackID]struct{}

	// subscriptions is the main subscription registry: subscriptionID → Subscription
	subscriptions map[SubscriptionID]*Subscription

	// subsByTrack is a helper index: trackID → set of subscriptionIDs
	// Makes it easy to remove all subscriptions when a track is unpublished.
	subsByTrack map[TrackID]map[SubscriptionID]struct{}

	// subsBySubscriber is a helper index: subscriberID → set of subscriptionIDs
	// Makes it easy to list subscriptions by subscriber.
	subsBySubscriber map[string]map[SubscriptionID]struct{}
}

// NewMediaRouter creates a new MediaRouter instance.
func NewMediaRouter() MediaRouter {
	return &mediaRouter{
		tracks:            make(map[TrackID]*LocalTrack),
		tracksByPublisher: make(map[string]map[TrackID]struct{}),
		subscriptions:     make(map[SubscriptionID]*Subscription),
		subsByTrack:       make(map[TrackID]map[SubscriptionID]struct{}),
		subsBySubscriber:  make(map[string]map[SubscriptionID]struct{}),
	}
}

// AddTrack adds a new published track to the router.
func (r *mediaRouter) AddTrack(ctx context.Context, track *LocalTrack) error {
	if track == nil {
		return fmt.Errorf("track cannot be nil")
	}

	// Validate track (includes TrackID UUID v4 validation)
	if err := track.Validate(); err != nil {
		return fmt.Errorf("invalid track: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if track already exists
	if _, exists := r.tracks[track.ID]; exists {
		return fmt.Errorf("track %s already exists", track.ID)
	}

	// Add to main registry
	r.tracks[track.ID] = track

	// Add to publisher index
	if r.tracksByPublisher[track.PublisherID] == nil {
		r.tracksByPublisher[track.PublisherID] = make(map[TrackID]struct{})
	}
	r.tracksByPublisher[track.PublisherID][track.ID] = struct{}{}

	return nil
}

// RemoveTrack removes a published track from the router.
// Also removes all subscriptions to this track.
func (r *mediaRouter) RemoveTrack(ctx context.Context, trackID TrackID) error {
	if err := trackID.Validate(); err != nil {
		return fmt.Errorf("invalid track ID: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if track exists
	track, exists := r.tracks[trackID]
	if !exists {
		return fmt.Errorf("track %s not found", trackID)
	}

	// Remove all subscriptions to this track
	if subIDs, ok := r.subsByTrack[trackID]; ok {
		for subID := range subIDs {
			// Remove from main subscription registry
			if sub, exists := r.subscriptions[subID]; exists {
				// Remove from subscriber index
				if subSet, ok := r.subsBySubscriber[sub.SubscriberID]; ok {
					delete(subSet, subID)
					if len(subSet) == 0 {
						delete(r.subsBySubscriber, sub.SubscriberID)
					}
				}
				delete(r.subscriptions, subID)
			}
		}
		delete(r.subsByTrack, trackID)
	}

	// Remove from publisher index
	if trackSet, ok := r.tracksByPublisher[track.PublisherID]; ok {
		delete(trackSet, trackID)
		if len(trackSet) == 0 {
			delete(r.tracksByPublisher, track.PublisherID)
		}
	}

	// Remove from main registry
	delete(r.tracks, trackID)

	return nil
}

// Subscribe creates a subscription to a track.
func (r *mediaRouter) Subscribe(ctx context.Context, subscriberID string, trackID TrackID, opts *SubscribeOptions) (*Subscription, error) {
	if subscriberID == "" {
		return nil, fmt.Errorf("subscriber ID cannot be empty")
	}
	if err := trackID.Validate(); err != nil {
		return nil, fmt.Errorf("invalid track ID: %w", err)
	}
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid subscribe options: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if track exists
	track, exists := r.tracks[trackID]
	if !exists {
		return nil, fmt.Errorf("track %s not found", trackID)
	}

	// Get preferred layer or default
	preferredLayer := opts.GetPreferredLayerOrDefault()

	// Validate preferred layer against track capabilities
	if preferredLayer != "" {
		// If the track is simulcast, validate that the requested layer is available
		if track.IsSimulcast() {
			layers := track.GetLayers()
			if len(layers) == 0 {
				return nil, fmt.Errorf("track %s is simulcast but has no layers specified", trackID)
			}
			// Check if the requested layer is available
			layerFound := false
			for _, layer := range layers {
				if layer == preferredLayer {
					layerFound = true
					break
				}
			}
			if !layerFound {
				return nil, fmt.Errorf("track %s does not have layer %s, available layers: %v", trackID, preferredLayer, layers)
			}
		} else {
			// Non-simulcast tracks only support high quality (default)
			if preferredLayer != SimulcastLayerHigh {
				return nil, fmt.Errorf("track %s does not support simulcast, only high quality is available", trackID)
			}
		}
	}

	// Create new subscription
	sub := NewSubscription(subscriberID, track.PublisherID, trackID, preferredLayer)

	// Validate subscription
	if err := sub.Validate(); err != nil {
		return nil, fmt.Errorf("invalid subscription: %w", err)
	}

	// Add to main subscription registry
	r.subscriptions[sub.ID] = sub

	// Add to track index
	if r.subsByTrack[trackID] == nil {
		r.subsByTrack[trackID] = make(map[SubscriptionID]struct{})
	}
	r.subsByTrack[trackID][sub.ID] = struct{}{}

	// Add to subscriber index
	if r.subsBySubscriber[subscriberID] == nil {
		r.subsBySubscriber[subscriberID] = make(map[SubscriptionID]struct{})
	}
	r.subsBySubscriber[subscriberID][sub.ID] = struct{}{}

	// Return a copy to prevent external modifications
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
func (r *mediaRouter) Unsubscribe(ctx context.Context, subscriptionID SubscriptionID) error {
	if err := subscriptionID.Validate(); err != nil {
		return fmt.Errorf("invalid subscription ID: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if subscription exists
	sub, exists := r.subscriptions[subscriptionID]
	if !exists {
		return fmt.Errorf("subscription %s not found", subscriptionID)
	}

	// Remove from track index
	if subSet, ok := r.subsByTrack[sub.TrackID]; ok {
		delete(subSet, subscriptionID)
		if len(subSet) == 0 {
			delete(r.subsByTrack, sub.TrackID)
		}
	}

	// Remove from subscriber index
	if subSet, ok := r.subsBySubscriber[sub.SubscriberID]; ok {
		delete(subSet, subscriptionID)
		if len(subSet) == 0 {
			delete(r.subsBySubscriber, sub.SubscriberID)
		}
	}

	// Remove from main registry
	delete(r.subscriptions, subscriptionID)

	return nil
}

// GetTrack retrieves track information by ID.
// Returns a pointer to the internal LocalTrack. Treat as read-only.
func (r *mediaRouter) GetTrack(ctx context.Context, trackID TrackID) (*LocalTrack, error) {
	if err := trackID.Validate(); err != nil {
		return nil, fmt.Errorf("invalid track ID: %w", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	track, exists := r.tracks[trackID]
	if !exists {
		return nil, fmt.Errorf("track %s not found", trackID)
	}

	// Return the track (LocalTrack's metadata access is already thread-safe)
	return track, nil
}

// ListTracks lists all tracks in the router.
// Returns pointers to internal LocalTrack objects. Treat as read-only.
func (r *mediaRouter) ListTracks(ctx context.Context) ([]*LocalTrack, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Create a slice of track pointers
	tracks := make([]*LocalTrack, 0, len(r.tracks))
	for _, track := range r.tracks {
		tracks = append(tracks, track)
	}

	return tracks, nil
}

// GetSubscription retrieves a subscription by ID (internal helper method).
// Not part of the MediaRouter interface but useful for testing and internal use.
func (r *mediaRouter) GetSubscription(ctx context.Context, subscriptionID SubscriptionID) (*Subscription, error) {
	if err := subscriptionID.Validate(); err != nil {
		return nil, fmt.Errorf("invalid subscription ID: %w", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	sub, exists := r.subscriptions[subscriptionID]
	if !exists {
		return nil, fmt.Errorf("subscription %s not found", subscriptionID)
	}

	// Return a copy to prevent external modifications
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

// ListSubscriptionsByTrack lists all subscriptions for a specific track (internal helper method).
func (r *mediaRouter) ListSubscriptionsByTrack(ctx context.Context, trackID TrackID) ([]*Subscription, error) {
	if err := trackID.Validate(); err != nil {
		return nil, fmt.Errorf("invalid track ID: %w", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	subIDs, exists := r.subsByTrack[trackID]
	if !exists {
		return []*Subscription{}, nil
	}

	// Create a copy of the subscription list
	subs := make([]*Subscription, 0, len(subIDs))
	for subID := range subIDs {
		if sub, ok := r.subscriptions[subID]; ok {
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
	}

	return subs, nil
}

// ListSubscriptionsBySubscriber lists all subscriptions for a specific subscriber (internal helper method).
func (r *mediaRouter) ListSubscriptionsBySubscriber(ctx context.Context, subscriberID string) ([]*Subscription, error) {
	if subscriberID == "" {
		return nil, fmt.Errorf("subscriber ID cannot be empty")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	subIDs, exists := r.subsBySubscriber[subscriberID]
	if !exists {
		return []*Subscription{}, nil
	}

	// Create a copy of the subscription list
	subs := make([]*Subscription, 0, len(subIDs))
	for subID := range subIDs {
		if sub, ok := r.subscriptions[subID]; ok {
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
	}

	return subs, nil
}
