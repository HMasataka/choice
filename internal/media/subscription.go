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

	// SVCEnabled indicates if SVC is used instead of simulcast.
	// Per design.md section 4.1: SVC is optional for VP9/AV1 codecs.
	SVCEnabled bool

	// PreferredSVCLayer is the preferred SVC layer (if SVC is enabled).
	// Represented as "S<n>T<n>" string (e.g., "S2T2").
	PreferredSVCLayer string

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
	if s.SVCEnabled {
		if s.PreferredLayer != "" {
			return fmt.Errorf("cannot set both SVC and simulcast layer preference")
		}
		if s.PreferredSVCLayer == "" {
			return fmt.Errorf("SVC enabled but preferred SVC layer is empty")
		}
		if err := validateSVCLayerFormat(s.PreferredSVCLayer); err != nil {
			return err
		}
	} else if s.PreferredSVCLayer != "" {
		return fmt.Errorf("preferred SVC layer set but SVC is disabled")
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

// NewSVCSubscription creates a new Subscription with SVC enabled.
// preferredSVCLayer should be in "S<n>T<n>" format (e.g., "S2T2").
func NewSVCSubscription(subscriberID, publisherID string, trackID TrackID, preferredSVCLayer string) *Subscription {
	now := time.Now()
	return &Subscription{
		ID:                GenerateSubscriptionID(),
		SubscriberID:      subscriberID,
		PublisherID:       publisherID,
		TrackID:           trackID,
		SVCEnabled:        true,
		PreferredSVCLayer: preferredSVCLayer,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

// UpdatePreferredSVCLayer updates the preferred SVC layer.
// layer should be in "S<n>T<n>" format (e.g., "S2T2").
func (s *Subscription) UpdatePreferredSVCLayer(layer string) error {
	if layer == "" {
		return fmt.Errorf("SVC layer cannot be empty")
	}
	if err := validateSVCLayerFormat(layer); err != nil {
		return err
	}
	s.PreferredSVCLayer = layer
	s.UpdatedAt = time.Now()
	return nil
}

// IsSVCEnabled returns true if this subscription uses SVC.
func (s *Subscription) IsSVCEnabled() bool {
	return s.SVCEnabled
}

// GetPreferredSVCLayer returns the preferred SVC layer string.
func (s *Subscription) GetPreferredSVCLayer() string {
	return s.PreferredSVCLayer
}

// SubscribeOptions contains options for subscribing to a track.
type SubscribeOptions struct {
	// PreferredLayer is the preferred simulcast layer.
	// Per requirements.md: h (high), m (medium), l (low)
	// If empty, defaults to high (h).
	PreferredLayer SimulcastLayer

	// SVCEnabled indicates if SVC should be used instead of simulcast.
	// Per design.md section 4.1: SVC is optional for VP9/AV1 codecs.
	SVCEnabled bool

	// PreferredSVCLayer is the preferred SVC layer (if SVCEnabled is true).
	// Should be in "S<n>T<n>" format (e.g., "S2T2").
	// If empty and SVCEnabled is true, defaults to highest layer (S2T2).
	PreferredSVCLayer string
}

// Validate validates the SubscribeOptions.
func (opts *SubscribeOptions) Validate() error {
	if opts == nil {
		return nil
	}
	// SVC and Simulcast are mutually exclusive
	if opts.SVCEnabled && opts.PreferredLayer != "" {
		return fmt.Errorf("cannot set both SVC and simulcast layer preference")
	}
	if !opts.SVCEnabled && opts.PreferredSVCLayer != "" {
		return fmt.Errorf("preferred SVC layer set but SVC is disabled")
	}
	// PreferredLayer validation (allow empty for default)
	if opts.PreferredLayer != "" {
		if err := opts.PreferredLayer.Validate(); err != nil {
			return fmt.Errorf("invalid preferred layer: %w", err)
		}
	}
	// PreferredSVCLayer validation (allow empty for default)
	if opts.PreferredSVCLayer != "" {
		if err := validateSVCLayerFormat(opts.PreferredSVCLayer); err != nil {
			return err
		}
	}
	return nil
}

func validateSVCLayerFormat(layer string) error {
	if len(layer) != 4 || layer[0] != 'S' || layer[2] != 'T' {
		return fmt.Errorf("invalid SVC layer format: %s (expected S<n>T<n>)", layer)
	}
	if layer[1] < '0' || layer[1] > '9' || layer[3] < '0' || layer[3] > '9' {
		return fmt.Errorf("invalid SVC layer format: %s (expected S<n>T<n>)", layer)
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

// GetPreferredSVCLayerOrDefault returns the preferred SVC layer or default (S2T2).
func (opts *SubscribeOptions) GetPreferredSVCLayerOrDefault() string {
	if opts == nil || opts.PreferredSVCLayer == "" {
		return "S2T2" // Default to highest layer
	}
	return opts.PreferredSVCLayer
}

// IsSVCEnabled returns true if SVC is enabled in the options.
func (opts *SubscribeOptions) IsSVCEnabled() bool {
	if opts == nil {
		return false
	}
	return opts.SVCEnabled
}

// SubscriptionInfo contains information about a subscription for serialization.
// This is used in signaling responses.
type SubscriptionInfo struct {
	ID                string `json:"id"`
	SubscriberID      string `json:"subscriberId"`
	PublisherID       string `json:"publisherId"`
	TrackID           string `json:"trackId"`
	PreferredLayer    string `json:"preferredLayer,omitempty"`
	SVCEnabled        bool   `json:"svcEnabled,omitempty"`
	PreferredSVCLayer string `json:"preferredSvcLayer,omitempty"`
}

// ToSubscriptionInfo converts Subscription to SubscriptionInfo for serialization.
func (s *Subscription) ToSubscriptionInfo() *SubscriptionInfo {
	info := &SubscriptionInfo{
		ID:           s.ID.String(),
		SubscriberID: s.SubscriberID,
		PublisherID:  s.PublisherID,
		TrackID:      s.TrackID.String(),
	}

	if s.SVCEnabled {
		info.SVCEnabled = true
		info.PreferredSVCLayer = s.PreferredSVCLayer
	} else {
		info.PreferredLayer = s.PreferredLayer.String()
	}

	return info
}
