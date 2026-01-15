package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", LevelDebug},
		{"DEBUG", LevelDebug},
		{"info", LevelInfo},
		{"INFO", LevelInfo},
		{"warn", LevelWarn},
		{"warning", LevelWarn},
		{"error", LevelError},
		{"ERROR", LevelError},
		{"invalid", LevelInfo}, // defaults to info
		{"", LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseLevel(tt.input)
			if got != tt.expected {
				t.Errorf("parseLevel(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestMaskIPv4(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"192.168.1.100", "192.168.1.xxx"},
		{"10.0.0.1", "10.0.0.xxx"},
		{"no ip here", "no ip here"},
		{"IP: 192.168.1.50", "IP: 192.168.1.xxx"},
		{"multiple 10.0.0.1 and 192.168.1.1", "multiple 10.0.0.xxx and 192.168.1.xxx"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := maskIPv4(tt.input)
			if got != tt.expected {
				t.Errorf("maskIPv4(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestMaskIPv6(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"full IPv6", "2001:0db8:85a3:0000:0000:8a2e:0370:7334", "[IPv6-MASKED]"},
		{"compressed IPv6", "2001:db8::1", "[IPv6-MASKED]"},
		{"loopback IPv6", "::1", "[IPv6-MASKED]"},
		{"no IPv6", "just a normal string", "just a normal string"},
		{"false positive check a:b", "a:b", "a:b"},
		{"false positive check key:value", "key:value", "key:value"},
		{"false positive check time format", "12:30:45", "12:30:45"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskIPv6(tt.input)
			if got != tt.expected {
				t.Errorf("maskIPv6() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestMaskIPv6InString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"full IPv6 in sentence", "connected from 2001:0db8:85a3:0000:0000:8a2e:0370:7334 via TCP", "connected from [IPv6-MASKED] via TCP"},
		{"compressed IPv6 in sentence", "host 2001:db8::1 unreachable", "host [IPv6-MASKED] unreachable"},
		{"loopback in sentence", "listening on ::1 port 8080", "listening on [IPv6-MASKED] port 8080"},
		{"no IPv6", "just a normal string", "just a normal string"},
		{"false positive check", "key:value pair", "key:value pair"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskIPv6InString(tt.input)
			if got != tt.expected {
				t.Errorf("maskIPv6InString() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestMaskJWT(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"full JWT", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U", "eyJhbGci..."},
		{"no JWT", "just a normal string", "just a normal string"},
		{"embedded JWT", "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U", "Bearer eyJhbGci..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskJWT(tt.input)
			if got != tt.expected {
				t.Errorf("maskJWT() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestExtractSDPType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"offer", "v=0\r\na=setup:actpass\r\n", "offer"},
		{"answer active", "v=0\r\na=setup:active\r\n", "answer"},
		{"answer passive", "v=0\r\na=setup:passive\r\n", "answer"},
		{"unknown", "v=0\r\n", "sdp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSDPType(tt.input)
			if got != tt.expected {
				t.Errorf("extractSDPType() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestExtractCandidateType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"host", "candidate:1 1 UDP 2130706431 192.168.1.1 12345 typ host", "host"},
		{"srflx", "candidate:2 1 UDP 1694498815 1.2.3.4 54321 typ srflx raddr 192.168.1.1 rport 12345", "srflx"},
		{"relay", "candidate:3 1 UDP 16777215 5.6.7.8 11111 typ relay raddr 1.2.3.4 rport 54321", "relay"},
		{"no type", "invalid candidate", "candidate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCandidateType(tt.input)
			if got != tt.expected {
				t.Errorf("extractCandidateType() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{123, "123"},
		{-1, "-1"},
		{-123, "-123"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := itoa(tt.input)
			if got != tt.expected {
				t.Errorf("itoa(%d) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNewLogger(t *testing.T) {
	cfg := Config{
		Level:      "debug",
		Format:     "json",
		Output:     "stdout",
		PIIMasking: true,
	}

	l, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if l == nil {
		t.Fatal("New() returned nil logger")
	}
}

func TestNewLoggerTextFormat(t *testing.T) {
	cfg := Config{
		Level:      "info",
		Format:     "text",
		Output:     "stderr",
		PIIMasking: false,
	}

	l, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if l == nil {
		t.Fatal("New() returned nil logger")
	}
}

func TestLoggerWith(t *testing.T) {
	cfg := DefaultConfig()
	l, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	l2 := l.With("key", "value")
	if l2 == nil {
		t.Fatal("With() returned nil")
	}

	l3 := l.WithRoom("room-123")
	if l3 == nil {
		t.Fatal("WithRoom() returned nil")
	}

	l4 := l.WithParticipant("user-456")
	if l4 == nil {
		t.Fatal("WithParticipant() returned nil")
	}
}

func TestLoggerWithContext(t *testing.T) {
	cfg := DefaultConfig()
	l, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.WithValue(context.Background(), TraceIDKey, "trace-abc-123")
	l2 := l.WithContext(ctx)
	if l2 == nil {
		t.Fatal("WithContext() returned nil")
	}

	// Test without trace ID
	l3 := l.WithContext(context.Background())
	if l3 == nil {
		t.Fatal("WithContext() without trace ID returned nil")
	}
}

func TestDefaultLogger(t *testing.T) {
	l := Default()
	if l == nil {
		t.Fatal("Default() returned nil")
	}

	// Should return the same instance
	l2 := Default()
	if l != l2 {
		t.Error("Default() should return the same instance")
	}
}

func TestPIIMasking(t *testing.T) {
	var buf bytes.Buffer

	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		ReplaceAttr: makePIIMasker(),
	})
	l := slog.New(handler)

	// Test IP masking
	l.Info("test", "remote_ip", "192.168.1.100")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	if ip, ok := logEntry["remote_ip"].(string); ok {
		if !strings.Contains(ip, "xxx") {
			t.Errorf("IP should be masked, got: %s", ip)
		}
	}

	// Test token masking
	buf.Reset()
	l.Info("test", "token", "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1c2VyLTEyMyJ9.signature")

	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	if token, ok := logEntry["token"].(string); ok {
		if len(token) > 20 {
			t.Errorf("token should be truncated, got: %s", token)
		}
		if !strings.HasSuffix(token, "...") {
			t.Errorf("token should end with '...', got: %s", token)
		}
	}

	// Test SDP masking
	buf.Reset()
	sdp := "v=0\r\no=- 123 456 IN IP4 127.0.0.1\r\na=setup:actpass\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\n"
	l.Info("test", "sdp", sdp)

	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	if maskedSDP, ok := logEntry["sdp"].(string); ok {
		if strings.Contains(maskedSDP, "127.0.0.1") {
			t.Errorf("SDP content should be masked, got: %s", maskedSDP)
		}
		if !strings.Contains(maskedSDP, "offer") && !strings.Contains(maskedSDP, "lines") {
			t.Errorf("SDP should show type and line count, got: %s", maskedSDP)
		}
	}

	// Test candidate masking
	buf.Reset()
	l.Info("test", "candidate", "candidate:1 1 UDP 2130706431 192.168.1.1 12345 typ host")

	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	if cand, ok := logEntry["candidate"].(string); ok {
		if cand != "host" {
			t.Errorf("candidate should show only type, got: %s", cand)
		}
	}
}

func TestGlobalLogFunctions(t *testing.T) {
	// These should not panic
	cfg := DefaultConfig()
	l, _ := New(cfg)
	SetDefault(l)

	// Test global functions
	Debug("debug message")
	Info("info message")
	Warn("warn message")
	Error("error message")
}
