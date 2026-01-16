package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// Common errors for JWKS operations.
var (
	ErrKeyNotFound        = errors.New("key not found")
	ErrInvalidJWKS        = errors.New("invalid JWKS response")
	ErrInvalidKey         = errors.New("invalid key format")
	ErrFetchFailed        = errors.New("failed to fetch JWKS")
	ErrUnsupportedKeyType = errors.New("unsupported key type")
)

// JWKSConfig contains configuration for JWKS fetching.
type JWKSConfig struct {
	// URL is the JWKS endpoint URL.
	URL string
	// CacheTTL is how long to cache the JWKS (default: 1 hour).
	CacheTTL time.Duration
	// RefreshInterval is how often to refresh the cache proactively (default: 55 minutes).
	RefreshInterval time.Duration
	// RequestTimeout is the HTTP request timeout (default: 10 seconds).
	RequestTimeout time.Duration
	// RetryCount is the number of retries on failure (default: 3).
	RetryCount int
	// RetryDelay is the initial delay between retries (default: 1 second).
	RetryDelay time.Duration
}

// DefaultJWKSConfig returns the default JWKS configuration.
func DefaultJWKSConfig() JWKSConfig {
	return JWKSConfig{
		CacheTTL:        1 * time.Hour,
		RefreshInterval: 55 * time.Minute,
		RequestTimeout:  10 * time.Second,
		RetryCount:      3,
		RetryDelay:      1 * time.Second,
	}
}

// JWKS represents a JSON Web Key Set.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWK represents a JSON Web Key.
type JWK struct {
	// Key type (e.g., "RSA")
	Kty string `json:"kty"`
	// Key use (e.g., "sig" for signature)
	Use string `json:"use,omitempty"`
	// Algorithm (e.g., "RS256")
	Alg string `json:"alg,omitempty"`
	// Key ID
	Kid string `json:"kid,omitempty"`
	// RSA modulus (Base64URL encoded)
	N string `json:"n,omitempty"`
	// RSA exponent (Base64URL encoded)
	E string `json:"e,omitempty"`
}

// ToRSAPublicKey converts a JWK to an RSA public key.
func (j *JWK) ToRSAPublicKey() (*rsa.PublicKey, error) {
	if j.Kty != "RSA" {
		return nil, fmt.Errorf("%w: expected RSA, got %s", ErrUnsupportedKeyType, j.Kty)
	}

	// Check for empty modulus/exponent before decoding
	if j.N == "" {
		return nil, fmt.Errorf("%w: modulus (n) is empty", ErrInvalidKey)
	}
	if j.E == "" {
		return nil, fmt.Errorf("%w: exponent (e) is empty", ErrInvalidKey)
	}

	// Decode modulus
	nBytes, err := base64.RawURLEncoding.DecodeString(j.N)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode modulus: %w", ErrInvalidKey, err)
	}
	if len(nBytes) == 0 {
		return nil, fmt.Errorf("%w: modulus decodes to empty bytes", ErrInvalidKey)
	}
	n := new(big.Int).SetBytes(nBytes)
	if n.Sign() <= 0 {
		return nil, fmt.Errorf("%w: modulus must be positive", ErrInvalidKey)
	}

	// Decode exponent
	eBytes, err := base64.RawURLEncoding.DecodeString(j.E)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode exponent: %w", ErrInvalidKey, err)
	}
	if len(eBytes) == 0 {
		return nil, fmt.Errorf("%w: exponent decodes to empty bytes", ErrInvalidKey)
	}

	// Check for exponent overflow (max 4 bytes for int on 32-bit systems)
	if len(eBytes) > 4 {
		return nil, fmt.Errorf("%w: exponent too large", ErrInvalidKey)
	}

	// Convert exponent bytes to int
	var e int
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}

	// Validate exponent value (must be >= 3 and odd for RSA)
	if e < 3 {
		return nil, fmt.Errorf("%w: exponent must be >= 3, got %d", ErrInvalidKey, e)
	}
	if e%2 == 0 {
		return nil, fmt.Errorf("%w: exponent must be odd, got %d", ErrInvalidKey, e)
	}

	return &rsa.PublicKey{
		N: n,
		E: e,
	}, nil
}

// cachedKey represents a cached RSA public key.
type cachedKey struct {
	key       *rsa.PublicKey
	fetchedAt time.Time
}

// JWKSKeySource fetches and caches public keys from a JWKS endpoint.
type JWKSKeySource struct {
	config     JWKSConfig
	httpClient *http.Client
	mu         sync.RWMutex
	cache      map[string]*cachedKey
	allKeys    *JWKS
	lastFetch  time.Time
	closeCh    chan struct{}
	closeOnce  sync.Once
}

// NewJWKSKeySource creates a new JWKS key source.
func NewJWKSKeySource(cfg JWKSConfig) *JWKSKeySource {
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = DefaultJWKSConfig().CacheTTL
	}
	if cfg.RefreshInterval == 0 {
		cfg.RefreshInterval = DefaultJWKSConfig().RefreshInterval
	}
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = DefaultJWKSConfig().RequestTimeout
	}
	if cfg.RetryCount == 0 {
		cfg.RetryCount = DefaultJWKSConfig().RetryCount
	}
	if cfg.RetryDelay == 0 {
		cfg.RetryDelay = DefaultJWKSConfig().RetryDelay
	}

	ks := &JWKSKeySource{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
		cache:   make(map[string]*cachedKey),
		closeCh: make(chan struct{}),
	}

	// Start background refresh if URL is configured
	if cfg.URL != "" && cfg.RefreshInterval > 0 {
		go ks.refreshLoop()
	}

	return ks
}

// Close stops the background refresh goroutine.
func (ks *JWKSKeySource) Close() {
	ks.closeOnce.Do(func() {
		close(ks.closeCh)
	})
}

// refreshLoop periodically refreshes the JWKS cache.
func (ks *JWKSKeySource) refreshLoop() {
	ticker := time.NewTicker(ks.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), ks.config.RequestTimeout)
			//nolint:errcheck // Ignore errors in background refresh
			ks.fetchJWKS(ctx)
			cancel()
		case <-ks.closeCh:
			return
		}
	}
}

// GetKey returns the public key for the given key ID.
// If kid is empty and only one key exists, returns that key.
// Implements the KeySource interface.
func (ks *JWKSKeySource) GetKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	// Check cache first (only for non-empty kid)
	ks.mu.RLock()
	if kid != "" {
		if cached, ok := ks.cache[kid]; ok {
			if time.Since(cached.fetchedAt) < ks.config.CacheTTL {
				ks.mu.RUnlock()
				return cached.key, nil
			}
		}
	} else {
		// For empty kid, only use cache if exactly one key exists
		// This prevents ambiguity when multiple keys exist
		if ks.allKeys != nil && len(ks.allKeys.Keys) == 1 {
			singleKey := ks.allKeys.Keys[0]
			if cached, ok := ks.cache[singleKey.Kid]; ok {
				if time.Since(cached.fetchedAt) < ks.config.CacheTTL {
					ks.mu.RUnlock()
					return cached.key, nil
				}
			}
		}
	}
	ks.mu.RUnlock()

	// Fetch JWKS
	jwks, err := ks.fetchJWKS(ctx)
	if err != nil {
		return nil, err
	}

	// Find the key
	return ks.findKey(jwks, kid)
}

// fetchJWKS fetches the JWKS from the configured URL with retry logic.
func (ks *JWKSKeySource) fetchJWKS(ctx context.Context) (*JWKS, error) {
	if ks.config.URL == "" {
		return nil, fmt.Errorf("%w: JWKS URL not configured", ErrFetchFailed)
	}

	var lastErr error
	delay := ks.config.RetryDelay

	for i := 0; i <= ks.config.RetryCount; i++ {
		if i > 0 {
			select {
			case <-time.After(delay):
				delay *= 2 // Exponential backoff
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		jwks, err := ks.doFetch(ctx)
		if err == nil {
			return jwks, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("%w: after %d retries: %w", ErrFetchFailed, ks.config.RetryCount, lastErr)
}

// doFetch performs a single JWKS fetch.
func (ks *JWKSKeySource) doFetch(ctx context.Context) (*JWKS, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ks.config.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := ks.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // Body already fully read

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status code %d", ErrFetchFailed, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var jwks JWKS
	if err := json.Unmarshal(body, &jwks); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidJWKS, err)
	}

	// Update cache
	ks.updateCache(&jwks)

	return &jwks, nil
}

// updateCache updates the key cache with the fetched JWKS.
func (ks *JWKSKeySource) updateCache(jwks *JWKS) {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	now := time.Now()
	ks.allKeys = jwks
	ks.lastFetch = now

	for _, jwk := range jwks.Keys {
		key, err := jwk.ToRSAPublicKey()
		if err != nil {
			continue // Skip invalid keys
		}
		ks.cache[jwk.Kid] = &cachedKey{
			key:       key,
			fetchedAt: now,
		}
	}
}

// findKey finds a key in the JWKS by kid.
func (ks *JWKSKeySource) findKey(jwks *JWKS, kid string) (*rsa.PublicKey, error) {
	// If no kid specified and only one key exists, use that
	if kid == "" {
		if len(jwks.Keys) == 1 {
			return jwks.Keys[0].ToRSAPublicKey()
		}
		return nil, fmt.Errorf("%w: no kid specified and multiple keys present", ErrKeyNotFound)
	}

	// Find by kid
	for _, jwk := range jwks.Keys {
		if jwk.Kid == kid {
			return jwk.ToRSAPublicKey()
		}
	}

	return nil, fmt.Errorf("%w: kid=%s", ErrKeyNotFound, kid)
}

// CachedKeyCount returns the number of cached keys (for testing/monitoring).
func (ks *JWKSKeySource) CachedKeyCount() int {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return len(ks.cache)
}

// LastFetchTime returns the time of the last successful JWKS fetch.
func (ks *JWKSKeySource) LastFetchTime() time.Time {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.lastFetch
}

// ForceRefresh forces an immediate refresh of the JWKS cache.
func (ks *JWKSKeySource) ForceRefresh(ctx context.Context) error {
	_, err := ks.fetchJWKS(ctx)
	return err
}
