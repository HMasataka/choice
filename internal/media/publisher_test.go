package media

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

// newTestTrack creates a LocalTrack for testing purposes.
// It bypasses the normal validation that requires a non-nil webrtc.TrackRemote.
func newTestTrack(kind TrackKind, publisherID string) *LocalTrack {
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

func TestNewPublisher(t *testing.T) {
	p := NewPublisher("publisher-1")

	if p.ID != "publisher-1" {
		t.Errorf("expected ID publisher-1, got %s", p.ID)
	}
	if p.TrackCount() != 0 {
		t.Errorf("expected 0 tracks, got %d", p.TrackCount())
	}
	if p.GetCreatedAt().IsZero() {
		t.Error("expected non-zero created time")
	}
}

func TestPublisher_AddTrack(t *testing.T) {
	ctx := context.Background()
	p := NewPublisher("publisher-1")

	track := newTestTrack(TrackKindVideo, "publisher-1")

	err := p.AddTrack(ctx, track)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.TrackCount() != 1 {
		t.Errorf("expected 1 track, got %d", p.TrackCount())
	}
}

func TestPublisher_AddTrack_NilTrack(t *testing.T) {
	ctx := context.Background()
	p := NewPublisher("publisher-1")

	err := p.AddTrack(ctx, nil)
	if err == nil {
		t.Error("expected error for nil track")
	}
}

func TestPublisher_AddTrack_WrongPublisher(t *testing.T) {
	ctx := context.Background()
	p := NewPublisher("publisher-1")

	track := newTestTrack(TrackKindVideo, "publisher-2")

	err := p.AddTrack(ctx, track)
	if err == nil {
		t.Error("expected error for wrong publisher ID")
	}
}

func TestPublisher_AddTrack_Duplicate(t *testing.T) {
	ctx := context.Background()
	p := NewPublisher("publisher-1")

	track := newTestTrack(TrackKindVideo, "publisher-1")
	_ = p.AddTrack(ctx, track)

	err := p.AddTrack(ctx, track)
	if err == nil {
		t.Error("expected error for duplicate track")
	}
}

func TestPublisher_RemoveTrack(t *testing.T) {
	ctx := context.Background()
	p := NewPublisher("publisher-1")

	track := newTestTrack(TrackKindVideo, "publisher-1")
	_ = p.AddTrack(ctx, track)

	err := p.RemoveTrack(ctx, track.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.TrackCount() != 0 {
		t.Errorf("expected 0 tracks, got %d", p.TrackCount())
	}
}

func TestPublisher_RemoveTrack_NotFound(t *testing.T) {
	ctx := context.Background()
	p := NewPublisher("publisher-1")

	err := p.RemoveTrack(ctx, "non-existent-track-00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Error("expected error for non-existent track")
	}
}

func TestPublisher_GetTrack(t *testing.T) {
	ctx := context.Background()
	p := NewPublisher("publisher-1")

	track := newTestTrack(TrackKindVideo, "publisher-1")
	_ = p.AddTrack(ctx, track)

	retrieved, err := p.GetTrack(track.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if retrieved.ID != track.ID {
		t.Errorf("expected track ID %s, got %s", track.ID, retrieved.ID)
	}
}

func TestPublisher_ListTracks(t *testing.T) {
	ctx := context.Background()
	p := NewPublisher("publisher-1")

	track1 := newTestTrack(TrackKindVideo, "publisher-1")
	track2 := newTestTrack(TrackKindAudio, "publisher-1")
	_ = p.AddTrack(ctx, track1)
	_ = p.AddTrack(ctx, track2)

	tracks := p.ListTracks()
	if len(tracks) != 2 {
		t.Errorf("expected 2 tracks, got %d", len(tracks))
	}
}

func TestPublisher_Metadata(t *testing.T) {
	p := NewPublisher("publisher-1")

	p.SetMetadata("key1", "value1")
	p.SetMetadata("key2", "value2")

	value, found := p.GetMetadata("key1")
	if !found {
		t.Error("expected to find key1")
	}
	if value != "value1" {
		t.Errorf("expected value1, got %s", value)
	}

	all := p.GetAllMetadata()
	if len(all) != 2 {
		t.Errorf("expected 2 metadata entries, got %d", len(all))
	}
}

func TestPublisher_Callbacks(t *testing.T) {
	ctx := context.Background()
	p := NewPublisher("publisher-1")

	var addedTrack *LocalTrack
	var removedTrackID TrackID
	var wg sync.WaitGroup

	p.SetOnTrackAdded(func(track *LocalTrack) {
		addedTrack = track
		wg.Done()
	})
	p.SetOnTrackRemoved(func(trackID TrackID) {
		removedTrackID = trackID
		wg.Done()
	})

	track := newTestTrack(TrackKindVideo, "publisher-1")

	wg.Add(1)
	_ = p.AddTrack(ctx, track)
	wg.Wait()

	if addedTrack == nil || addedTrack.ID != track.ID {
		t.Error("expected track added callback with correct track")
	}

	wg.Add(1)
	_ = p.RemoveTrack(ctx, track.ID)
	wg.Wait()

	if removedTrackID != track.ID {
		t.Error("expected track removed callback with correct track ID")
	}
}

func TestPublisher_Concurrent(t *testing.T) {
	ctx := context.Background()
	p := NewPublisher("publisher-1")

	var wg sync.WaitGroup
	iterations := 50

	// Concurrent adds
	wg.Add(iterations)
	for i := 0; i < iterations; i++ {
		go func() {
			defer wg.Done()
			track := newTestTrack(TrackKindVideo, "publisher-1")
			_ = p.AddTrack(ctx, track)
		}()
	}
	wg.Wait()

	if p.TrackCount() != iterations {
		t.Errorf("expected %d tracks, got %d", iterations, p.TrackCount())
	}

	// Concurrent reads
	wg.Add(iterations)
	for i := 0; i < iterations; i++ {
		go func() {
			defer wg.Done()
			_ = p.ListTracks()
			_ = p.TrackCount()
		}()
	}
	wg.Wait()
}

// PublisherManager tests

func TestPublisherManager_GetOrCreate(t *testing.T) {
	m := NewPublisherManager()

	p1 := m.GetOrCreatePublisher("publisher-1")
	if p1 == nil {
		t.Fatal("expected non-nil publisher")
	}

	p2 := m.GetOrCreatePublisher("publisher-1")
	if p1 != p2 {
		t.Error("expected same publisher for same ID")
	}
}

func TestPublisherManager_Remove(t *testing.T) {
	m := NewPublisherManager()

	m.GetOrCreatePublisher("publisher-1")
	m.RemovePublisher("publisher-1")

	_, found := m.GetPublisher("publisher-1")
	if found {
		t.Error("expected publisher to be removed")
	}
}

func TestPublisherManager_List(t *testing.T) {
	m := NewPublisherManager()

	m.GetOrCreatePublisher("publisher-1")
	m.GetOrCreatePublisher("publisher-2")

	ids := m.ListPublishers()
	if len(ids) != 2 {
		t.Errorf("expected 2 publishers, got %d", len(ids))
	}

	if m.PublisherCount() != 2 {
		t.Errorf("expected count 2, got %d", m.PublisherCount())
	}
}

func TestPublisher_GetSimulcastLayers(t *testing.T) {
	ctx := context.Background()
	p := NewPublisher("publisher-1")

	track := newTestTrack(TrackKindVideo, "publisher-1")
	track.SetSimulcast(true)
	track.SetLayers([]SimulcastLayer{SimulcastLayerHigh, SimulcastLayerMedium, SimulcastLayerLow})
	_ = p.AddTrack(ctx, track)

	layers, err := p.GetSimulcastLayers(track.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(layers) != 3 {
		t.Errorf("expected 3 layers, got %d", len(layers))
	}
}

func TestPublisher_UpdatedAt(t *testing.T) {
	ctx := context.Background()
	p := NewPublisher("publisher-1")

	initialUpdated := p.GetUpdatedAt()

	time.Sleep(1 * time.Millisecond)

	track := newTestTrack(TrackKindVideo, "publisher-1")
	_ = p.AddTrack(ctx, track)

	afterAdd := p.GetUpdatedAt()
	if !afterAdd.After(initialUpdated) {
		t.Error("expected UpdatedAt to change after AddTrack")
	}
}
