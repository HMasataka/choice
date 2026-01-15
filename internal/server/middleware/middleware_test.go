package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HMasataka/choice/pkg/logger"
)

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.New(logger.DefaultConfig())
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return log
}

func TestRequestID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := GetRequestID(r.Context())
		if requestID == "" {
			t.Error("expected request ID in context")
		}
		w.WriteHeader(http.StatusOK)
	})

	wrapped := RequestID(handler)

	t.Run("generates new request ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()

		wrapped.ServeHTTP(w, req)

		requestID := w.Header().Get("X-Request-ID")
		if requestID == "" {
			t.Error("expected X-Request-ID header")
		}
		// UUID format: 8-4-4-4-12
		if len(requestID) != 36 {
			t.Errorf("expected UUID format, got %q", requestID)
		}
	})

	t.Run("uses existing request ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Request-ID", "existing-id")
		w := httptest.NewRecorder()

		wrapped.ServeHTTP(w, req)

		requestID := w.Header().Get("X-Request-ID")
		if requestID != "existing-id" {
			t.Errorf("expected 'existing-id', got %q", requestID)
		}
	})
}

func TestGetRequestID(t *testing.T) {
	t.Run("returns request ID from context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), RequestIDKey, "test-id")
		id := GetRequestID(ctx)
		if id != "test-id" {
			t.Errorf("expected 'test-id', got %q", id)
		}
	})

	t.Run("returns empty string for missing ID", func(t *testing.T) {
		id := GetRequestID(context.Background())
		if id != "" {
			t.Errorf("expected empty string, got %q", id)
		}
	})
}

func TestLogging(t *testing.T) {
	log := newTestLogger(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	wrapped := Chain(handler, Logging(log), RequestID)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRecovery(t *testing.T) {
	log := newTestLogger(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	wrapped := Chain(handler, Recovery(log), RequestID)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Internal Server Error") {
		t.Errorf("expected error message in body, got %q", body)
	}
}

func TestCORS(t *testing.T) {
	cfg := DefaultCORSConfig()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := CORS(cfg)(handler)

	t.Run("preflight request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/", nil)
		req.Header.Set("Origin", "http://example.com")
		w := httptest.NewRecorder()

		wrapped.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("expected status %d, got %d", http.StatusNoContent, w.Code)
		}

		allowOrigin := w.Header().Get("Access-Control-Allow-Origin")
		if allowOrigin != "*" {
			t.Errorf("expected origin '*' for wildcard config, got %q", allowOrigin)
		}

		allowMethods := w.Header().Get("Access-Control-Allow-Methods")
		if allowMethods == "" {
			t.Error("expected Access-Control-Allow-Methods header")
		}
	})

	t.Run("regular request with origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "http://example.com")
		w := httptest.NewRecorder()

		wrapped.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		allowOrigin := w.Header().Get("Access-Control-Allow-Origin")
		if allowOrigin != "*" {
			t.Errorf("expected origin '*' for wildcard config, got %q", allowOrigin)
		}
	})

	t.Run("request without origin skips CORS headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()

		wrapped.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		allowOrigin := w.Header().Get("Access-Control-Allow-Origin")
		if allowOrigin != "" {
			t.Errorf("expected no Access-Control-Allow-Origin header, got %q", allowOrigin)
		}
	})
}

func TestCORSWithSpecificOrigins(t *testing.T) {
	cfg := CORSConfig{
		AllowedOrigins:   []string{"http://allowed.com"},
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: false,
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := CORS(cfg)(handler)

	t.Run("allowed origin echoed with Vary header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "http://allowed.com")
		w := httptest.NewRecorder()

		wrapped.ServeHTTP(w, req)

		allowOrigin := w.Header().Get("Access-Control-Allow-Origin")
		if allowOrigin != "http://allowed.com" {
			t.Errorf("expected origin 'http://allowed.com', got %q", allowOrigin)
		}

		vary := w.Header().Get("Vary")
		if vary != "Origin" {
			t.Errorf("expected Vary header 'Origin', got %q", vary)
		}
	})

	t.Run("disallowed origin gets no CORS headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "http://notallowed.com")
		w := httptest.NewRecorder()

		wrapped.ServeHTTP(w, req)

		allowOrigin := w.Header().Get("Access-Control-Allow-Origin")
		if allowOrigin != "" {
			t.Errorf("expected no Access-Control-Allow-Origin header, got %q", allowOrigin)
		}
	})
}

func TestCORSWithCredentials(t *testing.T) {
	cfg := DefaultCORSConfig()
	cfg.AllowCredentials = true

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := CORS(cfg)(handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	allowCreds := w.Header().Get("Access-Control-Allow-Credentials")
	if allowCreds != "true" {
		t.Errorf("expected Allow-Credentials 'true', got %q", allowCreds)
	}
}

func TestChain(t *testing.T) {
	var order []string

	m1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m1-before")
			next.ServeHTTP(w, r)
			order = append(order, "m1-after")
		})
	}

	m2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m2-before")
			next.ServeHTTP(w, r)
			order = append(order, "m2-after")
		})
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
	})

	wrapped := Chain(handler, m1, m2)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	expected := []string{"m1-before", "m2-before", "handler", "m2-after", "m1-after"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d calls, got %d", len(expected), len(order))
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("expected order[%d] = %q, got %q", i, v, order[i])
		}
	}
}

func TestSecureHeaders(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := SecureHeaders(handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	headers := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"X-XSS-Protection":       "1; mode=block",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}

	for header, expected := range headers {
		if v := w.Header().Get(header); v != expected {
			t.Errorf("expected %s = %q, got %q", header, expected, v)
		}
	}
}

func TestContentType(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := ContentType("application/json")(handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}
}

func TestTimeout(t *testing.T) {
	t.Run("request completes before timeout", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})

		wrapped := Timeout(1 * time.Second)(handler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()

		wrapped.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("request times out", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		})

		wrapped := Timeout(10 * time.Millisecond)(handler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()

		wrapped.ServeHTTP(w, req)

		if w.Code != http.StatusGatewayTimeout {
			t.Errorf("expected status %d, got %d", http.StatusGatewayTimeout, w.Code)
		}

		body := w.Body.String()
		if !strings.Contains(body, "Gateway Timeout") {
			t.Errorf("expected timeout message in body, got %q", body)
		}

		contentType := w.Header().Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got %q", contentType)
		}
	})
}

func TestResponseWriter(t *testing.T) {
	w := httptest.NewRecorder()
	rw := newResponseWriter(w)

	// Test default status
	if rw.status != http.StatusOK {
		t.Errorf("expected default status %d, got %d", http.StatusOK, rw.status)
	}

	// Test Write triggers WriteHeader
	_, _ = rw.Write([]byte("test"))
	if !rw.wroteHeader {
		t.Error("expected wroteHeader to be true after Write")
	}

	// Test size tracking
	if rw.size != 4 {
		t.Errorf("expected size 4, got %d", rw.size)
	}

	// Test multiple WriteHeader calls
	rw.WriteHeader(http.StatusNotFound) // Should be ignored
	if rw.status != http.StatusOK {
		t.Errorf("expected status to remain %d, got %d", http.StatusOK, rw.status)
	}
}

func TestJoinStrings(t *testing.T) {
	tests := []struct {
		input    []string
		sep      string
		expected string
	}{
		{[]string{}, ", ", ""},
		{[]string{"a"}, ", ", "a"},
		{[]string{"a", "b"}, ", ", "a, b"},
		{[]string{"a", "b", "c"}, "-", "a-b-c"},
	}

	for _, tt := range tests {
		got := joinStrings(tt.input, tt.sep)
		if got != tt.expected {
			t.Errorf("joinStrings(%v, %q) = %q, want %q", tt.input, tt.sep, got, tt.expected)
		}
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{123, "123"},
		{-1, "-1"},
	}

	for _, tt := range tests {
		got := itoa(tt.input)
		if got != tt.expected {
			t.Errorf("itoa(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
