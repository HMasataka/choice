package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRedisClient is a mock implementation of RedisClient for testing.
type mockRedisClient struct {
	data   map[string]string
	sets   map[string]map[string]bool
	hashes map[string]map[string]string
}

func newMockRedisClient() *mockRedisClient {
	return &mockRedisClient{
		data:   make(map[string]string),
		sets:   make(map[string]map[string]bool),
		hashes: make(map[string]map[string]string),
	}
}

func (m *mockRedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	switch v := value.(type) {
	case string:
		m.data[key] = v
	case []byte:
		m.data[key] = string(v)
	default:
		m.data[key] = fmt.Sprintf("%v", v)
	}
	return nil
}

func (m *mockRedisClient) Get(ctx context.Context, key string) (string, error) {
	val, exists := m.data[key]
	if !exists {
		return "", nil
	}
	return val, nil
}

func (m *mockRedisClient) Del(ctx context.Context, keys ...string) error {
	for _, key := range keys {
		delete(m.data, key)
		delete(m.hashes, key)
	}
	return nil
}

func (m *mockRedisClient) SAdd(ctx context.Context, key string, members ...interface{}) error {
	if m.sets[key] == nil {
		m.sets[key] = make(map[string]bool)
	}
	for _, member := range members {
		m.sets[key][member.(string)] = true
	}
	return nil
}

func (m *mockRedisClient) SMembers(ctx context.Context, key string) ([]string, error) {
	set, exists := m.sets[key]
	if !exists {
		return []string{}, nil
	}
	members := make([]string, 0, len(set))
	for member := range set {
		members = append(members, member)
	}
	return members, nil
}

func (m *mockRedisClient) SRem(ctx context.Context, key string, members ...interface{}) error {
	if m.sets[key] == nil {
		return nil
	}
	for _, member := range members {
		delete(m.sets[key], member.(string))
	}
	return nil
}

func (m *mockRedisClient) Expire(ctx context.Context, key string, expiration time.Duration) error {
	// Mock implementation: In real tests, we would track TTL, but for unit tests we just succeed
	return nil
}

func (m *mockRedisClient) Exists(ctx context.Context, keys ...string) (int64, error) {
	var count int64
	for _, key := range keys {
		if _, exists := m.data[key]; exists {
			count++
		} else if _, exists := m.hashes[key]; exists {
			count++
		}
	}
	return count, nil
}

func (m *mockRedisClient) HSet(ctx context.Context, key string, values ...interface{}) error {
	if m.hashes[key] == nil {
		m.hashes[key] = make(map[string]string)
	}
	for i := 0; i < len(values); i += 2 {
		field := fmt.Sprintf("%v", values[i])
		value := fmt.Sprintf("%v", values[i+1])
		m.hashes[key][field] = value
	}
	return nil
}

func (m *mockRedisClient) HGet(ctx context.Context, key string, field string) (string, error) {
	hash, exists := m.hashes[key]
	if !exists {
		return "", nil
	}
	return hash[field], nil
}

func (m *mockRedisClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	hash, exists := m.hashes[key]
	if !exists {
		return map[string]string{}, nil
	}
	result := make(map[string]string, len(hash))
	for k, v := range hash {
		result[k] = v
	}
	return result, nil
}

func (m *mockRedisClient) HIncrBy(ctx context.Context, key string, field string, incr int64) (int64, error) {
	if m.hashes[key] == nil {
		m.hashes[key] = make(map[string]string)
	}
	current := int64(0)
	if val, exists := m.hashes[key][field]; exists {
		var err error
		current, err = parseInt64(val)
		if err != nil {
			return 0, err
		}
	}
	newVal := current + incr
	m.hashes[key][field] = fmt.Sprintf("%d", newVal)
	return newVal, nil
}

func (m *mockRedisClient) Keys(ctx context.Context, pattern string) ([]string, error) {
	// Simple pattern matching for testing (only supports * at the end)
	var keys []string
	prefix := pattern[:len(pattern)-1]
	for key := range m.data {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			keys = append(keys, key)
		}
	}
	for key := range m.hashes {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func parseInt64(s string) (int64, error) {
	var v int64
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}

func (m *mockRedisClient) Close() error {
	return nil
}

func TestRedisStore_SaveSession(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisStore(client, "test:session:")
	defer store.Close()

	ctx := context.Background()
	session := &Session{
		SessionID:       "session-1",
		ParticipantID:   "participant-1",
		RoomID:          "room-1",
		PublishedTracks: []string{"track-1"},
		Subscriptions:   []string{"sub-1"},
		Metadata:        map[string]interface{}{"key": "value"},
		UserAgent:       "test-agent",
		IPAddress:       "127.0.0.1",
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(1 * time.Hour),
	}

	err := store.SaveSession(ctx, session)
	require.NoError(t, err)

	// Retrieve the session
	retrieved, err := store.GetSession(ctx, "session-1")
	require.NoError(t, err)
	assert.Equal(t, session.SessionID, retrieved.SessionID)
	assert.Equal(t, session.ParticipantID, retrieved.ParticipantID)
	assert.Equal(t, session.RoomID, retrieved.RoomID)
}

func TestRedisStore_GetSession(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisStore(client, "test:session:")
	defer store.Close()

	ctx := context.Background()

	t.Run("returns error for non-existent session", func(t *testing.T) {
		_, err := store.GetSession(ctx, "nonexistent")
		assert.Equal(t, ErrSessionNotFound, err)
	})

	t.Run("returns error for expired session", func(t *testing.T) {
		// Note: For expired sessions, SaveSession will return an error
		// because TTL will be negative
		session := &Session{
			SessionID:     "session-expired",
			ParticipantID: "participant-1",
			RoomID:        "room-1",
			CreatedAt:     time.Now().Add(-2 * time.Hour),
			ExpiresAt:     time.Now().Add(-1 * time.Hour),
		}

		err := store.SaveSession(ctx, session)
		assert.Equal(t, ErrSessionExpired, err)
	})
}

func TestRedisStore_DeleteSession(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisStore(client, "test:session:")
	defer store.Close()

	ctx := context.Background()
	session := &Session{
		SessionID:     "session-1",
		ParticipantID: "participant-1",
		RoomID:        "room-1",
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(1 * time.Hour),
	}

	err := store.SaveSession(ctx, session)
	require.NoError(t, err)

	err = store.DeleteSession(ctx, "session-1")
	require.NoError(t, err)

	_, err = store.GetSession(ctx, "session-1")
	assert.Equal(t, ErrSessionNotFound, err)
}

func TestRedisStore_UpdateSession(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisStore(client, "test:session:")
	defer store.Close()

	ctx := context.Background()
	session := &Session{
		SessionID:       "session-1",
		ParticipantID:   "participant-1",
		RoomID:          "room-1",
		PublishedTracks: []string{"track-1"},
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(1 * time.Hour),
	}

	err := store.SaveSession(ctx, session)
	require.NoError(t, err)

	// Update session
	session.PublishedTracks = append(session.PublishedTracks, "track-2")
	err = store.UpdateSession(ctx, session)
	require.NoError(t, err)

	// Retrieve updated session
	retrieved, err := store.GetSession(ctx, "session-1")
	require.NoError(t, err)
	assert.Len(t, retrieved.PublishedTracks, 2)
}

func TestRedisStore_GetSessionsByParticipant(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisStore(client, "test:session:")
	defer store.Close()

	ctx := context.Background()
	session1 := &Session{
		SessionID:     "session-1",
		ParticipantID: "participant-1",
		RoomID:        "room-1",
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(1 * time.Hour),
	}
	session2 := &Session{
		SessionID:     "session-2",
		ParticipantID: "participant-1",
		RoomID:        "room-2",
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(1 * time.Hour),
	}

	err := store.SaveSession(ctx, session1)
	require.NoError(t, err)
	err = store.SaveSession(ctx, session2)
	require.NoError(t, err)

	sessions, err := store.GetSessionsByParticipant(ctx, "participant-1")
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
}

func TestRedisStore_GetSessionsByRoom(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisStore(client, "test:session:")
	defer store.Close()

	ctx := context.Background()
	session1 := &Session{
		SessionID:     "session-1",
		ParticipantID: "participant-1",
		RoomID:        "room-1",
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(1 * time.Hour),
	}
	session2 := &Session{
		SessionID:     "session-2",
		ParticipantID: "participant-2",
		RoomID:        "room-1",
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(1 * time.Hour),
	}

	err := store.SaveSession(ctx, session1)
	require.NoError(t, err)
	err = store.SaveSession(ctx, session2)
	require.NoError(t, err)

	sessions, err := store.GetSessionsByRoom(ctx, "room-1")
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
}

// TestRedisStore_Integration tests RedisStore with a real Redis instance.
// This test is skipped by default and should be run manually when a Redis instance is available.
func TestRedisStore_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Create a real Redis client
	client, err := NewGoRedisClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   1, // Use a separate database for testing
	})
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}
	defer client.Close()

	store := NewRedisStore(client, "test:integration:")
	defer store.Close()

	ctx := context.Background()

	// Clean up before test
	_ = client.Del(ctx, "test:integration:session-1")

	session := &Session{
		SessionID:       "session-1",
		ParticipantID:   "participant-1",
		RoomID:          "room-1",
		PublishedTracks: []string{"track-1"},
		Subscriptions:   []string{"sub-1"},
		Metadata:        map[string]interface{}{"key": "value"},
		UserAgent:       "test-agent",
		IPAddress:       "127.0.0.1",
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(30 * time.Second),
	}

	// Save session
	err = store.SaveSession(ctx, session)
	require.NoError(t, err)

	// Retrieve session
	retrieved, err := store.GetSession(ctx, "session-1")
	require.NoError(t, err)
	assert.Equal(t, session.SessionID, retrieved.SessionID)
	assert.Equal(t, session.ParticipantID, retrieved.ParticipantID)
	assert.Equal(t, session.RoomID, retrieved.RoomID)

	// Clean up after test
	_ = store.DeleteSession(ctx, "session-1")
}
