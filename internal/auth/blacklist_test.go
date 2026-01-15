package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestInMemoryBlacklistStore(t *testing.T) {
	t.Run("add and check blacklisted", func(t *testing.T) {
		store := NewInMemoryBlacklistStore(0) // No cleanup for test
		defer store.Close()

		ctx := context.Background()
		tokenID := "test-token-123"
		expiration := time.Now().Add(time.Hour)

		// Add token
		err := store.Add(ctx, tokenID, expiration)
		if err != nil {
			t.Fatalf("failed to add token: %v", err)
		}

		// Check if blacklisted
		blacklisted, err := store.IsBlacklisted(ctx, tokenID)
		if err != nil {
			t.Fatalf("failed to check blacklist: %v", err)
		}
		if !blacklisted {
			t.Error("expected token to be blacklisted")
		}
	})

	t.Run("non-blacklisted token", func(t *testing.T) {
		store := NewInMemoryBlacklistStore(0)
		defer store.Close()

		ctx := context.Background()

		blacklisted, err := store.IsBlacklisted(ctx, "non-existent")
		if err != nil {
			t.Fatalf("failed to check blacklist: %v", err)
		}
		if blacklisted {
			t.Error("expected token to not be blacklisted")
		}
	})

	t.Run("remove from blacklist", func(t *testing.T) {
		store := NewInMemoryBlacklistStore(0)
		defer store.Close()

		ctx := context.Background()
		tokenID := "test-token-456"

		store.Add(ctx, tokenID, time.Now().Add(time.Hour))

		// Remove
		err := store.Remove(ctx, tokenID)
		if err != nil {
			t.Fatalf("failed to remove token: %v", err)
		}

		// Check it's no longer blacklisted
		blacklisted, err := store.IsBlacklisted(ctx, tokenID)
		if err != nil {
			t.Fatalf("failed to check blacklist: %v", err)
		}
		if blacklisted {
			t.Error("expected token to not be blacklisted after removal")
		}
	})

	t.Run("expired entry not blacklisted", func(t *testing.T) {
		store := NewInMemoryBlacklistStore(0)
		defer store.Close()

		ctx := context.Background()
		tokenID := "expired-token"

		// Add with past expiration
		store.Add(ctx, tokenID, time.Now().Add(-time.Hour))

		blacklisted, err := store.IsBlacklisted(ctx, tokenID)
		if err != nil {
			t.Fatalf("failed to check blacklist: %v", err)
		}
		if blacklisted {
			t.Error("expected expired token to not be blacklisted")
		}
	})

	t.Run("count excludes expired entries", func(t *testing.T) {
		store := NewInMemoryBlacklistStore(0)
		defer store.Close()

		ctx := context.Background()

		// Add valid entries
		store.Add(ctx, "valid-1", time.Now().Add(time.Hour))
		store.Add(ctx, "valid-2", time.Now().Add(time.Hour))

		// Add expired entry
		store.Add(ctx, "expired", time.Now().Add(-time.Hour))

		count, err := store.Count(ctx)
		if err != nil {
			t.Fatalf("failed to count: %v", err)
		}
		if count != 2 {
			t.Errorf("expected count 2, got %d", count)
		}
	})

	t.Run("cleanup removes expired entries", func(t *testing.T) {
		store := NewInMemoryBlacklistStore(0) // Manual cleanup
		defer store.Close()

		ctx := context.Background()

		// Add entry that will expire
		store.Add(ctx, "soon-expired", time.Now().Add(10*time.Millisecond))
		store.Add(ctx, "valid", time.Now().Add(time.Hour))

		// Wait for expiration
		time.Sleep(20 * time.Millisecond)

		// Manual cleanup
		store.cleanup()

		// Check counts
		count, _ := store.Count(ctx)
		if count != 1 {
			t.Errorf("expected count 1 after cleanup, got %d", count)
		}
	})
}

func TestTokenBlacklist(t *testing.T) {
	cfg := DefaultBlacklistConfig()

	t.Run("revoke and check", func(t *testing.T) {
		store := NewInMemoryBlacklistStore(0)
		defer store.Close()
		blacklist := NewTokenBlacklist(store, cfg)

		ctx := context.Background()
		tokenID := "jwt-id-123"

		err := blacklist.Revoke(ctx, tokenID, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatalf("failed to revoke: %v", err)
		}

		revoked, err := blacklist.IsRevoked(ctx, tokenID)
		if err != nil {
			t.Fatalf("failed to check revoked: %v", err)
		}
		if !revoked {
			t.Error("expected token to be revoked")
		}
	})

	t.Run("revoke with empty token ID fails", func(t *testing.T) {
		store := NewInMemoryBlacklistStore(0)
		defer store.Close()
		blacklist := NewTokenBlacklist(store, cfg)

		ctx := context.Background()

		err := blacklist.Revoke(ctx, "", time.Now().Add(time.Hour))
		if err == nil {
			t.Error("expected error for empty token ID")
		}
	})

	t.Run("empty token ID returns not revoked", func(t *testing.T) {
		store := NewInMemoryBlacklistStore(0)
		defer store.Close()
		blacklist := NewTokenBlacklist(store, cfg)

		ctx := context.Background()

		revoked, err := blacklist.IsRevoked(ctx, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if revoked {
			t.Error("expected empty token ID to not be revoked")
		}
	})

	t.Run("unrevoke removes token", func(t *testing.T) {
		store := NewInMemoryBlacklistStore(0)
		defer store.Close()
		blacklist := NewTokenBlacklist(store, cfg)

		ctx := context.Background()
		tokenID := "to-unrevoke"

		blacklist.Revoke(ctx, tokenID, time.Now().Add(time.Hour))

		err := blacklist.Unrevoke(ctx, tokenID)
		if err != nil {
			t.Fatalf("failed to unrevoke: %v", err)
		}

		revoked, _ := blacklist.IsRevoked(ctx, tokenID)
		if revoked {
			t.Error("expected token to not be revoked after unrevoke")
		}
	})

	t.Run("revoke with zero expiration uses default TTL", func(t *testing.T) {
		store := NewInMemoryBlacklistStore(0)
		defer store.Close()
		blacklist := NewTokenBlacklist(store, cfg)

		ctx := context.Background()
		tokenID := "default-ttl-token"

		err := blacklist.Revoke(ctx, tokenID, time.Time{}) // Zero time
		if err != nil {
			t.Fatalf("failed to revoke: %v", err)
		}

		// Token should be revoked
		revoked, _ := blacklist.IsRevoked(ctx, tokenID)
		if !revoked {
			t.Error("expected token to be revoked")
		}
	})

	t.Run("count returns blacklist size", func(t *testing.T) {
		store := NewInMemoryBlacklistStore(0)
		defer store.Close()
		blacklist := NewTokenBlacklist(store, cfg)

		ctx := context.Background()

		blacklist.Revoke(ctx, "token-1", time.Now().Add(time.Hour))
		blacklist.Revoke(ctx, "token-2", time.Now().Add(time.Hour))

		count, err := blacklist.Count(ctx)
		if err != nil {
			t.Fatalf("failed to count: %v", err)
		}
		if count != 2 {
			t.Errorf("expected count 2, got %d", count)
		}
	})
}

func TestValidateWithBlacklist(t *testing.T) {
	// Generate key pair for testing
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	keySource := NewStaticKeySource(&privateKey.PublicKey)
	validator := NewJWTValidator(DefaultJWTConfig(), keySource)

	cfg := DefaultBlacklistConfig()

	t.Run("valid token not blacklisted", func(t *testing.T) {
		store := NewInMemoryBlacklistStore(0)
		defer store.Close()
		blacklist := NewTokenBlacklist(store, cfg)

		ctx := context.Background()

		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "user-123",
				ID:        "jwt-id-abc",
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}

		tokenString, _ := GenerateToken(claims, privateKey)

		result, err := ValidateWithBlacklist(ctx, validator, blacklist, tokenString)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Subject != "user-123" {
			t.Errorf("expected subject user-123, got %s", result.Subject)
		}
	})

	t.Run("blacklisted token rejected", func(t *testing.T) {
		store := NewInMemoryBlacklistStore(0)
		defer store.Close()
		blacklist := NewTokenBlacklist(store, cfg)

		ctx := context.Background()
		tokenID := "blacklisted-jwt-id"

		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "user-123",
				ID:        tokenID,
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}

		tokenString, _ := GenerateToken(claims, privateKey)

		// Blacklist the token
		blacklist.Revoke(ctx, tokenID, time.Now().Add(time.Hour))

		_, err := ValidateWithBlacklist(ctx, validator, blacklist, tokenString)
		if err == nil {
			t.Error("expected error for blacklisted token")
		}
		if err != ErrTokenBlacklisted {
			t.Errorf("expected ErrTokenBlacklisted, got %v", err)
		}
	})

	t.Run("token without jti uses fallback ID", func(t *testing.T) {
		store := NewInMemoryBlacklistStore(0)
		defer store.Close()
		blacklist := NewTokenBlacklist(store, cfg)

		ctx := context.Background()
		issuedAt := time.Now()

		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "user-456",
				ID:        "", // No JWT ID
				IssuedAt:  jwt.NewNumericDate(issuedAt),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}

		tokenString, _ := GenerateToken(claims, privateKey)

		// Blacklist using fallback ID format
		fallbackID := "user-456:" + issuedAt.Format(time.RFC3339)
		blacklist.Revoke(ctx, fallbackID, time.Now().Add(time.Hour))

		_, err := ValidateWithBlacklist(ctx, validator, blacklist, tokenString)
		if err == nil {
			t.Error("expected error for blacklisted token")
		}
		if err != ErrTokenBlacklisted {
			t.Errorf("expected ErrTokenBlacklisted, got %v", err)
		}
	})

	t.Run("invalid token rejected before blacklist check", func(t *testing.T) {
		store := NewInMemoryBlacklistStore(0)
		defer store.Close()
		blacklist := NewTokenBlacklist(store, cfg)

		ctx := context.Background()

		_, err := ValidateWithBlacklist(ctx, validator, blacklist, "invalid.token.string")
		if err == nil {
			t.Error("expected error for invalid token")
		}
		// Should be a validation error, not a blacklist error
		if err == ErrTokenBlacklisted {
			t.Error("expected validation error, not blacklist error")
		}
	})
}

func TestDefaultBlacklistConfig(t *testing.T) {
	cfg := DefaultBlacklistConfig()

	if cfg.CleanupInterval != 10*time.Minute {
		t.Errorf("expected CleanupInterval 10m, got %v", cfg.CleanupInterval)
	}
	if cfg.DefaultTTL != 24*time.Hour {
		t.Errorf("expected DefaultTTL 24h, got %v", cfg.DefaultTTL)
	}
}

func TestInMemoryBlacklistStore_CleanupLoop(t *testing.T) {
	// Test that cleanup loop works without panic
	store := NewInMemoryBlacklistStore(50 * time.Millisecond)

	ctx := context.Background()
	store.Add(ctx, "short-lived", time.Now().Add(10*time.Millisecond))
	store.Add(ctx, "long-lived", time.Now().Add(time.Hour))

	// Wait for cleanup
	time.Sleep(100 * time.Millisecond)

	// Short-lived should be cleaned up
	count, _ := store.Count(ctx)
	if count != 1 {
		t.Errorf("expected count 1 after cleanup, got %d", count)
	}

	store.Close()
}
