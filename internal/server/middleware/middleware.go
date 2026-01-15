package middleware

import (
	"context"
	"net/http"
	"runtime/debug"
	"sync"
	"time"

	"github.com/HMasataka/choice/pkg/logger"
	"github.com/google/uuid"
)

// contextKey is a type for context keys to avoid collisions.
type contextKey string

// RequestIDKey is the context key for request ID.
const RequestIDKey contextKey = "request_id"

// Chain chains multiple middleware functions.
func Chain(h http.Handler, middleware ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}

// RequestID adds a unique request ID to each request.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
		w.Header().Set("X-Request-ID", requestID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID retrieves the request ID from the context.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	size        int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, status: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	rw.status = code
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.size += n
	return n, err
}

// Logging creates a logging middleware.
// Note: Works with RequestID middleware in any order - retrieves request ID from response header.
func Logging(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := newResponseWriter(w)

			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)

			// Get request ID from response header (set by RequestID middleware)
			// This works regardless of middleware ordering
			requestID := wrapped.Header().Get("X-Request-ID")

			log.Info("http request",
				"request_id", requestID,
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.status,
				"duration_ms", duration.Milliseconds(),
				"size", wrapped.size,
				"remote_ip", r.RemoteAddr,
				"user_agent", r.UserAgent(),
			)
		})
	}
}

// Recovery creates a panic recovery middleware.
func Recovery(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					requestID := GetRequestID(r.Context())

					log.Error("panic recovered",
						"request_id", requestID,
						"error", err,
						"stack", string(debug.Stack()),
					)

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"Internal Server Error","code":500}`))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// CORSConfig contains CORS configuration.
type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAge           int
}

// DefaultCORSConfig returns the default CORS configuration.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           86400, // 24 hours
	}
}

// CORS creates a CORS middleware.
func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	// Pre-compute if wildcard is allowed
	hasWildcard := false
	for _, o := range cfg.AllowedOrigins {
		if o == "*" {
			hasWildcard = true
			break
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Skip CORS headers if no Origin header present
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Check if origin is allowed
			allowed := false
			if hasWildcard {
				allowed = true
			} else {
				for _, o := range cfg.AllowedOrigins {
					if o == origin {
						allowed = true
						break
					}
				}
			}

			if allowed {
				// Use "*" for wildcard, actual origin otherwise
				if hasWildcard && !cfg.AllowCredentials {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					// Add Vary header when echoing specific origin
					w.Header().Add("Vary", "Origin")
				}
				if cfg.AllowCredentials {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
			}

			// Handle preflight request
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", joinStrings(cfg.AllowedMethods, ", "))
				w.Header().Set("Access-Control-Allow-Headers", joinStrings(cfg.AllowedHeaders, ", "))
				if len(cfg.ExposedHeaders) > 0 {
					w.Header().Set("Access-Control-Expose-Headers", joinStrings(cfg.ExposedHeaders, ", "))
				}
				if cfg.MaxAge > 0 {
					w.Header().Set("Access-Control-Max-Age", itoa(cfg.MaxAge))
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			if len(cfg.ExposedHeaders) > 0 {
				w.Header().Set("Access-Control-Expose-Headers", joinStrings(cfg.ExposedHeaders, ", "))
			}

			next.ServeHTTP(w, r)
		})
	}
}

// joinStrings joins strings with a separator.
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// itoa converts int to string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + itoa(-n)
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// ContentType sets the Content-Type header for JSON responses.
func ContentType(contentType string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", contentType)
			next.ServeHTTP(w, r)
		})
	}
}

// Timeout creates a timeout middleware that returns 504 Gateway Timeout with JSON response.
// Uses a buffered approach to prevent data races when timeout occurs.
// Note: This middleware buffers responses and does not support http.Flusher, http.Hijacker,
// or http.Pusher interfaces. Do not use with streaming responses or WebSocket upgrades.
func Timeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			done := make(chan struct{})
			tw := &timeoutWriter{
				header: make(http.Header),
				code:   http.StatusOK,
			}

			go func() {
				next.ServeHTTP(tw, r.WithContext(ctx))
				close(done)
			}()

			select {
			case <-done:
				tw.copyTo(w)
			case <-ctx.Done():
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusGatewayTimeout)
				_, _ = w.Write([]byte(`{"error":"Gateway Timeout","code":504}`))
			}
		})
	}
}

// timeoutWriter buffers the response to prevent partial writes on timeout.
// Uses a mutex for concurrent write safety.
type timeoutWriter struct {
	mu     sync.Mutex
	header http.Header
	body   []byte
	code   int
}

func (tw *timeoutWriter) Header() http.Header {
	return tw.header
}

func (tw *timeoutWriter) Write(b []byte) (int, error) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	tw.body = append(tw.body, b...)
	return len(b), nil
}

func (tw *timeoutWriter) WriteHeader(code int) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	tw.code = code
}

// copyTo safely copies the buffered response to the actual ResponseWriter.
func (tw *timeoutWriter) copyTo(w http.ResponseWriter) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	for k, v := range tw.header {
		w.Header()[k] = v
	}
	w.WriteHeader(tw.code)
	_, _ = w.Write(tw.body)
}

// SecureHeaders adds security headers to responses.
func SecureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}
