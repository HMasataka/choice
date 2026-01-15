package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestJWK_ToRSAPublicKey(t *testing.T) {
	// Generate a test key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	publicKey := &privateKey.PublicKey

	// Create JWK from public key
	jwk := JWK{
		Kty: "RSA",
		Kid: "test-key-1",
		N:   base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(bigEndianBytes(publicKey.E)),
	}

	t.Run("valid RSA key", func(t *testing.T) {
		key, err := jwk.ToRSAPublicKey()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if key.N.Cmp(publicKey.N) != 0 {
			t.Error("modulus mismatch")
		}
		if key.E != publicKey.E {
			t.Errorf("exponent mismatch: got %d, want %d", key.E, publicKey.E)
		}
	})

	t.Run("unsupported key type", func(t *testing.T) {
		ecJWK := JWK{
			Kty: "EC",
			Kid: "ec-key",
		}
		_, err := ecJWK.ToRSAPublicKey()
		if err == nil {
			t.Error("expected error for EC key type")
		}
		if !contains(err.Error(), "unsupported key type") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("invalid modulus encoding", func(t *testing.T) {
		invalidJWK := JWK{
			Kty: "RSA",
			Kid: "invalid",
			N:   "not-valid-base64!!!",
			E:   jwk.E,
		}
		_, err := invalidJWK.ToRSAPublicKey()
		if err == nil {
			t.Error("expected error for invalid modulus")
		}
	})

	t.Run("invalid exponent encoding", func(t *testing.T) {
		invalidJWK := JWK{
			Kty: "RSA",
			Kid: "invalid",
			N:   jwk.N,
			E:   "not-valid-base64!!!",
		}
		_, err := invalidJWK.ToRSAPublicKey()
		if err == nil {
			t.Error("expected error for invalid exponent")
		}
	})

	t.Run("empty modulus", func(t *testing.T) {
		invalidJWK := JWK{
			Kty: "RSA",
			Kid: "empty-n",
			N:   "",
			E:   jwk.E,
		}
		_, err := invalidJWK.ToRSAPublicKey()
		if err == nil {
			t.Error("expected error for empty modulus")
		}
	})

	t.Run("empty exponent", func(t *testing.T) {
		invalidJWK := JWK{
			Kty: "RSA",
			Kid: "empty-e",
			N:   jwk.N,
			E:   "",
		}
		_, err := invalidJWK.ToRSAPublicKey()
		if err == nil {
			t.Error("expected error for empty exponent")
		}
	})

	t.Run("exponent zero", func(t *testing.T) {
		invalidJWK := JWK{
			Kty: "RSA",
			Kid: "zero-e",
			N:   jwk.N,
			E:   base64.RawURLEncoding.EncodeToString([]byte{0}),
		}
		_, err := invalidJWK.ToRSAPublicKey()
		if err == nil {
			t.Error("expected error for exponent = 0")
		}
	})

	t.Run("exponent one", func(t *testing.T) {
		invalidJWK := JWK{
			Kty: "RSA",
			Kid: "one-e",
			N:   jwk.N,
			E:   base64.RawURLEncoding.EncodeToString([]byte{1}),
		}
		_, err := invalidJWK.ToRSAPublicKey()
		if err == nil {
			t.Error("expected error for exponent = 1")
		}
	})

	t.Run("even exponent", func(t *testing.T) {
		invalidJWK := JWK{
			Kty: "RSA",
			Kid: "even-e",
			N:   jwk.N,
			E:   base64.RawURLEncoding.EncodeToString([]byte{4}),
		}
		_, err := invalidJWK.ToRSAPublicKey()
		if err == nil {
			t.Error("expected error for even exponent")
		}
	})

	t.Run("exponent too large", func(t *testing.T) {
		invalidJWK := JWK{
			Kty: "RSA",
			Kid: "large-e",
			N:   jwk.N,
			E:   base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3, 4, 5}), // 5 bytes
		}
		_, err := invalidJWK.ToRSAPublicKey()
		if err == nil {
			t.Error("expected error for oversized exponent")
		}
	})
}

func TestJWKSKeySource_GetKey(t *testing.T) {
	// Generate test keys
	key1, _ := rsa.GenerateKey(rand.Reader, 2048)
	key2, _ := rsa.GenerateKey(rand.Reader, 2048)

	jwks := JWKS{
		Keys: []JWK{
			{
				Kty: "RSA",
				Kid: "key-1",
				Alg: "RS256",
				Use: "sig",
				N:   base64.RawURLEncoding.EncodeToString(key1.PublicKey.N.Bytes()),
				E:   base64.RawURLEncoding.EncodeToString(bigEndianBytes(key1.PublicKey.E)),
			},
			{
				Kty: "RSA",
				Kid: "key-2",
				Alg: "RS256",
				Use: "sig",
				N:   base64.RawURLEncoding.EncodeToString(key2.PublicKey.N.Bytes()),
				E:   base64.RawURLEncoding.EncodeToString(bigEndianBytes(key2.PublicKey.E)),
			},
		},
	}

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	}))
	defer server.Close()

	cfg := JWKSConfig{
		URL:             server.URL,
		CacheTTL:        1 * time.Hour,
		RefreshInterval: 0, // Disable background refresh for tests
		RequestTimeout:  5 * time.Second,
		RetryCount:      1,
		RetryDelay:      10 * time.Millisecond,
	}

	ks := NewJWKSKeySource(cfg)
	defer ks.Close()

	ctx := context.Background()

	t.Run("get key by kid", func(t *testing.T) {
		key, err := ks.GetKey(ctx, "key-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if key.N.Cmp(key1.PublicKey.N) != 0 {
			t.Error("returned wrong key")
		}
	})

	t.Run("get second key by kid", func(t *testing.T) {
		key, err := ks.GetKey(ctx, "key-2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if key.N.Cmp(key2.PublicKey.N) != 0 {
			t.Error("returned wrong key")
		}
	})

	t.Run("key not found", func(t *testing.T) {
		_, err := ks.GetKey(ctx, "non-existent")
		if err == nil {
			t.Error("expected error for non-existent key")
		}
	})

	t.Run("empty kid with multiple keys", func(t *testing.T) {
		_, err := ks.GetKey(ctx, "")
		if err == nil {
			t.Error("expected error when no kid specified with multiple keys")
		}
	})

	t.Run("cached key", func(t *testing.T) {
		// First request populates cache
		_, _ = ks.GetKey(ctx, "key-1")

		// Check cache count
		if ks.CachedKeyCount() < 1 {
			t.Error("expected at least one cached key")
		}

		// Second request should use cache
		key, err := ks.GetKey(ctx, "key-1")
		if err != nil {
			t.Fatalf("unexpected error on cached request: %v", err)
		}
		if key == nil {
			t.Error("expected non-nil key from cache")
		}
	})
}

func TestJWKSKeySource_SingleKey(t *testing.T) {
	// Generate a single test key
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)

	jwks := JWKS{
		Keys: []JWK{
			{
				Kty: "RSA",
				Kid: "single-key",
				Alg: "RS256",
				Use: "sig",
				N:   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
				E:   base64.RawURLEncoding.EncodeToString(bigEndianBytes(privateKey.PublicKey.E)),
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	}))
	defer server.Close()

	cfg := JWKSConfig{
		URL:             server.URL,
		CacheTTL:        1 * time.Hour,
		RefreshInterval: 0,
		RequestTimeout:  5 * time.Second,
		RetryCount:      1,
		RetryDelay:      10 * time.Millisecond,
	}

	ks := NewJWKSKeySource(cfg)
	defer ks.Close()

	ctx := context.Background()

	t.Run("empty kid with single key returns that key", func(t *testing.T) {
		key, err := ks.GetKey(ctx, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if key.N.Cmp(privateKey.PublicKey.N) != 0 {
			t.Error("returned wrong key")
		}
	})
}

func TestJWKSKeySource_MultipleKeysEmptyKidAmbiguity(t *testing.T) {
	// Test that empty kid is rejected when multiple keys exist, even if one has empty kid
	key1, _ := rsa.GenerateKey(rand.Reader, 2048)
	key2, _ := rsa.GenerateKey(rand.Reader, 2048)

	// One key has empty kid (ambiguous case)
	jwks := JWKS{
		Keys: []JWK{
			{
				Kty: "RSA",
				Kid: "", // Empty kid
				N:   base64.RawURLEncoding.EncodeToString(key1.PublicKey.N.Bytes()),
				E:   base64.RawURLEncoding.EncodeToString(bigEndianBytes(key1.PublicKey.E)),
			},
			{
				Kty: "RSA",
				Kid: "key-2",
				N:   base64.RawURLEncoding.EncodeToString(key2.PublicKey.N.Bytes()),
				E:   base64.RawURLEncoding.EncodeToString(bigEndianBytes(key2.PublicKey.E)),
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	}))
	defer server.Close()

	cfg := JWKSConfig{
		URL:             server.URL,
		CacheTTL:        1 * time.Hour,
		RefreshInterval: 0,
		RequestTimeout:  5 * time.Second,
		RetryCount:      1,
		RetryDelay:      10 * time.Millisecond,
	}

	ks := NewJWKSKeySource(cfg)
	defer ks.Close()

	ctx := context.Background()

	t.Run("empty kid rejected with multiple keys even if one has empty kid", func(t *testing.T) {
		_, err := ks.GetKey(ctx, "")
		if err == nil {
			t.Error("expected error when empty kid requested with multiple keys")
		}
	})

	t.Run("specific kid still works", func(t *testing.T) {
		key, err := ks.GetKey(ctx, "key-2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key.N.Cmp(key2.PublicKey.N) != 0 {
			t.Error("returned wrong key")
		}
	})
}

func TestJWKSKeySource_FetchErrors(t *testing.T) {
	t.Run("server returns error status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		cfg := JWKSConfig{
			URL:            server.URL,
			RequestTimeout: 5 * time.Second,
			RetryCount:     1,
			RetryDelay:     10 * time.Millisecond,
		}

		ks := NewJWKSKeySource(cfg)
		defer ks.Close()

		_, err := ks.GetKey(context.Background(), "test")
		if err == nil {
			t.Error("expected error for server error")
		}
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("not valid json"))
		}))
		defer server.Close()

		cfg := JWKSConfig{
			URL:            server.URL,
			RequestTimeout: 5 * time.Second,
			RetryCount:     1,
			RetryDelay:     10 * time.Millisecond,
		}

		ks := NewJWKSKeySource(cfg)
		defer ks.Close()

		_, err := ks.GetKey(context.Background(), "test")
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("no URL configured", func(t *testing.T) {
		cfg := JWKSConfig{
			URL:            "",
			RequestTimeout: 5 * time.Second,
		}

		ks := NewJWKSKeySource(cfg)
		defer ks.Close()

		_, err := ks.GetKey(context.Background(), "test")
		if err == nil {
			t.Error("expected error for missing URL")
		}
	})

	t.Run("context canceled", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(5 * time.Second) // Slow response
		}))
		defer server.Close()

		cfg := JWKSConfig{
			URL:            server.URL,
			RequestTimeout: 1 * time.Second,
			RetryCount:     0,
			RetryDelay:     10 * time.Millisecond,
		}

		ks := NewJWKSKeySource(cfg)
		defer ks.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		_, err := ks.GetKey(ctx, "test")
		if err == nil {
			t.Error("expected error for canceled context")
		}
	})
}

func TestJWKSKeySource_Retry(t *testing.T) {
	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		// Generate a key for the successful response
		privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
		jwks := JWKS{
			Keys: []JWK{
				{
					Kty: "RSA",
					Kid: "retry-key",
					N:   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
					E:   base64.RawURLEncoding.EncodeToString(bigEndianBytes(privateKey.PublicKey.E)),
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	}))
	defer server.Close()

	cfg := JWKSConfig{
		URL:            server.URL,
		RequestTimeout: 5 * time.Second,
		RetryCount:     3,
		RetryDelay:     10 * time.Millisecond,
	}

	ks := NewJWKSKeySource(cfg)
	defer ks.Close()

	key, err := ks.GetKey(context.Background(), "retry-key")
	if err != nil {
		t.Fatalf("unexpected error after retries: %v", err)
	}

	if key == nil {
		t.Error("expected non-nil key")
	}

	if attempts < 3 {
		t.Errorf("expected at least 3 attempts, got %d", attempts)
	}
}

func TestJWKSKeySource_CacheExpiry(t *testing.T) {
	fetchCount := 0
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount++
		jwks := JWKS{
			Keys: []JWK{
				{
					Kty: "RSA",
					Kid: "cache-key",
					N:   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
					E:   base64.RawURLEncoding.EncodeToString(bigEndianBytes(privateKey.PublicKey.E)),
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	}))
	defer server.Close()

	cfg := JWKSConfig{
		URL:             server.URL,
		CacheTTL:        50 * time.Millisecond, // Short TTL for testing
		RefreshInterval: 0,
		RequestTimeout:  5 * time.Second,
		RetryCount:      0,
	}

	ks := NewJWKSKeySource(cfg)
	defer ks.Close()

	ctx := context.Background()

	// First request
	_, err := ks.GetKey(ctx, "cache-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second request (should use cache)
	_, err = ks.GetKey(ctx, "cache-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fetchCount != 1 {
		t.Errorf("expected 1 fetch (cached), got %d", fetchCount)
	}

	// Wait for cache to expire
	time.Sleep(60 * time.Millisecond)

	// Third request (cache expired, should fetch again)
	_, err = ks.GetKey(ctx, "cache-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fetchCount != 2 {
		t.Errorf("expected 2 fetches after cache expiry, got %d", fetchCount)
	}
}

func TestJWKSKeySource_ForceRefresh(t *testing.T) {
	fetchCount := 0
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount++
		jwks := JWKS{
			Keys: []JWK{
				{
					Kty: "RSA",
					Kid: "refresh-key",
					N:   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
					E:   base64.RawURLEncoding.EncodeToString(bigEndianBytes(privateKey.PublicKey.E)),
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	}))
	defer server.Close()

	cfg := JWKSConfig{
		URL:             server.URL,
		CacheTTL:        1 * time.Hour, // Long TTL
		RefreshInterval: 0,
		RequestTimeout:  5 * time.Second,
		RetryCount:      0,
	}

	ks := NewJWKSKeySource(cfg)
	defer ks.Close()

	ctx := context.Background()

	// Initial fetch
	_, _ = ks.GetKey(ctx, "refresh-key")

	// Force refresh
	err := ks.ForceRefresh(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have fetched twice
	if fetchCount != 2 {
		t.Errorf("expected 2 fetches after force refresh, got %d", fetchCount)
	}

	// Check last fetch time was updated
	if ks.LastFetchTime().IsZero() {
		t.Error("expected non-zero last fetch time")
	}
}

func TestDefaultJWKSConfig(t *testing.T) {
	cfg := DefaultJWKSConfig()

	if cfg.CacheTTL != 1*time.Hour {
		t.Errorf("expected CacheTTL 1h, got %v", cfg.CacheTTL)
	}
	if cfg.RefreshInterval != 55*time.Minute {
		t.Errorf("expected RefreshInterval 55m, got %v", cfg.RefreshInterval)
	}
	if cfg.RequestTimeout != 10*time.Second {
		t.Errorf("expected RequestTimeout 10s, got %v", cfg.RequestTimeout)
	}
	if cfg.RetryCount != 3 {
		t.Errorf("expected RetryCount 3, got %d", cfg.RetryCount)
	}
	if cfg.RetryDelay != 1*time.Second {
		t.Errorf("expected RetryDelay 1s, got %v", cfg.RetryDelay)
	}
}

// bigEndianBytes converts an int to big-endian bytes.
func bigEndianBytes(e int) []byte {
	if e == 0 {
		return []byte{0}
	}
	var bytes []byte
	for e > 0 {
		bytes = append([]byte{byte(e & 0xff)}, bytes...)
		e >>= 8
	}
	return bytes
}
