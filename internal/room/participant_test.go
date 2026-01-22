package room

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewParticipant(t *testing.T) {
	t.Run("creates participant with provided ID", func(t *testing.T) {
		p := NewParticipant("participant-1", "room-1")

		assert.Equal(t, "participant-1", p.ID)
		assert.Equal(t, "room-1", p.RoomID)
		assert.Equal(t, ParticipantStateJoining, p.State)
		assert.Equal(t, "publisher", p.Role)
		assert.NotNil(t, p.Metadata)
		assert.Empty(t, p.publishedTracks)
		assert.Empty(t, p.subscriptions)
		assert.False(t, p.JoinedAt.IsZero())
		assert.False(t, p.UpdatedAt.IsZero())
	})

	t.Run("generates ID when empty", func(t *testing.T) {
		p := NewParticipant("", "room-1")

		assert.NotEmpty(t, p.ID)
		// Should be a UUID format
		assert.Len(t, p.ID, 36)
	})

	t.Run("creates participant with options", func(t *testing.T) {
		metadata := map[string]interface{}{"displayName": "Alice"}
		p := NewParticipant("participant-1", "room-1",
			WithRole("admin"),
			WithParticipantMetadata(metadata),
		)

		assert.Equal(t, "admin", p.Role)
		assert.Equal(t, "Alice", p.Metadata["displayName"])
	})
}

func TestParticipant_State(t *testing.T) {
	t.Run("sets and gets state", func(t *testing.T) {
		p := NewParticipant("p1", "room-1")

		p.SetState(ParticipantStateJoined)
		assert.Equal(t, ParticipantStateJoined, p.GetState())

		p.SetState(ParticipantStatePublishing)
		assert.Equal(t, ParticipantStatePublishing, p.GetState())

		p.SetState(ParticipantStateSubscribing)
		assert.Equal(t, ParticipantStateSubscribing, p.GetState())

		p.SetState(ParticipantStateLeaving)
		assert.Equal(t, ParticipantStateLeaving, p.GetState())

		p.SetState(ParticipantStateLeft)
		assert.Equal(t, ParticipantStateLeft, p.GetState())
	})

	t.Run("updates UpdatedAt on state change", func(t *testing.T) {
		p := NewParticipant("p1", "room-1")
		initialUpdatedAt := p.UpdatedAt

		p.SetState(ParticipantStateJoined)
		assert.True(t, p.UpdatedAt.Equal(initialUpdatedAt) || p.UpdatedAt.After(initialUpdatedAt))
	})
}

func TestParticipant_PublishedTracks(t *testing.T) {
	t.Run("adds published track", func(t *testing.T) {
		p := NewParticipant("p1", "room-1")
		p.SetState(ParticipantStateJoined)

		p.AddPublishedTrack("track-1")
		tracks := p.GetPublishedTracks()

		assert.Len(t, tracks, 1)
		assert.Equal(t, "track-1", tracks[0])
		assert.Equal(t, ParticipantStatePublishing, p.GetState())
	})

	t.Run("adds multiple tracks", func(t *testing.T) {
		p := NewParticipant("p1", "room-1")

		p.AddPublishedTrack("track-1")
		p.AddPublishedTrack("track-2")
		p.AddPublishedTrack("track-3")

		tracks := p.GetPublishedTracks()
		assert.Len(t, tracks, 3)
	})

	t.Run("removes published track", func(t *testing.T) {
		p := NewParticipant("p1", "room-1")
		p.AddPublishedTrack("track-1")
		p.AddPublishedTrack("track-2")

		p.RemovePublishedTrack("track-1")

		tracks := p.GetPublishedTracks()
		assert.Len(t, tracks, 1)
		assert.Equal(t, "track-2", tracks[0])
	})

	t.Run("removing non-existent track does nothing", func(t *testing.T) {
		p := NewParticipant("p1", "room-1")
		p.AddPublishedTrack("track-1")

		p.RemovePublishedTrack("nonexistent")

		tracks := p.GetPublishedTracks()
		assert.Len(t, tracks, 1)
	})

	t.Run("returns copy of tracks", func(t *testing.T) {
		p := NewParticipant("p1", "room-1")
		p.AddPublishedTrack("track-1")

		tracks := p.GetPublishedTracks()
		tracks[0] = "modified"

		// Original should be unchanged
		originalTracks := p.GetPublishedTracks()
		assert.Equal(t, "track-1", originalTracks[0])
	})
}

func TestParticipant_Subscriptions(t *testing.T) {
	t.Run("adds subscription", func(t *testing.T) {
		p := NewParticipant("p1", "room-1")
		p.SetState(ParticipantStateJoined)

		p.AddSubscription("sub-1")
		subs := p.GetSubscriptions()

		assert.Len(t, subs, 1)
		assert.Equal(t, "sub-1", subs[0])
		assert.Equal(t, ParticipantStateSubscribing, p.GetState())
	})

	t.Run("adds multiple subscriptions", func(t *testing.T) {
		p := NewParticipant("p1", "room-1")

		p.AddSubscription("sub-1")
		p.AddSubscription("sub-2")
		p.AddSubscription("sub-3")

		subs := p.GetSubscriptions()
		assert.Len(t, subs, 3)
	})

	t.Run("removes subscription", func(t *testing.T) {
		p := NewParticipant("p1", "room-1")
		p.AddSubscription("sub-1")
		p.AddSubscription("sub-2")

		p.RemoveSubscription("sub-1")

		subs := p.GetSubscriptions()
		assert.Len(t, subs, 1)
		assert.Equal(t, "sub-2", subs[0])
	})

	t.Run("removing non-existent subscription does nothing", func(t *testing.T) {
		p := NewParticipant("p1", "room-1")
		p.AddSubscription("sub-1")

		p.RemoveSubscription("nonexistent")

		subs := p.GetSubscriptions()
		assert.Len(t, subs, 1)
	})

	t.Run("returns copy of subscriptions", func(t *testing.T) {
		p := NewParticipant("p1", "room-1")
		p.AddSubscription("sub-1")

		subs := p.GetSubscriptions()
		subs[0] = "modified"

		// Original should be unchanged
		originalSubs := p.GetSubscriptions()
		assert.Equal(t, "sub-1", originalSubs[0])
	})
}

func TestParticipant_Metadata(t *testing.T) {
	t.Run("gets metadata", func(t *testing.T) {
		metadata := map[string]interface{}{"key": "value"}
		p := NewParticipant("p1", "room-1", WithParticipantMetadata(metadata))

		got := p.GetMetadata()
		assert.Equal(t, "value", got["key"])
	})

	t.Run("sets metadata", func(t *testing.T) {
		p := NewParticipant("p1", "room-1")
		newMetadata := map[string]interface{}{"newKey": "newValue"}

		p.SetMetadata(newMetadata)

		got := p.GetMetadata()
		assert.Equal(t, "newValue", got["newKey"])
	})

	t.Run("returns copy of metadata", func(t *testing.T) {
		metadata := map[string]interface{}{"key": "value"}
		p := NewParticipant("p1", "room-1", WithParticipantMetadata(metadata))

		got := p.GetMetadata()
		got["key"] = "modified"

		// Original should be unchanged
		original := p.GetMetadata()
		assert.Equal(t, "value", original["key"])
	})
}

func TestParticipant_Role(t *testing.T) {
	t.Run("gets role", func(t *testing.T) {
		p := NewParticipant("p1", "room-1", WithRole("moderator"))
		assert.Equal(t, "moderator", p.GetRole())
	})

	t.Run("sets role", func(t *testing.T) {
		p := NewParticipant("p1", "room-1")
		p.SetRole("admin")
		assert.Equal(t, "admin", p.GetRole())
	})
}

func TestParticipant_SetPublishedTracks(t *testing.T) {
	t.Run("sets published tracks for reconnection", func(t *testing.T) {
		p := NewParticipant("p1", "room-1")
		p.SetState(ParticipantStateJoined)

		trackIDs := []string{"track-1", "track-2", "track-3"}
		p.SetPublishedTracks(trackIDs)

		tracks := p.GetPublishedTracks()
		assert.Len(t, tracks, 3)
		assert.Equal(t, ParticipantStatePublishing, p.GetState())
	})

	t.Run("replaces existing tracks", func(t *testing.T) {
		p := NewParticipant("p1", "room-1")
		p.AddPublishedTrack("old-track")

		p.SetPublishedTracks([]string{"new-track-1", "new-track-2"})

		tracks := p.GetPublishedTracks()
		assert.Len(t, tracks, 2)
		assert.Equal(t, "new-track-1", tracks[0])
		assert.Equal(t, "new-track-2", tracks[1])
	})

	t.Run("does not change state when empty tracks", func(t *testing.T) {
		p := NewParticipant("p1", "room-1")
		p.SetState(ParticipantStateJoined)

		p.SetPublishedTracks([]string{})

		assert.Equal(t, ParticipantStateJoined, p.GetState())
	})
}

func TestParticipant_SetSubscriptions(t *testing.T) {
	t.Run("sets subscriptions for reconnection", func(t *testing.T) {
		p := NewParticipant("p1", "room-1")
		p.SetState(ParticipantStateJoined)

		subIDs := []string{"sub-1", "sub-2", "sub-3"}
		p.SetSubscriptions(subIDs)

		subs := p.GetSubscriptions()
		assert.Len(t, subs, 3)
		assert.Equal(t, ParticipantStateSubscribing, p.GetState())
	})

	t.Run("replaces existing subscriptions", func(t *testing.T) {
		p := NewParticipant("p1", "room-1")
		p.AddSubscription("old-sub")

		p.SetSubscriptions([]string{"new-sub-1", "new-sub-2"})

		subs := p.GetSubscriptions()
		assert.Len(t, subs, 2)
		assert.Equal(t, "new-sub-1", subs[0])
		assert.Equal(t, "new-sub-2", subs[1])
	})

	t.Run("does not change state when empty subscriptions", func(t *testing.T) {
		p := NewParticipant("p1", "room-1")
		p.SetState(ParticipantStateJoined)

		p.SetSubscriptions([]string{})

		assert.Equal(t, ParticipantStateJoined, p.GetState())
	})
}

func TestParticipant_Concurrency(t *testing.T) {
	p := NewParticipant("p1", "room-1")

	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 5)

	for i := 0; i < numGoroutines; i++ {
		idx := i
		// State operations
		go func() {
			defer wg.Done()
			p.SetState(ParticipantStateJoined)
			p.GetState()
		}()

		// Track operations
		go func() {
			defer wg.Done()
			trackID := "track-" + string(rune('0'+idx%10))
			p.AddPublishedTrack(trackID)
			p.GetPublishedTracks()
			p.RemovePublishedTrack(trackID)
		}()

		// Subscription operations
		go func() {
			defer wg.Done()
			subID := "sub-" + string(rune('0'+idx%10))
			p.AddSubscription(subID)
			p.GetSubscriptions()
			p.RemoveSubscription(subID)
		}()

		// Metadata operations
		go func() {
			defer wg.Done()
			p.SetMetadata(map[string]interface{}{"key": idx})
			p.GetMetadata()
		}()

		// Role operations
		go func() {
			defer wg.Done()
			p.SetRole("role-" + string(rune('0'+idx%10)))
			p.GetRole()
		}()
	}

	wg.Wait()
	// Should not panic or race
}
