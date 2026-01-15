package media

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// SubscriptionID is a unique identifier for a subscription.
// Generated server-side using UUID v4.
type SubscriptionID string

// GenerateSubscriptionID generates a new unique subscription ID.
func GenerateSubscriptionID() SubscriptionID {
	return SubscriptionID(uuid.New().String())
}

// String returns the string representation of SubscriptionID.
func (id SubscriptionID) String() string {
	return string(id)
}

// Validate checks if the SubscriptionID is valid (non-empty and valid UUID v4 format).
func (id SubscriptionID) Validate() error {
	if id == "" {
		return fmt.Errorf("subscription ID cannot be empty")
	}
	// Validate UUID format
	parsed, err := uuid.Parse(string(id))
	if err != nil {
		return fmt.Errorf("subscription ID must be a valid UUID: %w", err)
	}
	// Ensure UUID version 4 (per requirements.md)
	if parsed.Version() != 4 {
		return fmt.Errorf("subscription ID must be UUID v4, got v%d", parsed.Version())
	}
	return nil
}

// Subscription represents a subscriber's subscription to a publisher's track.
// This manages the relationship between a subscriber and a published track.
type Subscription struct {
	// ID is the unique identifier for this subscription (server-generated).
	ID SubscriptionID

	// SubscriberID is the ID of the participant who is subscribing.
	SubscriberID string

	// PublisherID is the ID of the participant who published the track.
	PublisherID string

	// TrackID is the ID of the track being subscribed to.
	TrackID TrackID

	// PreferredLayer is the preferred simulcast layer (if simulcast is enabled).
	// Per requirements.md: h (high), m (medium), l (low)
	PreferredLayer SimulcastLayer

	// CreatedAt is the time when this subscription was created.
	CreatedAt time.Time

	// UpdatedAt is the time when this subscription was last updated.
	UpdatedAt time.Time
}

// NewSubscription creates a new Subscription.
func NewSubscription(subscriberID, publisherID string, trackID TrackID, preferredLayer SimulcastLayer) *Subscription {
	now := time.Now()
	return &Subscription{
		ID:             GenerateSubscriptionID(),
		SubscriberID:   subscriberID,
		PublisherID:    publisherID,
		TrackID:        trackID,
		PreferredLayer: preferredLayer,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// Validate validates the Subscription.
func (s *Subscription) Validate() error {
	if err := s.ID.Validate(); err != nil {
		return fmt.Errorf("invalid subscription ID: %w", err)
	}
	if s.SubscriberID == "" {
		return fmt.Errorf("subscriber ID cannot be empty")
	}
	if s.PublisherID == "" {
		return fmt.Errorf("publisher ID cannot be empty")
	}
	if err := s.TrackID.Validate(); err != nil {
		return fmt.Errorf("invalid track ID: %w", err)
	}
	// PreferredLayer validation (allow empty for default)
	if s.PreferredLayer != "" {
		if err := s.PreferredLayer.Validate(); err != nil {
			return fmt.Errorf("invalid preferred layer: %w", err)
		}
	}
	return nil
}

// UpdatePreferredLayer updates the preferred simulcast layer.
func (s *Subscription) UpdatePreferredLayer(layer SimulcastLayer) error {
	if err := layer.Validate(); err != nil {
		return fmt.Errorf("invalid simulcast layer: %w", err)
	}
	s.PreferredLayer = layer
	s.UpdatedAt = time.Now()
	return nil
}

// SubscribeOptions contains options for subscribing to a track.
type SubscribeOptions struct {
	// PreferredLayer is the preferred simulcast layer.
	// Per requirements.md: h (high), m (medium), l (low)
	// If empty, defaults to high (h).
	PreferredLayer SimulcastLayer
}

// Validate validates the SubscribeOptions.
func (opts *SubscribeOptions) Validate() error {
	if opts == nil {
		return nil
	}
	// PreferredLayer validation (allow empty for default)
	if opts.PreferredLayer != "" {
		if err := opts.PreferredLayer.Validate(); err != nil {
			return fmt.Errorf("invalid preferred layer: %w", err)
		}
	}
	return nil
}

// GetPreferredLayerOrDefault returns the preferred layer or default (high).
func (opts *SubscribeOptions) GetPreferredLayerOrDefault() SimulcastLayer {
	if opts == nil || opts.PreferredLayer == "" {
		return SimulcastLayerHigh
	}
	return opts.PreferredLayer
}

// SubscriptionInfo contains information about a subscription for serialization.
// This is used in signaling responses.
type SubscriptionInfo struct {
	ID             string `json:"id"`
	SubscriberID   string `json:"subscriberId"`
	PublisherID    string `json:"publisherId"`
	TrackID        string `json:"trackId"`
	PreferredLayer string `json:"preferredLayer,omitempty"`
}

// ToSubscriptionInfo converts Subscription to SubscriptionInfo for serialization.
func (s *Subscription) ToSubscriptionInfo() *SubscriptionInfo {
	return &SubscriptionInfo{
		ID:             s.ID.String(),
		SubscriberID:   s.SubscriberID,
		PublisherID:    s.PublisherID,
		TrackID:        s.TrackID.String(),
		PreferredLayer: s.PreferredLayer.String(),
	}
}
