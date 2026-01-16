package media

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGenerateSubscriptionID(t *testing.T) {
	id1 := GenerateSubscriptionID()
	id2 := GenerateSubscriptionID()

	// IDs should be non-empty
	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)

	// IDs should be unique
	assert.NotEqual(t, id1, id2)

	// IDs should be valid UUIDs
	_, err := uuid.Parse(string(id1))
	assert.NoError(t, err)
	_, err = uuid.Parse(string(id2))
	assert.NoError(t, err)
}

func TestSubscriptionID_String(t *testing.T) {
	id := SubscriptionID("test-id")
	assert.Equal(t, "test-id", id.String())
}

func TestSubscriptionID_Validate(t *testing.T) {
	tests := []struct {
		name    string
		id      SubscriptionID
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid UUID v4",
			id:      GenerateSubscriptionID(),
			wantErr: false,
		},
		{
			name:    "empty ID",
			id:      "",
			wantErr: true,
			errMsg:  "cannot be empty",
		},
		{
			name:    "invalid UUID format",
			id:      SubscriptionID("not-a-uuid"),
			wantErr: true,
			errMsg:  "must be a valid UUID",
		},
		{
			name:    "UUID v1 (not v4)",
			id:      SubscriptionID("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
			wantErr: true,
			errMsg:  "must be UUID v4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.id.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewSubscription(t *testing.T) {
	subscriberID := "subscriber1"
	publisherID := "publisher1"
	trackID := GenerateTrackID()
	preferredLayer := SimulcastLayerHigh

	sub := NewSubscription(subscriberID, publisherID, trackID, preferredLayer)

	assert.NotNil(t, sub)
	assert.NotEmpty(t, sub.ID)
	assert.Equal(t, subscriberID, sub.SubscriberID)
	assert.Equal(t, publisherID, sub.PublisherID)
	assert.Equal(t, trackID, sub.TrackID)
	assert.Equal(t, preferredLayer, sub.PreferredLayer)
	assert.False(t, sub.CreatedAt.IsZero())
	assert.False(t, sub.UpdatedAt.IsZero())
	assert.Equal(t, sub.CreatedAt, sub.UpdatedAt)
}

func TestSubscription_Validate(t *testing.T) {
	validSub := NewSubscription("subscriber1", "publisher1", GenerateTrackID(), SimulcastLayerHigh)

	tests := []struct {
		name    string
		sub     *Subscription
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid subscription",
			sub:     validSub,
			wantErr: false,
		},
		{
			name: "invalid subscription ID",
			sub: &Subscription{
				ID:             "", // Invalid: empty
				SubscriberID:   "subscriber1",
				PublisherID:    "publisher1",
				TrackID:        GenerateTrackID(),
				PreferredLayer: SimulcastLayerHigh,
			},
			wantErr: true,
			errMsg:  "invalid subscription ID",
		},
		{
			name: "empty subscriber ID",
			sub: &Subscription{
				ID:             GenerateSubscriptionID(),
				SubscriberID:   "", // Invalid: empty
				PublisherID:    "publisher1",
				TrackID:        GenerateTrackID(),
				PreferredLayer: SimulcastLayerHigh,
			},
			wantErr: true,
			errMsg:  "subscriber ID cannot be empty",
		},
		{
			name: "empty publisher ID",
			sub: &Subscription{
				ID:             GenerateSubscriptionID(),
				SubscriberID:   "subscriber1",
				PublisherID:    "", // Invalid: empty
				TrackID:        GenerateTrackID(),
				PreferredLayer: SimulcastLayerHigh,
			},
			wantErr: true,
			errMsg:  "publisher ID cannot be empty",
		},
		{
			name: "invalid track ID",
			sub: &Subscription{
				ID:             GenerateSubscriptionID(),
				SubscriberID:   "subscriber1",
				PublisherID:    "publisher1",
				TrackID:        "", // Invalid: empty
				PreferredLayer: SimulcastLayerHigh,
			},
			wantErr: true,
			errMsg:  "invalid track ID",
		},
		{
			name: "invalid preferred layer",
			sub: &Subscription{
				ID:             GenerateSubscriptionID(),
				SubscriberID:   "subscriber1",
				PublisherID:    "publisher1",
				TrackID:        GenerateTrackID(),
				PreferredLayer: "invalid", // Invalid layer
			},
			wantErr: true,
			errMsg:  "invalid preferred layer",
		},
		{
			name: "empty preferred layer (allowed)",
			sub: &Subscription{
				ID:             GenerateSubscriptionID(),
				SubscriberID:   "subscriber1",
				PublisherID:    "publisher1",
				TrackID:        GenerateTrackID(),
				PreferredLayer: "", // Empty is allowed (will use default)
			},
			wantErr: false,
		},
		{
			name: "valid SVC subscription",
			sub: &Subscription{
				ID:                GenerateSubscriptionID(),
				SubscriberID:      "subscriber1",
				PublisherID:       "publisher1",
				TrackID:           GenerateTrackID(),
				SVCEnabled:        true,
				PreferredSVCLayer: "S1T0",
			},
			wantErr: false,
		},
		{
			name: "SVC enabled with empty preferred SVC layer",
			sub: &Subscription{
				ID:           GenerateSubscriptionID(),
				SubscriberID: "subscriber1",
				PublisherID:  "publisher1",
				TrackID:      GenerateTrackID(),
				SVCEnabled:   true,
			},
			wantErr: true,
			errMsg:  "preferred SVC layer is empty",
		},
		{
			name: "SVC enabled with simulcast layer set",
			sub: &Subscription{
				ID:             GenerateSubscriptionID(),
				SubscriberID:   "subscriber1",
				PublisherID:    "publisher1",
				TrackID:        GenerateTrackID(),
				SVCEnabled:     true,
				PreferredLayer: SimulcastLayerHigh,
			},
			wantErr: true,
			errMsg:  "cannot set both SVC and simulcast layer preference",
		},
		{
			name: "SVC layer set while SVC disabled",
			sub: &Subscription{
				ID:                GenerateSubscriptionID(),
				SubscriberID:      "subscriber1",
				PublisherID:       "publisher1",
				TrackID:           GenerateTrackID(),
				PreferredSVCLayer: "S1T0",
			},
			wantErr: true,
			errMsg:  "SVC is disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.sub.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSubscription_UpdatePreferredLayer(t *testing.T) {
	sub := NewSubscription("subscriber1", "publisher1", GenerateTrackID(), SimulcastLayerHigh)
	originalUpdatedAt := sub.UpdatedAt

	// Update to medium layer
	err := sub.UpdatePreferredLayer(SimulcastLayerMedium)
	assert.NoError(t, err)
	assert.Equal(t, SimulcastLayerMedium, sub.PreferredLayer)
	assert.True(t, sub.UpdatedAt.After(originalUpdatedAt))

	// Update to low layer
	err = sub.UpdatePreferredLayer(SimulcastLayerLow)
	assert.NoError(t, err)
	assert.Equal(t, SimulcastLayerLow, sub.PreferredLayer)

	// Try to update with invalid layer
	err = sub.UpdatePreferredLayer("invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid simulcast layer")
	assert.Equal(t, SimulcastLayerLow, sub.PreferredLayer) // Should remain unchanged
}

func TestSubscribeOptions_Validate(t *testing.T) {
	tests := []struct {
		name    string
		opts    *SubscribeOptions
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil options (allowed)",
			opts:    nil,
			wantErr: false,
		},
		{
			name:    "empty options (allowed)",
			opts:    &SubscribeOptions{},
			wantErr: false,
		},
		{
			name: "valid preferred layer",
			opts: &SubscribeOptions{
				PreferredLayer: SimulcastLayerHigh,
			},
			wantErr: false,
		},
		{
			name: "invalid preferred layer",
			opts: &SubscribeOptions{
				PreferredLayer: "invalid",
			},
			wantErr: true,
			errMsg:  "invalid preferred layer",
		},
		{
			name: "valid SVC options without explicit layer",
			opts: &SubscribeOptions{
				SVCEnabled: true,
			},
			wantErr: false,
		},
		{
			name: "valid SVC options with layer",
			opts: &SubscribeOptions{
				SVCEnabled:        true,
				PreferredSVCLayer: "S2T1",
			},
			wantErr: false,
		},
		{
			name: "SVC enabled with simulcast layer set",
			opts: &SubscribeOptions{
				SVCEnabled:     true,
				PreferredLayer: SimulcastLayerHigh,
			},
			wantErr: true,
			errMsg:  "cannot set both SVC and simulcast layer preference",
		},
		{
			name: "SVC layer set while SVC disabled",
			opts: &SubscribeOptions{
				PreferredSVCLayer: "S1T1",
			},
			wantErr: true,
			errMsg:  "SVC is disabled",
		},
		{
			name: "invalid SVC layer format",
			opts: &SubscribeOptions{
				SVCEnabled:        true,
				PreferredSVCLayer: "bad",
			},
			wantErr: true,
			errMsg:  "invalid SVC layer format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSubscribeOptions_GetPreferredLayerOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		opts     *SubscribeOptions
		expected SimulcastLayer
	}{
		{
			name:     "nil options returns default (high)",
			opts:     nil,
			expected: SimulcastLayerHigh,
		},
		{
			name:     "empty layer returns default (high)",
			opts:     &SubscribeOptions{},
			expected: SimulcastLayerHigh,
		},
		{
			name: "specified high layer",
			opts: &SubscribeOptions{
				PreferredLayer: SimulcastLayerHigh,
			},
			expected: SimulcastLayerHigh,
		},
		{
			name: "specified medium layer",
			opts: &SubscribeOptions{
				PreferredLayer: SimulcastLayerMedium,
			},
			expected: SimulcastLayerMedium,
		},
		{
			name: "specified low layer",
			opts: &SubscribeOptions{
				PreferredLayer: SimulcastLayerLow,
			},
			expected: SimulcastLayerLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.opts.GetPreferredLayerOrDefault()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSubscription_ToSubscriptionInfo(t *testing.T) {
	trackID := GenerateTrackID()
	sub := NewSubscription("subscriber1", "publisher1", trackID, SimulcastLayerMedium)

	info := sub.ToSubscriptionInfo()

	assert.NotNil(t, info)
	assert.Equal(t, sub.ID.String(), info.ID)
	assert.Equal(t, "subscriber1", info.SubscriberID)
	assert.Equal(t, "publisher1", info.PublisherID)
	assert.Equal(t, trackID.String(), info.TrackID)
	assert.Equal(t, SimulcastLayerMedium.String(), info.PreferredLayer)
}
