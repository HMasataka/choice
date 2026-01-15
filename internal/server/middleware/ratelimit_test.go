package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSlidingWindow(t *testing.T) {
	t.Run("allows requests under limit", func(t *testing.T) {
		sw := newSlidingWindow(3, time.Minute)

		for i := 0; i < 3; i++ {
			if !sw.Allow() {
				t.Errorf("expected request %d to be allowed", i+1)
			}
		}
	})

	t.Run("blocks requests over limit", func(t *testing.T) {
		sw := newSlidingWindow(2, time.Minute)

		sw.Allow()
		sw.Allow()

		if sw.Allow() {
			t.Error("expected request to be blocked")
		}
	})

	t.Run("allows requests after window expires", func(t *testing.T) {
		sw := newSlidingWindow(1, 10*time.Millisecond)

		sw.Allow()

		// Wait for window to expire
		time.Sleep(20 * time.Millisecond)

		if !sw.Allow() {
			t.Error("expected request to be allowed after window expired")
		}
	})

	t.Run("count returns correct value", func(t *testing.T) {
		sw := newSlidingWindow(10, time.Minute)

		sw.Allow()
		sw.Allow()
		sw.Allow()

		if count := sw.Count(); count != 3 {
			t.Errorf("expected count 3, got %d", count)
		}
	})
}

func TestRateLimiter(t *testing.T) {
	t.Run("allows requests under limit", func(t *testing.T) {
		cfg := RateLimitConfig{
			RequestsPerMinute: 5,
			CleanupInterval:   time.Hour, // Long interval to avoid cleanup during test
		}
		rl := NewRateLimiter(cfg)
		defer rl.Close()

		for i := 0; i < 5; i++ {
			if !rl.Allow("test-key", 5, time.Minute) {
				t.Errorf("expected request %d to be allowed", i+1)
			}
		}
	})

	t.Run("blocks requests over limit", func(t *testing.T) {
		cfg := RateLimitConfig{
			RequestsPerMinute: 2,
			CleanupInterval:   time.Hour,
		}
		rl := NewRateLimiter(cfg)
		defer rl.Close()

		rl.Allow("test-key", 2, time.Minute)
		rl.Allow("test-key", 2, time.Minute)

		if rl.Allow("test-key", 2, time.Minute) {
			t.Error("expected request to be blocked")
		}
	})

	t.Run("different keys have separate limits", func(t *testing.T) {
		cfg := RateLimitConfig{
			RequestsPerMinute: 1,
			CleanupInterval:   time.Hour,
		}
		rl := NewRateLimiter(cfg)
		defer rl.Close()

		rl.Allow("key1", 1, time.Minute)

		if !rl.Allow("key2", 1, time.Minute) {
			t.Error("expected different key to have separate limit")
		}
	})
}

func TestRateLimitMiddleware(t *testing.T) {
	t.Run("allows requests under limit", func(t *testing.T) {
		cfg := RateLimitConfig{
			RequestsPerMinute: 5,
			CleanupInterval:   time.Hour,
		}
		rl := NewRateLimiter(cfg)
		defer rl.Close()

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		wrapped := rl.RateLimit()(handler)

		for i := 0; i < 5; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = "192.168.1.1:12345"
			w := httptest.NewRecorder()

			wrapped.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
			}
		}
	})

	t.Run("blocks requests over limit", func(t *testing.T) {
		cfg := RateLimitConfig{
			RequestsPerMinute: 2,
			CleanupInterval:   time.Hour,
		}
		rl := NewRateLimiter(cfg)
		defer rl.Close()

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		wrapped := rl.RateLimit()(handler)

		// Use up the limit
		for i := 0; i < 2; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = "192.168.1.1:12345"
			w := httptest.NewRecorder()
			wrapped.ServeHTTP(w, req)
		}

		// This should be blocked
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()

		wrapped.ServeHTTP(w, req)

		if w.Code != http.StatusTooManyRequests {
			t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, w.Code)
		}
	})

	t.Run("uses authorization token as key", func(t *testing.T) {
		cfg := RateLimitConfig{
			RequestsPerMinute: 1,
			CleanupInterval:   time.Hour,
		}
		rl := NewRateLimiter(cfg)
		defer rl.Close()

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		wrapped := rl.RateLimit()(handler)

		// First request with token1
		req1 := httptest.NewRequest(http.MethodGet, "/", nil)
		req1.Header.Set("Authorization", "Bearer token1")
		w1 := httptest.NewRecorder()
		wrapped.ServeHTTP(w1, req1)

		// Second request with token2 (different token, should be allowed)
		req2 := httptest.NewRequest(http.MethodGet, "/", nil)
		req2.Header.Set("Authorization", "Bearer token2")
		w2 := httptest.NewRecorder()
		wrapped.ServeHTTP(w2, req2)

		if w2.Code != http.StatusOK {
			t.Errorf("expected different token to have separate limit, got status %d", w2.Code)
		}
	})
}

func TestRoomCreationLimitMiddleware(t *testing.T) {
	t.Run("limits room creation per user", func(t *testing.T) {
		cfg := RateLimitConfig{
			RoomCreationPerMinute: 2,
			CleanupInterval:       time.Hour,
		}
		rl := NewRateLimiter(cfg)
		defer rl.Close()

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		})

		wrapped := rl.RoomCreationLimit()(handler)

		// Use up the limit
		for i := 0; i < 2; i++ {
			req := httptest.NewRequest(http.MethodPost, "/rooms", nil)
			req.RemoteAddr = "192.168.1.1:12345"
			w := httptest.NewRecorder()
			wrapped.ServeHTTP(w, req)
		}

		// This should be blocked
		req := httptest.NewRequest(http.MethodPost, "/rooms", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()

		wrapped.ServeHTTP(w, req)

		if w.Code != http.StatusTooManyRequests {
			t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, w.Code)
		}
	})
}

func TestConnectionLimiter(t *testing.T) {
	t.Run("allows connections under limit", func(t *testing.T) {
		cfg := RateLimitConfig{
			ConnectionsPerIP:     3,
			GlobalMaxConnections: 100,
		}
		cl := NewConnectionLimiter(cfg)

		for i := 0; i < 3; i++ {
			if !cl.Acquire("192.168.1.1") {
				t.Errorf("expected connection %d to be allowed", i+1)
			}
		}

		if cl.Count("192.168.1.1") != 3 {
			t.Errorf("expected count 3, got %d", cl.Count("192.168.1.1"))
		}
	})

	t.Run("blocks connections over per-IP limit", func(t *testing.T) {
		cfg := RateLimitConfig{
			ConnectionsPerIP:     2,
			GlobalMaxConnections: 100,
		}
		cl := NewConnectionLimiter(cfg)

		cl.Acquire("192.168.1.1")
		cl.Acquire("192.168.1.1")

		if cl.Acquire("192.168.1.1") {
			t.Error("expected connection to be blocked")
		}
	})

	t.Run("blocks connections over global limit", func(t *testing.T) {
		cfg := RateLimitConfig{
			ConnectionsPerIP:     10,
			GlobalMaxConnections: 3,
		}
		cl := NewConnectionLimiter(cfg)

		cl.Acquire("192.168.1.1")
		cl.Acquire("192.168.1.2")
		cl.Acquire("192.168.1.3")

		if cl.Acquire("192.168.1.4") {
			t.Error("expected connection to be blocked by global limit")
		}
	})

	t.Run("release frees connection slot", func(t *testing.T) {
		cfg := RateLimitConfig{
			ConnectionsPerIP:     1,
			GlobalMaxConnections: 100,
		}
		cl := NewConnectionLimiter(cfg)

		cl.Acquire("192.168.1.1")
		cl.Release("192.168.1.1")

		if !cl.Acquire("192.168.1.1") {
			t.Error("expected connection to be allowed after release")
		}
	})

	t.Run("different IPs have separate limits", func(t *testing.T) {
		cfg := RateLimitConfig{
			ConnectionsPerIP:     1,
			GlobalMaxConnections: 100,
		}
		cl := NewConnectionLimiter(cfg)

		cl.Acquire("192.168.1.1")

		if !cl.Acquire("192.168.1.2") {
			t.Error("expected different IP to have separate limit")
		}
	})

	t.Run("TotalCount tracks all connections", func(t *testing.T) {
		cfg := RateLimitConfig{
			ConnectionsPerIP:     10,
			GlobalMaxConnections: 100,
		}
		cl := NewConnectionLimiter(cfg)

		cl.Acquire("192.168.1.1")
		cl.Acquire("192.168.1.2")
		cl.Acquire("192.168.1.1")

		if cl.TotalCount() != 3 {
			t.Errorf("expected total count 3, got %d", cl.TotalCount())
		}

		cl.Release("192.168.1.1")

		if cl.TotalCount() != 2 {
			t.Errorf("expected total count 2, got %d", cl.TotalCount())
		}
	})
}

func TestConnectionLimitMiddleware(t *testing.T) {
	t.Run("allows requests under limit", func(t *testing.T) {
		cfg := RateLimitConfig{
			ConnectionsPerIP:     5,
			GlobalMaxConnections: 100,
		}
		cl := NewConnectionLimiter(cfg)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		wrapped := cl.ConnectionLimit()(handler)

		for i := 0; i < 5; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = "192.168.1.1:12345"
			w := httptest.NewRecorder()

			wrapped.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
			}
		}
	})
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		xri        string
		expected   string
	}{
		{
			name:       "from RemoteAddr with port",
			remoteAddr: "192.168.1.1:12345",
			expected:   "192.168.1.1",
		},
		{
			name:       "from RemoteAddr without port",
			remoteAddr: "192.168.1.1",
			expected:   "192.168.1.1",
		},
		{
			name:       "from X-Forwarded-For single IP",
			remoteAddr: "127.0.0.1:12345",
			xff:        "192.168.1.100",
			expected:   "192.168.1.100",
		},
		{
			name:       "from X-Forwarded-For multiple IPs",
			remoteAddr: "127.0.0.1:12345",
			xff:        "192.168.1.100, 10.0.0.1, 172.16.0.1",
			expected:   "192.168.1.100",
		},
		{
			name:       "from X-Real-IP",
			remoteAddr: "127.0.0.1:12345",
			xri:        "192.168.1.200",
			expected:   "192.168.1.200",
		},
		{
			name:       "X-Forwarded-For takes precedence over X-Real-IP",
			remoteAddr: "127.0.0.1:12345",
			xff:        "192.168.1.100",
			xri:        "192.168.1.200",
			expected:   "192.168.1.100",
		},
		{
			name:       "IPv6 address with port",
			remoteAddr: "[2001:db8::1]:12345",
			expected:   "2001:db8::1",
		},
		{
			name:       "IPv6 address without port",
			remoteAddr: "2001:db8::1",
			expected:   "2001:db8::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xri != "" {
				req.Header.Set("X-Real-IP", tt.xri)
			}

			ip := extractIP(req)
			if ip != tt.expected {
				t.Errorf("expected IP %q, got %q", tt.expected, ip)
			}
		})
	}
}

func TestDefaultRateLimitConfig(t *testing.T) {
	cfg := DefaultRateLimitConfig()

	if cfg.RequestsPerMinute != 100 {
		t.Errorf("expected RequestsPerMinute 100, got %d", cfg.RequestsPerMinute)
	}
	if cfg.RoomCreationPerMinute != 10 {
		t.Errorf("expected RoomCreationPerMinute 10, got %d", cfg.RoomCreationPerMinute)
	}
	if cfg.ConnectionsPerIP != 50 {
		t.Errorf("expected ConnectionsPerIP 50, got %d", cfg.ConnectionsPerIP)
	}
	if cfg.GlobalMaxConnections != 10000 {
		t.Errorf("expected GlobalMaxConnections 10000, got %d", cfg.GlobalMaxConnections)
	}
	if cfg.CleanupInterval != time.Minute {
		t.Errorf("expected CleanupInterval 1m, got %v", cfg.CleanupInterval)
	}
}

func TestRateLimiterWithZeroCleanupInterval(t *testing.T) {
	// Test that zero cleanup interval does not panic
	cfg := RateLimitConfig{
		RequestsPerMinute: 5,
		CleanupInterval:   0, // No cleanup
	}

	// Should not panic
	rl := NewRateLimiter(cfg)

	// Should still work for rate limiting
	if !rl.Allow("test-key", 5, time.Minute) {
		t.Error("expected request to be allowed")
	}

	// Close should not panic either
	rl.Close()
}
