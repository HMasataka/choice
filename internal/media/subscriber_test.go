package media

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

// newTestTrackForSub creates a LocalTrack for subscriber testing purposes.
func newTestTrackForSub(kind TrackKind, publisherID string) *LocalTrack {
	now := time.Now()
	return &LocalTrack{
		ID:          GenerateTrackID(),
		Kind:        kind,
		PublisherID: publisherID,
		RoomID:      "test-room",
		metadata:    &TrackMetadata{},
		Track:       &webrtc.TrackRemote{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func TestNewSubscriber(t *testing.T) {
	s := NewSubscriber("subscriber-1")

	if s.ID != "subscriber-1" {
		t.Errorf("expected ID subscriber-1, got %s", s.ID)
	}
	if s.SubscriptionCount() != 0 {
		t.Errorf("expected 0 subscriptions, got %d", s.SubscriptionCount())
	}
	if s.GetCreatedAt().IsZero() {
		t.Error("expected non-zero created time")
	}
}

func TestSubscriber_Subscribe(t *testing.T) {
	ctx := context.Background()
	s := NewSubscriber("subscriber-1")

	track := newTestTrackForSub(TrackKindVideo, "publisher-1")

	sub, err := s.Subscribe(ctx, "publisher-1", track.ID, SimulcastLayerHigh)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sub.SubscriberID != "subscriber-1" {
		t.Errorf("expected subscriber-1, got %s", sub.SubscriberID)
	}
	if sub.PublisherID != "publisher-1" {
		t.Errorf("expected publisher-1, got %s", sub.PublisherID)
	}
	if sub.TrackID != track.ID {
		t.Errorf("expected track ID %s, got %s", track.ID, sub.TrackID)
	}
	if sub.PreferredLayer != SimulcastLayerHigh {
		t.Errorf("expected high layer, got %s", sub.PreferredLayer)
	}
	if s.SubscriptionCount() != 1 {
		t.Errorf("expected 1 subscription, got %d", s.SubscriptionCount())
	}
}

func TestSubscriber_Subscribe_EmptyPublisherID(t *testing.T) {
	ctx := context.Background()
	s := NewSubscriber("subscriber-1")

	track := newTestTrackForSub(TrackKindVideo, "publisher-1")

	_, err := s.Subscribe(ctx, "", track.ID, SimulcastLayerHigh)
	if err == nil {
		t.Error("expected error for empty publisher ID")
	}
}

func TestSubscriber_Subscribe_Duplicate(t *testing.T) {
	ctx := context.Background()
	s := NewSubscriber("subscriber-1")

	track := newTestTrackForSub(TrackKindVideo, "publisher-1")

	_, _ = s.Subscribe(ctx, "publisher-1", track.ID, SimulcastLayerHigh)

	_, err := s.Subscribe(ctx, "publisher-1", track.ID, SimulcastLayerHigh)
	if err == nil {
		t.Error("expected error for duplicate subscription")
	}
}

func TestSubscriber_Unsubscribe(t *testing.T) {
	ctx := context.Background()
	s := NewSubscriber("subscriber-1")

	track := newTestTrackForSub(TrackKindVideo, "publisher-1")
	sub, _ := s.Subscribe(ctx, "publisher-1", track.ID, SimulcastLayerHigh)

	err := s.Unsubscribe(ctx, sub.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if s.SubscriptionCount() != 0 {
		t.Errorf("expected 0 subscriptions, got %d", s.SubscriptionCount())
	}
}

func TestSubscriber_Unsubscribe_NotFound(t *testing.T) {
	ctx := context.Background()
	s := NewSubscriber("subscriber-1")

	err := s.Unsubscribe(ctx, "non-existent-00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Error("expected error for non-existent subscription")
	}
}

func TestSubscriber_UnsubscribeByTrack(t *testing.T) {
	ctx := context.Background()
	s := NewSubscriber("subscriber-1")

	track := newTestTrackForSub(TrackKindVideo, "publisher-1")
	_, _ = s.Subscribe(ctx, "publisher-1", track.ID, SimulcastLayerHigh)

	err := s.UnsubscribeByTrack(ctx, track.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if s.SubscriptionCount() != 0 {
		t.Errorf("expected 0 subscriptions, got %d", s.SubscriptionCount())
	}
}

func TestSubscriber_SetPreferredLayer(t *testing.T) {
	ctx := context.Background()
	s := NewSubscriber("subscriber-1")

	track := newTestTrackForSub(TrackKindVideo, "publisher-1")
	sub, _ := s.Subscribe(ctx, "publisher-1", track.ID, SimulcastLayerHigh)

	err := s.SetPreferredLayer(ctx, sub.ID, SimulcastLayerMedium)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	retrieved, _ := s.GetSubscription(sub.ID)
	if retrieved.PreferredLayer != SimulcastLayerMedium {
		t.Errorf("expected medium layer, got %s", retrieved.PreferredLayer)
	}
}

func TestSubscriber_GetSubscription(t *testing.T) {
	ctx := context.Background()
	s := NewSubscriber("subscriber-1")

	track := newTestTrackForSub(TrackKindVideo, "publisher-1")
	sub, _ := s.Subscribe(ctx, "publisher-1", track.ID, SimulcastLayerHigh)

	retrieved, err := s.GetSubscription(sub.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if retrieved.ID != sub.ID {
		t.Errorf("expected subscription ID %s, got %s", sub.ID, retrieved.ID)
	}
}

func TestSubscriber_GetSubscriptionByTrack(t *testing.T) {
	ctx := context.Background()
	s := NewSubscriber("subscriber-1")

	track := newTestTrackForSub(TrackKindVideo, "publisher-1")
	sub, _ := s.Subscribe(ctx, "publisher-1", track.ID, SimulcastLayerHigh)

	retrieved, err := s.GetSubscriptionByTrack(track.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if retrieved.ID != sub.ID {
		t.Errorf("expected subscription ID %s, got %s", sub.ID, retrieved.ID)
	}
}

func TestSubscriber_ListSubscriptions(t *testing.T) {
	ctx := context.Background()
	s := NewSubscriber("subscriber-1")

	track1 := newTestTrackForSub(TrackKindVideo, "publisher-1")
	track2 := newTestTrackForSub(TrackKindAudio, "publisher-1")
	_, _ = s.Subscribe(ctx, "publisher-1", track1.ID, SimulcastLayerHigh)
	_, _ = s.Subscribe(ctx, "publisher-1", track2.ID, SimulcastLayerHigh)

	subs := s.ListSubscriptions()
	if len(subs) != 2 {
		t.Errorf("expected 2 subscriptions, got %d", len(subs))
	}
}

func TestSubscriber_IsSubscribedTo(t *testing.T) {
	ctx := context.Background()
	s := NewSubscriber("subscriber-1")

	track := newTestTrackForSub(TrackKindVideo, "publisher-1")

	if s.IsSubscribedTo(track.ID) {
		t.Error("expected not subscribed initially")
	}

	_, _ = s.Subscribe(ctx, "publisher-1", track.ID, SimulcastLayerHigh)

	if !s.IsSubscribedTo(track.ID) {
		t.Error("expected subscribed after Subscribe")
	}
}

func TestSubscriber_Callbacks(t *testing.T) {
	ctx := context.Background()
	s := NewSubscriber("subscriber-1")

	var addedSub *Subscription
	var removedSubID SubscriptionID
	var layerChanges []struct {
		subID         SubscriptionID
		previousLayer SimulcastLayer
		newLayer      SimulcastLayer
	}
	var wg sync.WaitGroup

	s.SetOnSubscriptionAdded(func(sub *Subscription) {
		addedSub = sub
		wg.Done()
	})
	s.SetOnSubscriptionRemoved(func(subID SubscriptionID) {
		removedSubID = subID
		wg.Done()
	})
	s.SetOnLayerChanged(func(subID SubscriptionID, previousLayer, newLayer SimulcastLayer) {
		layerChanges = append(layerChanges, struct {
			subID         SubscriptionID
			previousLayer SimulcastLayer
			newLayer      SimulcastLayer
		}{subID, previousLayer, newLayer})
		wg.Done()
	})

	track := newTestTrackForSub(TrackKindVideo, "publisher-1")

	wg.Add(1)
	sub, _ := s.Subscribe(ctx, "publisher-1", track.ID, SimulcastLayerHigh)
	wg.Wait()

	if addedSub == nil || addedSub.ID != sub.ID {
		t.Error("expected subscription added callback with correct subscription")
	}

	wg.Add(1)
	_ = s.SetPreferredLayer(ctx, sub.ID, SimulcastLayerMedium)
	wg.Wait()

	if len(layerChanges) != 1 {
		t.Errorf("expected 1 layer change, got %d", len(layerChanges))
	}
	if layerChanges[0].previousLayer != SimulcastLayerHigh {
		t.Errorf("expected previous layer high, got %s", layerChanges[0].previousLayer)
	}
	if layerChanges[0].newLayer != SimulcastLayerMedium {
		t.Errorf("expected new layer medium, got %s", layerChanges[0].newLayer)
	}

	wg.Add(1)
	_ = s.Unsubscribe(ctx, sub.ID)
	wg.Wait()

	if removedSubID != sub.ID {
		t.Error("expected subscription removed callback with correct ID")
	}
}

func TestSubscriber_Concurrent(t *testing.T) {
	ctx := context.Background()
	s := NewSubscriber("subscriber-1")

	var wg sync.WaitGroup
	iterations := 50

	// Concurrent subscribes
	wg.Add(iterations)
	for i := 0; i < iterations; i++ {
		go func() {
			defer wg.Done()
			track := newTestTrackForSub(TrackKindVideo, "publisher-1")
			_, _ = s.Subscribe(ctx, "publisher-1", track.ID, SimulcastLayerHigh)
		}()
	}
	wg.Wait()

	if s.SubscriptionCount() != iterations {
		t.Errorf("expected %d subscriptions, got %d", iterations, s.SubscriptionCount())
	}

	// Concurrent reads
	wg.Add(iterations)
	for i := 0; i < iterations; i++ {
		go func() {
			defer wg.Done()
			_ = s.ListSubscriptions()
			_ = s.SubscriptionCount()
		}()
	}
	wg.Wait()
}

// SubscriberManager tests

func TestSubscriberManager_GetOrCreate(t *testing.T) {
	m := NewSubscriberManager()

	s1 := m.GetOrCreateSubscriber("subscriber-1")
	if s1 == nil {
		t.Fatal("expected non-nil subscriber")
	}

	s2 := m.GetOrCreateSubscriber("subscriber-1")
	if s1 != s2 {
		t.Error("expected same subscriber for same ID")
	}
}

func TestSubscriberManager_Remove(t *testing.T) {
	m := NewSubscriberManager()

	m.GetOrCreateSubscriber("subscriber-1")
	m.RemoveSubscriber("subscriber-1")

	_, found := m.GetSubscriber("subscriber-1")
	if found {
		t.Error("expected subscriber to be removed")
	}
}

func TestSubscriberManager_List(t *testing.T) {
	m := NewSubscriberManager()

	m.GetOrCreateSubscriber("subscriber-1")
	m.GetOrCreateSubscriber("subscriber-2")

	ids := m.ListSubscribers()
	if len(ids) != 2 {
		t.Errorf("expected 2 subscribers, got %d", len(ids))
	}

	if m.SubscriberCount() != 2 {
		t.Errorf("expected count 2, got %d", m.SubscriberCount())
	}
}

func TestSubscriberManager_GetSubscribersToTrack(t *testing.T) {
	ctx := context.Background()
	m := NewSubscriberManager()

	track := newTestTrackForSub(TrackKindVideo, "publisher-1")

	s1 := m.GetOrCreateSubscriber("subscriber-1")
	s2 := m.GetOrCreateSubscriber("subscriber-2")
	s3 := m.GetOrCreateSubscriber("subscriber-3")

	_, _ = s1.Subscribe(ctx, "publisher-1", track.ID, SimulcastLayerHigh)
	_, _ = s2.Subscribe(ctx, "publisher-1", track.ID, SimulcastLayerHigh)
	// s3 doesn't subscribe to this track

	subscribers := m.GetSubscribersToTrack(track.ID)
	if len(subscribers) != 2 {
		t.Errorf("expected 2 subscribers, got %d", len(subscribers))
	}

	_ = s3
}

func TestSubscriber_UpdatedAt(t *testing.T) {
	ctx := context.Background()
	s := NewSubscriber("subscriber-1")

	initialUpdated := s.GetUpdatedAt()

	time.Sleep(1 * time.Millisecond)

	track := newTestTrackForSub(TrackKindVideo, "publisher-1")
	_, _ = s.Subscribe(ctx, "publisher-1", track.ID, SimulcastLayerHigh)

	afterSubscribe := s.GetUpdatedAt()
	if !afterSubscribe.After(initialUpdated) {
		t.Error("expected UpdatedAt to change after Subscribe")
	}
}
