package signaling

import (
	"encoding/json"
	"sync"
	"time"
)

// WebSocketRateLimitConfig contains WebSocket-specific rate limit configuration.
type WebSocketRateLimitConfig struct {
	// ConnectionsPerSecondPerIP is the maximum WebSocket connection attempts per second per IP.
	ConnectionsPerSecondPerIP int
	// MessagesPerSecondPerConnection is the maximum signaling messages per second per connection.
	MessagesPerSecondPerConnection int
	// BandwidthLimitPerConnection is the maximum bandwidth per connection (bytes/second), 0 = unlimited.
	BandwidthLimitPerConnection int64
	// BandwidthLimitPerRoom is the maximum bandwidth per room (bytes/second), 0 = unlimited.
	BandwidthLimitPerRoom int64
	// MaxMessageSize is the maximum message size in bytes.
	MaxMessageSize int64
	// BurstSize is the number of requests that can burst above the rate.
	BurstSize int
}

// DefaultWebSocketRateLimitConfig returns the default WebSocket rate limit configuration.
func DefaultWebSocketRateLimitConfig() WebSocketRateLimitConfig {
	return WebSocketRateLimitConfig{
		ConnectionsPerSecondPerIP:      10,
		MessagesPerSecondPerConnection: 100,
		BandwidthLimitPerConnection:    0,         // Unlimited by default
		BandwidthLimitPerRoom:          0,         // Unlimited by default
		MaxMessageSize:                 64 * 1024, // 64KB
		BurstSize:                      20,
	}
}

// WebSocketRateLimiter manages rate limits for WebSocket connections.
type WebSocketRateLimiter struct {
	mu     sync.RWMutex
	config WebSocketRateLimitConfig

	// Connection rate limits by IP
	connectionLimits map[string]*tokenBucket

	// Message rate limits by connection ID
	messageLimits map[string]*tokenBucket

	// Bandwidth tracking by connection ID
	connectionBandwidth map[string]*bandwidthTracker

	// Bandwidth tracking by room ID
	roomBandwidth map[string]*bandwidthTracker

	cleanupCh chan struct{}
}

// tokenBucket implements a token bucket rate limiter.
type tokenBucket struct {
	tokens     float64
	capacity   float64
	rate       float64 // tokens per second
	lastRefill time.Time
	mu         sync.Mutex
}

func newTokenBucket(capacity, rate float64) *tokenBucket {
	return &tokenBucket{
		tokens:     capacity,
		capacity:   capacity,
		rate:       rate,
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed and consumes a token if so.
func (tb *tokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()

	// Refill tokens based on elapsed time
	tb.tokens = min(tb.capacity, tb.tokens+elapsed*tb.rate)
	tb.lastRefill = now

	// Check if we have tokens available
	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}

	return false
}

// bandwidthTracker tracks bandwidth usage over a time window.
type bandwidthTracker struct {
	mu          sync.Mutex
	bytesUsed   int64
	windowSize  time.Duration
	windowStart time.Time
	limit       int64 // bytes per second, 0 = unlimited
}

func newBandwidthTracker(limit int64, windowSize time.Duration) *bandwidthTracker {
	return &bandwidthTracker{
		bytesUsed:   0,
		windowSize:  windowSize,
		windowStart: time.Now(),
		limit:       limit,
	}
}

// Allow checks if the bandwidth limit allows the given number of bytes.
func (bt *bandwidthTracker) Allow(bytes int64) bool {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	// If no limit, always allow
	if bt.limit == 0 {
		return true
	}

	now := time.Now()
	elapsed := now.Sub(bt.windowStart)

	// Reset window if needed
	if elapsed >= bt.windowSize {
		bt.bytesUsed = 0
		bt.windowStart = now
	}

	// Check if adding these bytes would exceed the limit
	if bt.bytesUsed+bytes > bt.limit*int64(bt.windowSize.Seconds()) {
		return false
	}

	bt.bytesUsed += bytes
	return true
}

// NewWebSocketRateLimiter creates a new WebSocket rate limiter.
func NewWebSocketRateLimiter(cfg WebSocketRateLimitConfig) *WebSocketRateLimiter {
	rl := &WebSocketRateLimiter{
		config:              cfg,
		connectionLimits:    make(map[string]*tokenBucket),
		messageLimits:       make(map[string]*tokenBucket),
		connectionBandwidth: make(map[string]*bandwidthTracker),
		roomBandwidth:       make(map[string]*bandwidthTracker),
		cleanupCh:           make(chan struct{}),
	}

	// Start cleanup goroutine
	go rl.cleanupLoop()

	return rl
}

// Close stops the rate limiter cleanup goroutine.
func (rl *WebSocketRateLimiter) Close() {
	close(rl.cleanupCh)
}

func (rl *WebSocketRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
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

func (rl *WebSocketRateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Clean up stale connection rate limit entries
	for key, tb := range rl.connectionLimits {
		tb.mu.Lock()
		lastRefill := tb.lastRefill
		tb.mu.Unlock()
		if now.Sub(lastRefill) > 10*time.Minute {
			delete(rl.connectionLimits, key)
		}
	}

	// Clean up stale message rate limit entries
	for key, tb := range rl.messageLimits {
		tb.mu.Lock()
		lastRefill := tb.lastRefill
		tb.mu.Unlock()
		if now.Sub(lastRefill) > 10*time.Minute {
			delete(rl.messageLimits, key)
		}
	}

	// Clean up stale bandwidth tracking entries
	for key, bt := range rl.connectionBandwidth {
		bt.mu.Lock()
		windowStart := bt.windowStart
		bt.mu.Unlock()
		if now.Sub(windowStart) > 10*time.Minute {
			delete(rl.connectionBandwidth, key)
		}
	}

	for key, bt := range rl.roomBandwidth {
		bt.mu.Lock()
		windowStart := bt.windowStart
		bt.mu.Unlock()
		if now.Sub(windowStart) > 10*time.Minute {
			delete(rl.roomBandwidth, key)
		}
	}
}

// AllowConnection checks if a new connection is allowed from the given IP.
func (rl *WebSocketRateLimiter) AllowConnection(ip string) bool {
	if rl.config.ConnectionsPerSecondPerIP <= 0 {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	tb, ok := rl.connectionLimits[ip]
	if !ok {
		// Create new token bucket with burst support
		capacity := float64(rl.config.ConnectionsPerSecondPerIP * 2)
		if rl.config.BurstSize > 0 {
			capacity = float64(rl.config.BurstSize)
		}
		tb = newTokenBucket(capacity, float64(rl.config.ConnectionsPerSecondPerIP))
		rl.connectionLimits[ip] = tb
	}

	return tb.Allow()
}

// AllowMessage checks if a message is allowed for the given connection.
func (rl *WebSocketRateLimiter) AllowMessage(connectionID string, messageSize int64) bool {
	// Check message size
	if messageSize > rl.config.MaxMessageSize {
		return false
	}
	if rl.config.MessagesPerSecondPerConnection <= 0 {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Check message rate limit
	tb, ok := rl.messageLimits[connectionID]
	if !ok {
		capacity := float64(rl.config.MessagesPerSecondPerConnection * 2)
		if rl.config.BurstSize > 0 {
			capacity = float64(rl.config.BurstSize)
		}
		tb = newTokenBucket(capacity, float64(rl.config.MessagesPerSecondPerConnection))
		rl.messageLimits[connectionID] = tb
	}

	return tb.Allow()
}

// AllowBandwidth checks if the bandwidth limit allows the given message for the connection and room.
func (rl *WebSocketRateLimiter) AllowBandwidth(connectionID, roomID string, bytes int64) bool {
	if rl.config.BandwidthLimitPerConnection <= 0 && rl.config.BandwidthLimitPerRoom <= 0 {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Check connection bandwidth limit
	if rl.config.BandwidthLimitPerConnection > 0 {
		bt, ok := rl.connectionBandwidth[connectionID]
		if !ok {
			bt = newBandwidthTracker(rl.config.BandwidthLimitPerConnection, time.Second)
			rl.connectionBandwidth[connectionID] = bt
		}

		if !bt.Allow(bytes) {
			return false
		}
	}

	// Check room bandwidth limit
	if rl.config.BandwidthLimitPerRoom > 0 && roomID != "" {
		bt, ok := rl.roomBandwidth[roomID]
		if !ok {
			bt = newBandwidthTracker(rl.config.BandwidthLimitPerRoom, time.Second)
			rl.roomBandwidth[roomID] = bt
		}

		if !bt.Allow(bytes) {
			return false
		}
	}

	return true
}

// RemoveConnection removes rate limit tracking for a connection.
func (rl *WebSocketRateLimiter) RemoveConnection(connectionID string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	delete(rl.messageLimits, connectionID)
	delete(rl.connectionBandwidth, connectionID)
}

// RemoveRoom removes bandwidth tracking for a room.
func (rl *WebSocketRateLimiter) RemoveRoom(roomID string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	delete(rl.roomBandwidth, roomID)
}

// GetConnectionMessageRate returns the current message rate for a connection.
func (rl *WebSocketRateLimiter) GetConnectionMessageRate(connectionID string) float64 {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	tb, ok := rl.messageLimits[connectionID]
	if !ok {
		return 0
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	return tb.rate
}

// IsValidMessage checks if a message appears to be valid (basic sanity checks).
func (rl *WebSocketRateLimiter) IsValidMessage(message []byte) bool {
	// Check message size
	if int64(len(message)) > rl.config.MaxMessageSize {
		return false
	}

	// Check if message is empty
	if len(message) == 0 {
		return false
	}

	// Strict JSON validation
	return json.Valid(message)
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
