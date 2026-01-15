package auth

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Common errors for blacklist operations.
var (
	ErrTokenBlacklisted = errors.New("token is blacklisted")
	ErrBlacklistFailed  = errors.New("blacklist operation failed")
)

// BlacklistStore defines the interface for token blacklist storage.
type BlacklistStore interface {
	// Add adds a token to the blacklist with the given expiration time.
	// The token should be automatically removed after expiration.
	Add(ctx context.Context, tokenID string, expiration time.Time) error

	// IsBlacklisted checks if a token is in the blacklist.
	IsBlacklisted(ctx context.Context, tokenID string) (bool, error)

	// Remove removes a token from the blacklist.
	Remove(ctx context.Context, tokenID string) error

	// Count returns the number of tokens in the blacklist.
	Count(ctx context.Context) (int64, error)
}

// BlacklistConfig contains configuration for the token blacklist.
type BlacklistConfig struct {
	// CleanupInterval is how often to clean up expired entries (for in-memory store).
	CleanupInterval time.Duration
	// DefaultTTL is the default time-to-live for blacklist entries if no expiration is provided.
	DefaultTTL time.Duration
}

// DefaultBlacklistConfig returns the default blacklist configuration.
func DefaultBlacklistConfig() BlacklistConfig {
	return BlacklistConfig{
		CleanupInterval: 10 * time.Minute,
		DefaultTTL:      24 * time.Hour,
	}
}

// TokenBlacklist manages blacklisted JWT tokens.
type TokenBlacklist struct {
	store  BlacklistStore
	config BlacklistConfig
}

// NewTokenBlacklist creates a new token blacklist.
func NewTokenBlacklist(store BlacklistStore, cfg BlacklistConfig) *TokenBlacklist {
	return &TokenBlacklist{
		store:  store,
		config: cfg,
	}
}

// Revoke adds a token to the blacklist.
// The tokenID should be the JWT ID (jti claim) or a hash of the token.
// expiration is when the token would naturally expire (for automatic cleanup).
func (b *TokenBlacklist) Revoke(ctx context.Context, tokenID string, expiration time.Time) error {
	if tokenID == "" {
		return errors.New("token ID cannot be empty")
	}

	// Use default TTL if no expiration provided
	if expiration.IsZero() {
		expiration = time.Now().Add(b.config.DefaultTTL)
	}

	return b.store.Add(ctx, tokenID, expiration)
}

// IsRevoked checks if a token has been revoked.
func (b *TokenBlacklist) IsRevoked(ctx context.Context, tokenID string) (bool, error) {
	if tokenID == "" {
		return false, nil // Empty token ID is not considered blacklisted
	}
	return b.store.IsBlacklisted(ctx, tokenID)
}

// Unrevoke removes a token from the blacklist.
func (b *TokenBlacklist) Unrevoke(ctx context.Context, tokenID string) error {
	if tokenID == "" {
		return errors.New("token ID cannot be empty")
	}
	return b.store.Remove(ctx, tokenID)
}

// Count returns the number of blacklisted tokens.
func (b *TokenBlacklist) Count(ctx context.Context) (int64, error) {
	return b.store.Count(ctx)
}

// blacklistEntry represents a blacklist entry in memory.
type blacklistEntry struct {
	expiration time.Time
}

// InMemoryBlacklistStore is an in-memory implementation of BlacklistStore.
// Suitable for development and testing, or single-node deployments.
type InMemoryBlacklistStore struct {
	mu        sync.RWMutex
	entries   map[string]*blacklistEntry
	closeCh   chan struct{}
	closeOnce sync.Once
}

// NewInMemoryBlacklistStore creates a new in-memory blacklist store.
func NewInMemoryBlacklistStore(cleanupInterval time.Duration) *InMemoryBlacklistStore {
	store := &InMemoryBlacklistStore{
		entries: make(map[string]*blacklistEntry),
		closeCh: make(chan struct{}),
	}

	// Start cleanup goroutine if interval is positive
	if cleanupInterval > 0 {
		go store.cleanupLoop(cleanupInterval)
	}

	return store
}

// Close stops the cleanup goroutine.
func (s *InMemoryBlacklistStore) Close() {
	s.closeOnce.Do(func() {
		close(s.closeCh)
	})
}

// cleanupLoop periodically removes expired entries.
func (s *InMemoryBlacklistStore) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.closeCh:
			return
		}
	}
}

// cleanup removes all expired entries.
func (s *InMemoryBlacklistStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for tokenID, entry := range s.entries {
		if now.After(entry.expiration) {
			delete(s.entries, tokenID)
		}
	}
}

// Add adds a token to the blacklist.
func (s *InMemoryBlacklistStore) Add(_ context.Context, tokenID string, expiration time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[tokenID] = &blacklistEntry{
		expiration: expiration,
	}
	return nil
}

// IsBlacklisted checks if a token is blacklisted.
func (s *InMemoryBlacklistStore) IsBlacklisted(_ context.Context, tokenID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, exists := s.entries[tokenID]
	if !exists {
		return false, nil
	}

	// Check if entry has expired
	if time.Now().After(entry.expiration) {
		return false, nil
	}

	return true, nil
}

// Remove removes a token from the blacklist.
func (s *InMemoryBlacklistStore) Remove(_ context.Context, tokenID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.entries, tokenID)
	return nil
}

// Count returns the number of tokens in the blacklist (excluding expired ones).
func (s *InMemoryBlacklistStore) Count(_ context.Context) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	var count int64
	for _, entry := range s.entries {
		if now.Before(entry.expiration) {
			count++
		}
	}
	return count, nil
}

// ValidateWithBlacklist validates a token and checks if it's blacklisted.
// This is a helper function that combines JWT validation with blacklist checking.
func ValidateWithBlacklist(
	ctx context.Context,
	validator *JWTValidator,
	blacklist *TokenBlacklist,
	tokenString string,
) (*Claims, error) {
	// First validate the token
	claims, err := validator.Validate(ctx, tokenString)
	if err != nil {
		return nil, err
	}

	// Check if the token is blacklisted using the JWT ID
	tokenID := claims.ID
	if tokenID == "" {
		// If no JWT ID, use subject + issued at as a fallback identifier
		if claims.IssuedAt != nil {
			tokenID = claims.Subject + ":" + claims.IssuedAt.Time.Format(time.RFC3339)
		}
	}

	if tokenID != "" {
		isRevoked, err := blacklist.IsRevoked(ctx, tokenID)
		if err != nil {
			return nil, err
		}
		if isRevoked {
			return nil, ErrTokenBlacklisted
		}
	}

	return claims, nil
}
