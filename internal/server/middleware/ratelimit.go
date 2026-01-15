package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimiter provides rate limiting functionality.
type RateLimiter struct {
	mu        sync.RWMutex
	windows   map[string]*slidingWindow
	config    RateLimitConfig
	cleanupCh chan struct{}
}

// RateLimitConfig contains rate limit configuration.
type RateLimitConfig struct {
	// RequestsPerMinute is the default rate limit for REST API requests.
	RequestsPerMinute int
	// RoomCreationPerMinute is the rate limit for room creation.
	RoomCreationPerMinute int
	// ConnectionsPerIP is the maximum concurrent connections per IP.
	ConnectionsPerIP int
	// GlobalMaxConnections is the global maximum connections.
	GlobalMaxConnections int
	// CleanupInterval is how often to clean up expired entries.
	CleanupInterval time.Duration
}

// DefaultRateLimitConfig returns the default rate limit configuration.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerMinute:     100,
		RoomCreationPerMinute: 10,
		ConnectionsPerIP:      50,
		GlobalMaxConnections:  10000,
		CleanupInterval:       1 * time.Minute,
	}
}

// slidingWindow implements a sliding window rate limiter.
type slidingWindow struct {
	timestamps []time.Time
	limit      int
	window     time.Duration
}

func newSlidingWindow(limit int, window time.Duration) *slidingWindow {
	return &slidingWindow{
		timestamps: make([]time.Time, 0, limit),
		limit:      limit,
		window:     window,
	}
}

// Allow checks if a request is allowed and records it if so.
func (sw *slidingWindow) Allow() bool {
	now := time.Now()
	cutoff := now.Add(-sw.window)

	// Remove expired timestamps
	validStart := 0
	for i, ts := range sw.timestamps {
		if ts.After(cutoff) {
			validStart = i
			break
		}
		if i == len(sw.timestamps)-1 {
			validStart = len(sw.timestamps)
		}
	}
	sw.timestamps = sw.timestamps[validStart:]

	// Check if under limit
	if len(sw.timestamps) >= sw.limit {
		return false
	}

	// Record this request
	sw.timestamps = append(sw.timestamps, now)
	return true
}

// Count returns the current count within the window.
func (sw *slidingWindow) Count() int {
	now := time.Now()
	cutoff := now.Add(-sw.window)

	count := 0
	for _, ts := range sw.timestamps {
		if ts.After(cutoff) {
			count++
		}
	}
	return count
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		windows:   make(map[string]*slidingWindow),
		config:    cfg,
		cleanupCh: make(chan struct{}),
	}

	// Start cleanup goroutine only if interval is positive
	if cfg.CleanupInterval > 0 {
		go rl.cleanupLoop()
	}

	return rl
}

// Close stops the rate limiter cleanup goroutine.
func (rl *RateLimiter) Close() {
	// Only close if cleanup goroutine was started
	if rl.config.CleanupInterval > 0 {
		close(rl.cleanupCh)
	}
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.cleanupCh:
			return
		}
	}
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for key, window := range rl.windows {
		// Remove windows with no recent activity
		if len(window.timestamps) == 0 {
			delete(rl.windows, key)
			continue
		}

		// Check if all timestamps are expired
		lastTimestamp := window.timestamps[len(window.timestamps)-1]
		if now.Sub(lastTimestamp) > window.window {
			delete(rl.windows, key)
		}
	}
}

// Allow checks if a request is allowed for the given key.
func (rl *RateLimiter) Allow(key string, limit int, window time.Duration) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	sw, ok := rl.windows[key]
	if !ok {
		sw = newSlidingWindow(limit, window)
		rl.windows[key] = sw
	}

	return sw.Allow()
}

// RateLimit creates a general rate limit middleware.
func (rl *RateLimiter) RateLimit() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := rl.getClientKey(r)

			if !rl.Allow(key, rl.config.RequestsPerMinute, time.Minute) {
				http.Error(w, `{"error":"Rate limit exceeded","code":429}`, http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RoomCreationLimit creates a rate limit middleware for room creation.
func (rl *RateLimiter) RoomCreationLimit() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := "room:" + rl.getClientKey(r)

			if !rl.Allow(key, rl.config.RoomCreationPerMinute, time.Minute) {
				http.Error(w, `{"error":"Room creation rate limit exceeded","code":429}`, http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ConnectionLimiter manages connection-based limits.
type ConnectionLimiter struct {
	mu          sync.RWMutex
	connections map[string]int
	total       int
	config      RateLimitConfig
}

// NewConnectionLimiter creates a new connection limiter.
func NewConnectionLimiter(cfg RateLimitConfig) *ConnectionLimiter {
	return &ConnectionLimiter{
		connections: make(map[string]int),
		config:      cfg,
	}
}

// Acquire attempts to acquire a connection slot for the given IP.
// Returns true if successful, false if limits are exceeded.
func (cl *ConnectionLimiter) Acquire(ip string) bool {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	// Check global limit
	if cl.total >= cl.config.GlobalMaxConnections {
		return false
	}

	// Check per-IP limit
	if cl.connections[ip] >= cl.config.ConnectionsPerIP {
		return false
	}

	cl.connections[ip]++
	cl.total++
	return true
}

// Release releases a connection slot for the given IP.
func (cl *ConnectionLimiter) Release(ip string) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.connections[ip] > 0 {
		cl.connections[ip]--
		cl.total--

		if cl.connections[ip] == 0 {
			delete(cl.connections, ip)
		}
	}
}

// Count returns the current connection count for the given IP.
func (cl *ConnectionLimiter) Count(ip string) int {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.connections[ip]
}

// TotalCount returns the total number of connections.
func (cl *ConnectionLimiter) TotalCount() int {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.total
}

// ConnectionLimit creates a middleware that limits connections per IP.
func (cl *ConnectionLimiter) ConnectionLimit() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractIP(r)

			if !cl.Acquire(ip) {
				http.Error(w, `{"error":"Connection limit exceeded","code":429}`, http.StatusTooManyRequests)
				return
			}

			// Note: For HTTP requests, we release immediately after the request completes.
			// For WebSocket connections, the caller should manage Acquire/Release manually.
			defer cl.Release(ip)

			next.ServeHTTP(w, r)
		})
	}
}

// getClientKey extracts a unique key for the client from the request.
// Uses Authorization token if present, otherwise falls back to IP address.
func (rl *RateLimiter) getClientKey(r *http.Request) string {
	// Try to use authorization token
	if auth := r.Header.Get("Authorization"); auth != "" {
		return "token:" + auth
	}

	// Fall back to IP address
	return "ip:" + extractIP(r)
}

// extractIP extracts the client IP from the request.
func extractIP(r *http.Request) string {
	// Check X-Forwarded-For header (for proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	// Use net.SplitHostPort for correct handling of IPv4 and IPv6 addresses
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// If SplitHostPort fails, the address might not have a port
		// Return as-is (handles plain IPv4 and IPv6 addresses)
		return r.RemoteAddr
	}
	return host
}
