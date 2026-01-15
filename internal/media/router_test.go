package media

import (
	"context"
	"sync"
	"testing"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaRouter_AddTrack(t *testing.T) {
	router := NewMediaRouter()
	ctx := context.Background()

	// Create a mock WebRTC track
	mockTrack := &webrtc.TrackRemote{}

	// Create a valid track
	track := NewLocalTrack("publisher1", "room1", TrackKindVideo, mockTrack, &TrackMetadata{
		Label:     "camera",
		Simulcast: true,
		Layers:    []SimulcastLayer{SimulcastLayerHigh, SimulcastLayerMedium, SimulcastLayerLow},
		MID:       "0",
		SSRC:      12345,
	})

	// Test adding a track
	err := router.AddTrack(ctx, track)
	assert.NoError(t, err)

	// Verify track was added
	retrieved, err := router.GetTrack(ctx, track.ID)
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, track.ID, retrieved.ID)
	assert.Equal(t, track.PublisherID, retrieved.PublisherID)

	// Test adding duplicate track
	err = router.AddTrack(ctx, track)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestMediaRouter_AddTrack_NilTrack(t *testing.T) {
	router := NewMediaRouter()
	ctx := context.Background()

	err := router.AddTrack(ctx, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "track cannot be nil")
}

func TestMediaRouter_AddTrack_InvalidTrack(t *testing.T) {
	router := NewMediaRouter()
	ctx := context.Background()

	// Create an invalid track (missing required fields)
	track := &LocalTrack{
		ID:   "", // Invalid: empty ID
		Kind: TrackKindVideo,
	}

	err := router.AddTrack(ctx, track)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid track")
}

func TestMediaRouter_RemoveTrack(t *testing.T) {
	router := NewMediaRouter()
	ctx := context.Background()

	// Create and add a track
	mockTrack := &webrtc.TrackRemote{}
	track := NewLocalTrack("publisher1", "room1", TrackKindVideo, mockTrack, &TrackMetadata{
		Label: "camera",
		MID:   "0",
		SSRC:  12345,
	})
	err := router.AddTrack(ctx, track)
	require.NoError(t, err)

	// Create a subscription to the track
	opts := &SubscribeOptions{PreferredLayer: SimulcastLayerHigh}
	sub, err := router.Subscribe(ctx, "subscriber1", track.ID, opts)
	require.NoError(t, err)
	require.NotNil(t, sub)

	// Remove the track
	err = router.RemoveTrack(ctx, track.ID)
	assert.NoError(t, err)

	// Verify track was removed
	_, err = router.GetTrack(ctx, track.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Verify subscription was also removed
	r := router.(*mediaRouter)
	_, err = r.GetSubscription(ctx, sub.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestMediaRouter_RemoveTrack_NotFound(t *testing.T) {
	router := NewMediaRouter()
	ctx := context.Background()

	trackID := GenerateTrackID()
	err := router.RemoveTrack(ctx, trackID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestMediaRouter_Subscribe(t *testing.T) {
	router := NewMediaRouter()
	ctx := context.Background()

	// Create and add a track
	mockTrack := &webrtc.TrackRemote{}
	track := NewLocalTrack("publisher1", "room1", TrackKindVideo, mockTrack, &TrackMetadata{
		Label:     "camera",
		Simulcast: true,
		Layers:    []SimulcastLayer{SimulcastLayerHigh, SimulcastLayerMedium, SimulcastLayerLow},
		MID:       "0",
		SSRC:      12345,
	})
	err := router.AddTrack(ctx, track)
	require.NoError(t, err)

	// Subscribe to the track
	opts := &SubscribeOptions{PreferredLayer: SimulcastLayerMedium}
	sub, err := router.Subscribe(ctx, "subscriber1", track.ID, opts)
	assert.NoError(t, err)
	assert.NotNil(t, sub)
	assert.Equal(t, "subscriber1", sub.SubscriberID)
	assert.Equal(t, "publisher1", sub.PublisherID)
	assert.Equal(t, track.ID, sub.TrackID)
	assert.Equal(t, SimulcastLayerMedium, sub.PreferredLayer)

	// Verify subscription was stored
	r := router.(*mediaRouter)
	retrieved, err := r.GetSubscription(ctx, sub.ID)
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, sub.ID, retrieved.ID)
}

func TestMediaRouter_Subscribe_DefaultLayer(t *testing.T) {
	router := NewMediaRouter()
	ctx := context.Background()

	// Create and add a track
	mockTrack := &webrtc.TrackRemote{}
	track := NewLocalTrack("publisher1", "room1", TrackKindVideo, mockTrack, &TrackMetadata{
		Label: "camera",
		MID:   "0",
		SSRC:  12345,
	})
	err := router.AddTrack(ctx, track)
	require.NoError(t, err)

	// Subscribe without specifying layer (should default to high)
	opts := &SubscribeOptions{}
	sub, err := router.Subscribe(ctx, "subscriber1", track.ID, opts)
	assert.NoError(t, err)
	assert.NotNil(t, sub)
	assert.Equal(t, SimulcastLayerHigh, sub.PreferredLayer)
}

func TestMediaRouter_Subscribe_TrackNotFound(t *testing.T) {
	router := NewMediaRouter()
	ctx := context.Background()

	trackID := GenerateTrackID()
	opts := &SubscribeOptions{PreferredLayer: SimulcastLayerHigh}
	sub, err := router.Subscribe(ctx, "subscriber1", trackID, opts)
	assert.Error(t, err)
	assert.Nil(t, sub)
	assert.Contains(t, err.Error(), "not found")
}

func TestMediaRouter_Subscribe_EmptySubscriberID(t *testing.T) {
	router := NewMediaRouter()
	ctx := context.Background()

	// Create and add a track
	mockTrack := &webrtc.TrackRemote{}
	track := NewLocalTrack("publisher1", "room1", TrackKindVideo, mockTrack, &TrackMetadata{
		Label: "camera",
		MID:   "0",
		SSRC:  12345,
	})
	err := router.AddTrack(ctx, track)
	require.NoError(t, err)

	// Try to subscribe with empty subscriber ID
	opts := &SubscribeOptions{PreferredLayer: SimulcastLayerHigh}
	sub, err := router.Subscribe(ctx, "", track.ID, opts)
	assert.Error(t, err)
	assert.Nil(t, sub)
	assert.Contains(t, err.Error(), "subscriber ID cannot be empty")
}

func TestMediaRouter_Subscribe_LayerValidation(t *testing.T) {
	router := NewMediaRouter()
	ctx := context.Background()

	mockTrack := &webrtc.TrackRemote{}

	// Test 1: Subscribe to non-simulcast track with non-high layer should fail
	nonSimulcastTrack := NewLocalTrack("publisher1", "room1", TrackKindVideo, mockTrack, &TrackMetadata{
		Label:     "camera",
		Simulcast: false,
		MID:       "0",
		SSRC:      12345,
	})
	err := router.AddTrack(ctx, nonSimulcastTrack)
	require.NoError(t, err)

	opts := &SubscribeOptions{PreferredLayer: SimulcastLayerMedium}
	sub, err := router.Subscribe(ctx, "subscriber1", nonSimulcastTrack.ID, opts)
	assert.Error(t, err)
	assert.Nil(t, sub)
	assert.Contains(t, err.Error(), "does not support simulcast")

	// Test 2: Subscribe to non-simulcast track with high layer should succeed
	opts = &SubscribeOptions{PreferredLayer: SimulcastLayerHigh}
	sub, err = router.Subscribe(ctx, "subscriber1", nonSimulcastTrack.ID, opts)
	assert.NoError(t, err)
	assert.NotNil(t, sub)

	// Test 3: Subscribe to simulcast track with unavailable medium layer should fail
	simulcastTrack := NewLocalTrack("publisher2", "room1", TrackKindVideo, mockTrack, &TrackMetadata{
		Label:     "screen",
		Simulcast: true,
		Layers:    []SimulcastLayer{SimulcastLayerHigh, SimulcastLayerLow}, // No medium layer
		MID:       "1",
		SSRC:      12346,
	})
	err = router.AddTrack(ctx, simulcastTrack)
	require.NoError(t, err)

	opts = &SubscribeOptions{PreferredLayer: SimulcastLayerMedium}
	sub, err = router.Subscribe(ctx, "subscriber2", simulcastTrack.ID, opts)
	assert.Error(t, err)
	assert.Nil(t, sub)
	assert.Contains(t, err.Error(), "does not have layer")

	// Test 4: Subscribe to simulcast track with available layer should succeed
	opts = &SubscribeOptions{PreferredLayer: SimulcastLayerLow}
	sub, err = router.Subscribe(ctx, "subscriber2", simulcastTrack.ID, opts)
	assert.NoError(t, err)
	assert.NotNil(t, sub)
	assert.Equal(t, SimulcastLayerLow, sub.PreferredLayer)

	// Test 5: Subscribe to simulcast track with available high layer should succeed
	opts = &SubscribeOptions{PreferredLayer: SimulcastLayerHigh}
	sub, err = router.Subscribe(ctx, "subscriber3", simulcastTrack.ID, opts)
	assert.NoError(t, err)
	assert.NotNil(t, sub)
	assert.Equal(t, SimulcastLayerHigh, sub.PreferredLayer)

	// Test 6: Subscribe to simulcast track missing high layer should fail
	partialSimulcastTrack := NewLocalTrack("publisher3", "room1", TrackKindVideo, mockTrack, &TrackMetadata{
		Label:     "lowres-camera",
		Simulcast: true,
		Layers:    []SimulcastLayer{SimulcastLayerMedium, SimulcastLayerLow}, // No high layer
		MID:       "2",
		SSRC:      12347,
	})
	err = router.AddTrack(ctx, partialSimulcastTrack)
	require.NoError(t, err)

	opts = &SubscribeOptions{PreferredLayer: SimulcastLayerHigh}
	sub, err = router.Subscribe(ctx, "subscriber4", partialSimulcastTrack.ID, opts)
	assert.Error(t, err)
	assert.Nil(t, sub)
	assert.Contains(t, err.Error(), "does not have layer")
}

func TestMediaRouter_Unsubscribe(t *testing.T) {
	router := NewMediaRouter()
	ctx := context.Background()

	// Create and add a track
	mockTrack := &webrtc.TrackRemote{}
	track := NewLocalTrack("publisher1", "room1", TrackKindVideo, mockTrack, &TrackMetadata{
		Label: "camera",
		MID:   "0",
		SSRC:  12345,
	})
	err := router.AddTrack(ctx, track)
	require.NoError(t, err)

	// Subscribe to the track
	opts := &SubscribeOptions{PreferredLayer: SimulcastLayerHigh}
	sub, err := router.Subscribe(ctx, "subscriber1", track.ID, opts)
	require.NoError(t, err)

	// Unsubscribe
	err = router.Unsubscribe(ctx, sub.ID)
	assert.NoError(t, err)

	// Verify subscription was removed
	r := router.(*mediaRouter)
	_, err = r.GetSubscription(ctx, sub.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestMediaRouter_Unsubscribe_NotFound(t *testing.T) {
	router := NewMediaRouter()
	ctx := context.Background()

	subID := GenerateSubscriptionID()
	err := router.Unsubscribe(ctx, subID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestMediaRouter_GetTrack(t *testing.T) {
	router := NewMediaRouter()
	ctx := context.Background()

	// Create and add a track
	mockTrack := &webrtc.TrackRemote{}
	track := NewLocalTrack("publisher1", "room1", TrackKindVideo, mockTrack, &TrackMetadata{
		Label: "camera",
		MID:   "0",
		SSRC:  12345,
	})
	err := router.AddTrack(ctx, track)
	require.NoError(t, err)

	// Get the track
	retrieved, err := router.GetTrack(ctx, track.ID)
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, track.ID, retrieved.ID)
	assert.Equal(t, track.PublisherID, retrieved.PublisherID)
	assert.Equal(t, track.Kind, retrieved.Kind)
}

func TestMediaRouter_GetTrack_NotFound(t *testing.T) {
	router := NewMediaRouter()
	ctx := context.Background()

	trackID := GenerateTrackID()
	track, err := router.GetTrack(ctx, trackID)
	assert.Error(t, err)
	assert.Nil(t, track)
	assert.Contains(t, err.Error(), "not found")
}

func TestMediaRouter_ListTracks(t *testing.T) {
	router := NewMediaRouter()
	ctx := context.Background()

	// Initially empty
	tracks, err := router.ListTracks(ctx)
	assert.NoError(t, err)
	assert.Empty(t, tracks)

	// Add multiple tracks
	mockTrack := &webrtc.TrackRemote{}
	track1 := NewLocalTrack("publisher1", "room1", TrackKindVideo, mockTrack, &TrackMetadata{
		Label: "camera",
		MID:   "0",
		SSRC:  12345,
	})
	track2 := NewLocalTrack("publisher1", "room1", TrackKindAudio, mockTrack, &TrackMetadata{
		Label: "microphone",
		MID:   "1",
		SSRC:  12346,
	})
	track3 := NewLocalTrack("publisher2", "room1", TrackKindVideo, mockTrack, &TrackMetadata{
		Label: "screen",
		MID:   "2",
		SSRC:  12347,
	})

	err = router.AddTrack(ctx, track1)
	require.NoError(t, err)
	err = router.AddTrack(ctx, track2)
	require.NoError(t, err)
	err = router.AddTrack(ctx, track3)
	require.NoError(t, err)

	// List all tracks
	tracks, err = router.ListTracks(ctx)
	assert.NoError(t, err)
	assert.Len(t, tracks, 3)

	// Verify track IDs are present
	trackIDs := make(map[TrackID]bool)
	for _, track := range tracks {
		trackIDs[track.ID] = true
	}
	assert.True(t, trackIDs[track1.ID])
	assert.True(t, trackIDs[track2.ID])
	assert.True(t, trackIDs[track3.ID])
}

func TestMediaRouter_ListSubscriptionsByTrack(t *testing.T) {
	router := NewMediaRouter().(*mediaRouter)
	ctx := context.Background()

	// Create and add a track
	mockTrack := &webrtc.TrackRemote{}
	track := NewLocalTrack("publisher1", "room1", TrackKindVideo, mockTrack, &TrackMetadata{
		Label: "camera",
		MID:   "0",
		SSRC:  12345,
	})
	err := router.AddTrack(ctx, track)
	require.NoError(t, err)

	// Create multiple subscriptions
	opts := &SubscribeOptions{PreferredLayer: SimulcastLayerHigh}
	sub1, err := router.Subscribe(ctx, "subscriber1", track.ID, opts)
	require.NoError(t, err)
	sub2, err := router.Subscribe(ctx, "subscriber2", track.ID, opts)
	require.NoError(t, err)

	// List subscriptions for the track
	subs, err := router.ListSubscriptionsByTrack(ctx, track.ID)
	assert.NoError(t, err)
	assert.Len(t, subs, 2)

	// Verify subscription IDs
	subIDs := make(map[SubscriptionID]bool)
	for _, sub := range subs {
		subIDs[sub.ID] = true
	}
	assert.True(t, subIDs[sub1.ID])
	assert.True(t, subIDs[sub2.ID])
}

func TestMediaRouter_ListSubscriptionsBySubscriber(t *testing.T) {
	router := NewMediaRouter().(*mediaRouter)
	ctx := context.Background()

	// Create and add multiple tracks
	mockTrack := &webrtc.TrackRemote{}
	track1 := NewLocalTrack("publisher1", "room1", TrackKindVideo, mockTrack, &TrackMetadata{
		Label: "camera",
		MID:   "0",
		SSRC:  12345,
	})
	track2 := NewLocalTrack("publisher1", "room1", TrackKindAudio, mockTrack, &TrackMetadata{
		Label: "microphone",
		MID:   "1",
		SSRC:  12346,
	})
	err := router.AddTrack(ctx, track1)
	require.NoError(t, err)
	err = router.AddTrack(ctx, track2)
	require.NoError(t, err)

	// Subscribe to multiple tracks
	opts := &SubscribeOptions{PreferredLayer: SimulcastLayerHigh}
	sub1, err := router.Subscribe(ctx, "subscriber1", track1.ID, opts)
	require.NoError(t, err)
	sub2, err := router.Subscribe(ctx, "subscriber1", track2.ID, opts)
	require.NoError(t, err)

	// List subscriptions for the subscriber
	subs, err := router.ListSubscriptionsBySubscriber(ctx, "subscriber1")
	assert.NoError(t, err)
	assert.Len(t, subs, 2)

	// Verify subscription IDs
	subIDs := make(map[SubscriptionID]bool)
	for _, sub := range subs {
		subIDs[sub.ID] = true
	}
	assert.True(t, subIDs[sub1.ID])
	assert.True(t, subIDs[sub2.ID])
}

func TestMediaRouter_ConcurrentAccess(t *testing.T) {
	router := NewMediaRouter()
	ctx := context.Background()

	mockTrack := &webrtc.TrackRemote{}

	// Concurrent AddTrack
	var wg sync.WaitGroup
	numGoroutines := 10
	wg.Add(numGoroutines)

	// Collect errors from goroutines using a channel
	errChan := make(chan error, numGoroutines*2) // Buffer for both AddTrack and Subscribe errors

	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			defer wg.Done()
			track := NewLocalTrack("publisher1", "room1", TrackKindVideo, mockTrack, &TrackMetadata{
				Label: "camera",
				MID:   string(rune('0' + index)),
				SSRC:  uint32(12345 + index),
			})
			if err := router.AddTrack(ctx, track); err != nil {
				errChan <- err
			}
		}(i)
	}

	wg.Wait()

	// Check for any AddTrack errors
	close(errChan)
	for err := range errChan {
		t.Errorf("AddTrack error: %v", err)
	}

	// Verify all tracks were added
	tracks, err := router.ListTracks(ctx)
	assert.NoError(t, err)
	assert.Len(t, tracks, numGoroutines)

	// Concurrent Subscribe
	wg.Add(numGoroutines)
	subErrChan := make(chan error, numGoroutines*numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			defer wg.Done()
			opts := &SubscribeOptions{PreferredLayer: SimulcastLayerHigh}
			for _, track := range tracks {
				if _, err := router.Subscribe(ctx, "subscriber1", track.ID, opts); err != nil {
					subErrChan <- err
				}
			}
		}(i)
	}

	wg.Wait()

	// Check for any Subscribe errors
	close(subErrChan)
	for err := range subErrChan {
		t.Errorf("Subscribe error: %v", err)
	}

	// Verify subscriptions
	r := router.(*mediaRouter)
	subs, err := r.ListSubscriptionsBySubscriber(ctx, "subscriber1")
	assert.NoError(t, err)
	// Should have numGoroutines * numTracks subscriptions
	assert.Equal(t, numGoroutines*numGoroutines, len(subs))
}

func TestMediaRouter_RemoveTrack_CascadeUnsubscribe(t *testing.T) {
	router := NewMediaRouter()
	ctx := context.Background()

	// Create and add a track
	mockTrack := &webrtc.TrackRemote{}
	track := NewLocalTrack("publisher1", "room1", TrackKindVideo, mockTrack, &TrackMetadata{
		Label: "camera",
		MID:   "0",
		SSRC:  12345,
	})
	err := router.AddTrack(ctx, track)
	require.NoError(t, err)

	// Create multiple subscriptions
	opts := &SubscribeOptions{PreferredLayer: SimulcastLayerHigh}
	sub1, err := router.Subscribe(ctx, "subscriber1", track.ID, opts)
	require.NoError(t, err)
	sub2, err := router.Subscribe(ctx, "subscriber2", track.ID, opts)
	require.NoError(t, err)
	sub3, err := router.Subscribe(ctx, "subscriber3", track.ID, opts)
	require.NoError(t, err)

	// Remove the track
	err = router.RemoveTrack(ctx, track.ID)
	assert.NoError(t, err)

	// Verify all subscriptions were removed
	r := router.(*mediaRouter)
	_, err = r.GetSubscription(ctx, sub1.ID)
	assert.Error(t, err)
	_, err = r.GetSubscription(ctx, sub2.ID)
	assert.Error(t, err)
	_, err = r.GetSubscription(ctx, sub3.ID)
	assert.Error(t, err)
}
