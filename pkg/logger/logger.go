package logger

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"regexp"
	"strings"
)

// Level represents log levels.
type Level = slog.Level

// Log levels.
const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

// Config represents logger configuration.
type Config struct {
	Level      string
	Format     string // "json" or "text"
	Output     string // "stdout", "stderr", or file path
	PIIMasking bool
}

// Logger wraps slog.Logger with PII masking support.
type Logger struct {
	*slog.Logger
	config Config
}

// DefaultConfig returns the default logger configuration.
func DefaultConfig() Config {
	return Config{
		Level:      "info",
		Format:     "json",
		Output:     "stdout",
		PIIMasking: true,
	}
}

// New creates a new Logger instance.
func New(cfg Config) (*Logger, error) {
	level := parseLevel(cfg.Level)

	var output io.Writer
	switch cfg.Output {
	case "stdout", "":
		output = os.Stdout
	case "stderr":
		output = os.Stderr
	default:
		f, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, err
		}
		output = f
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: level,
	}

	if cfg.PIIMasking {
		opts.ReplaceAttr = makePIIMasker()
	}

	if cfg.Format == "text" {
		handler = slog.NewTextHandler(output, opts)
	} else {
		handler = slog.NewJSONHandler(output, opts)
	}

	return &Logger{
		Logger: slog.New(handler),
		config: cfg,
	}, nil
}

// parseLevel converts string level to slog.Level.
func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// PII masking patterns.
var (
	ipv4Pattern = regexp.MustCompile(`(\d{1,3}\.\d{1,3}\.\d{1,3}\.)\d{1,3}`)
	jwtPattern  = regexp.MustCompile(`eyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*`)
)

// PIIMaskingKeys defines keys whose values should be masked.
var PIIMaskingKeys = map[string]bool{
	"ip":            true,
	"ip_address":    true,
	"remote_ip":     true,
	"client_ip":     true,
	"token":         true,
	"jwt":           true,
	"authorization": true,
	"bearer":        true,
	"sdp":           true,
	"candidate":     true,
}

// makePIIMasker creates an attribute replacer for PII masking.
func makePIIMasker() func(groups []string, a slog.Attr) slog.Attr {
	return func(groups []string, a slog.Attr) slog.Attr {
		key := strings.ToLower(a.Key)

		// Check if this key should be masked
		if PIIMaskingKeys[key] {
			return maskAttr(a)
		}

		// Check for IP patterns and JWT patterns in string values
		if a.Value.Kind() == slog.KindString {
			str := a.Value.String()
			masked := str

			// Mask IPv4 addresses
			if ipv4Pattern.MatchString(masked) {
				masked = maskIPv4(masked)
			}

			// Mask IPv6 addresses (using net.ParseIP for accuracy)
			masked = maskIPv6InString(masked)

			// Mask JWT tokens in arbitrary strings
			if jwtPattern.MatchString(masked) {
				masked = maskJWT(masked)
			}

			if masked != str {
				return slog.String(a.Key, masked)
			}
		}

		return a
	}
}

// maskAttr masks sensitive attribute values.
func maskAttr(a slog.Attr) slog.Attr {
	if a.Value.Kind() != slog.KindString {
		return a
	}

	str := a.Value.String()
	key := strings.ToLower(a.Key)

	switch {
	case key == "token" || key == "jwt" || key == "authorization" || key == "bearer":
		// Show only first 8 characters for JWT tokens
		if len(str) > 8 {
			return slog.String(a.Key, str[:8]+"...")
		}
		return slog.String(a.Key, "[REDACTED]")

	case key == "sdp":
		// Show only SDP type and line count
		lines := strings.Count(str, "\n") + 1
		sdpType := extractSDPType(str)
		return slog.String(a.Key, sdpType+" ("+itoa(lines)+" lines)")

	case key == "candidate":
		// Show only candidate type
		candType := extractCandidateType(str)
		return slog.String(a.Key, candType)

	case strings.Contains(key, "ip"):
		masked := maskIPv4(str)
		masked = maskIPv6(masked)
		return slog.String(a.Key, masked)
	}

	return a
}

// maskIPv4 masks the last octet of an IPv4 address.
func maskIPv4(ip string) string {
	return ipv4Pattern.ReplaceAllString(ip, "${1}xxx")
}

// maskIPv6InString finds and masks IPv6 addresses in a string using net.ParseIP.
func maskIPv6InString(s string) string {
	words := strings.Fields(s)
	changed := false
	for i, word := range words {
		// Try to parse as IP address
		ip := net.ParseIP(word)
		if ip != nil && ip.To4() == nil {
			// This is an IPv6 address (To4 returns nil for IPv6)
			words[i] = "[IPv6-MASKED]"
			changed = true
		}
	}
	if changed {
		return strings.Join(words, " ")
	}
	return s
}

// maskIPv6 masks a single IPv6 address value.
func maskIPv6(s string) string {
	ip := net.ParseIP(s)
	if ip != nil && ip.To4() == nil {
		return "[IPv6-MASKED]"
	}
	return s
}

// maskJWT masks JWT tokens in strings (shows first 8 chars).
func maskJWT(s string) string {
	return jwtPattern.ReplaceAllStringFunc(s, func(token string) string {
		if len(token) > 8 {
			return token[:8] + "..."
		}
		return "[JWT-MASKED]"
	})
}

// extractSDPType extracts the type (offer/answer) from SDP content.
func extractSDPType(sdp string) string {
	if strings.Contains(sdp, "a=setup:actpass") {
		return "offer"
	}
	if strings.Contains(sdp, "a=setup:active") || strings.Contains(sdp, "a=setup:passive") {
		return "answer"
	}
	return "sdp"
}

// extractCandidateType extracts the type from an ICE candidate string.
func extractCandidateType(candidate string) string {
	parts := strings.Fields(candidate)
	for i, part := range parts {
		if part == "typ" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return "candidate"
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

// WithContext returns a Logger with context values.
func (l *Logger) WithContext(ctx context.Context) *Logger {
	// Extract trace ID from context if available
	if traceID := ctx.Value(TraceIDKey); traceID != nil {
		return &Logger{
			Logger: l.Logger.With("trace_id", traceID),
			config: l.config,
		}
	}
	return l
}

// With returns a Logger with additional attributes.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{
		Logger: l.Logger.With(args...),
		config: l.config,
	}
}

// WithRoom returns a Logger with room context.
func (l *Logger) WithRoom(roomID string) *Logger {
	return l.With("room_id", roomID)
}

// WithParticipant returns a Logger with participant context.
func (l *Logger) WithParticipant(participantID string) *Logger {
	return l.With("participant_id", participantID)
}

// Context key types for type-safe context values.
type contextKey string

// TraceIDKey is the context key for trace IDs.
const TraceIDKey contextKey = "trace_id"

// Global logger instance.
var defaultLogger *Logger

// SetDefault sets the default global logger.
func SetDefault(l *Logger) {
	defaultLogger = l
	slog.SetDefault(l.Logger)
}

// Default returns the default global logger.
func Default() *Logger {
	if defaultLogger == nil {
		l, _ := New(DefaultConfig()) //nolint:errcheck // Default config is always valid
		defaultLogger = l
	}
	return defaultLogger
}

// Debug logs at debug level using the default logger.
func Debug(msg string, args ...any) {
	Default().Debug(msg, args...)
}

// Info logs at info level using the default logger.
func Info(msg string, args ...any) {
	Default().Info(msg, args...)
}

// Warn logs at warn level using the default logger.
func Warn(msg string, args ...any) {
	Default().Warn(msg, args...)
}

// Error logs at error level using the default logger.
func Error(msg string, args ...any) {
	Default().Error(msg, args...)
}
