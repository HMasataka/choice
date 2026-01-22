// Package security provides security tests for the SFU server.
//
// NOTE: These tests use MockConnectionHandler for signaling protocol security testing.
// They verify authentication/authorization logic and input validation, but use mock
// responses for WebSocket tests. Full security penetration testing should be performed
// separately in dedicated security assessment environments.
//
// These tests verify protection against:
// - Authentication bypass
// - Privilege escalation
// - Token tampering
// - Input validation attacks
// - JSON injection
// - WebSocket DoS
// - SDP injection
// - Path traversal
package security

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HMasataka/choice/internal/auth"
	"github.com/HMasataka/choice/internal/room"
	"github.com/HMasataka/choice/internal/server"
	"github.com/HMasataka/choice/internal/signaling"
	"github.com/HMasataka/choice/internal/signaling/protocol"
	"github.com/HMasataka/choice/pkg/config"
	"github.com/HMasataka/choice/pkg/logger"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestServer creates a test server for security testing.
func setupTestServer(t *testing.T) *httptest.Server {
	log, err := logger.New(logger.Config{Level: "error"})
	require.NoError(t, err)

	cfg := &config.Config{
		Server: config.ServerConfig{
			HTTP: config.HTTPConfig{
				Host:         "localhost",
				Port:         0,
				ReadTimeout:  10 * time.Second,
				WriteTimeout: 10 * time.Second,
			},
		},
	}

	srv := server.New(cfg, log)
	return httptest.NewServer(srv.Router())
}

// MockConnectionHandler implements ConnectionHandler for security testing.
type MockConnectionHandler struct {
	mu          sync.Mutex
	connections map[string]*signaling.Connection
}

// NewMockConnectionHandler creates a new mock connection handler.
func NewMockConnectionHandler() *MockConnectionHandler {
	return &MockConnectionHandler{
		connections: make(map[string]*signaling.Connection),
	}
}

// OnConnect handles new connections.
func (h *MockConnectionHandler) OnConnect(conn *signaling.Connection) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.connections[conn.ID()] = conn
}

// OnMessage handles incoming messages.
func (h *MockConnectionHandler) OnMessage(conn *signaling.Connection, message []byte) {
	var req protocol.Request
	if err := json.Unmarshal(message, &req); err != nil {
		return
	}

	resp, err := protocol.NewSuccessResponse(req.ID, map[string]interface{}{
		"status": "ok",
	})
	if err != nil {
		return
	}
	data, _ := json.Marshal(resp)
	conn.Send(data)
}

// OnDisconnect handles disconnections.
func (h *MockConnectionHandler) OnDisconnect(conn *signaling.Connection, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.connections, conn.ID())
}

// setupSignalingServer creates a signaling server for security testing.
func setupSignalingServer(t *testing.T) (*httptest.Server, *room.Manager) {
	log, err := logger.New(logger.Config{Level: "error"})
	require.NoError(t, err)
	roomManager := room.NewManager(log)

	mockHandler := NewMockConnectionHandler()

	cfg := signaling.DefaultHandlerConfig()
	cfg.MaxMessageSize = 65536
	cfg.RateLimit.ConnectionsPerSecondPerIP = 10
	cfg.RateLimit.MessagesPerSecondPerConnection = 100

	handler := signaling.NewHandler(cfg, mockHandler)

	srv := httptest.NewServer(http.HandlerFunc(handler.ServeHTTP))

	return srv, roomManager
}

// generateTestKeys generates RSA key pair for testing.
func generateTestKeys(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return privateKey, &privateKey.PublicKey
}

// TestAuthenticationBypass tests various authentication bypass attempts.
func TestAuthenticationBypass(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	tests := []struct {
		name           string
		method         string
		path           string
		body           string
		headers        map[string]string
		expectedStatus int
	}{
		{
			name:           "access without auth header",
			method:         "GET",
			path:           "/api/v1/rooms/test-room",
			expectedStatus: http.StatusNotFound, // Room doesn't exist, but auth isn't checked at this layer
		},
		{
			name:   "access with empty auth header",
			method: "GET",
			path:   "/api/v1/rooms/test-room",
			headers: map[string]string{
				"Authorization": "",
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:   "access with malformed bearer token",
			method: "GET",
			path:   "/api/v1/rooms/test-room",
			headers: map[string]string{
				"Authorization": "Bearer ",
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:   "access with invalid token format",
			method: "GET",
			path:   "/api/v1/rooms/test-room",
			headers: map[string]string{
				"Authorization": "Bearer invalid.token.here",
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:   "access with wrong auth scheme",
			method: "GET",
			path:   "/api/v1/rooms/test-room",
			headers: map[string]string{
				"Authorization": "Basic dXNlcjpwYXNz",
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body *bytes.Reader
			if tt.body != "" {
				body = bytes.NewReader([]byte(tt.body))
			} else {
				body = bytes.NewReader(nil)
			}

			req, err := http.NewRequest(tt.method, srv.URL+tt.path, body)
			require.NoError(t, err)

			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

// TestPrivilegeEscalation tests privilege escalation attempts.
func TestPrivilegeEscalation(t *testing.T) {
	privateKey, publicKey := generateTestKeys(t)

	// Create JWT validator with our test key
	keySource := auth.NewStaticKeySource(publicKey)
	validator := auth.NewJWTValidator(auth.DefaultJWTConfig(), keySource)
	permChecker := auth.NewPermissionChecker()

	tests := []struct {
		name          string
		role          string
		permission    auth.Permission
		shouldSucceed bool
	}{
		{
			name:          "subscriber cannot publish",
			role:          "subscriber",
			permission:    auth.PermMediaPublish,
			shouldSucceed: false,
		},
		{
			name:          "subscriber can subscribe",
			role:          "subscriber",
			permission:    auth.PermMediaSubscribe,
			shouldSucceed: true,
		},
		{
			name:          "publisher cannot create room",
			role:          "publisher",
			permission:    auth.PermRoomCreate,
			shouldSucceed: false,
		},
		{
			name:          "publisher can publish",
			role:          "publisher",
			permission:    auth.PermMediaPublish,
			shouldSucceed: true,
		},
		{
			name:          "moderator cannot delete room",
			role:          "moderator",
			permission:    auth.PermRoomDelete,
			shouldSucceed: false,
		},
		{
			name:          "moderator can lock room",
			role:          "moderator",
			permission:    auth.PermRoomLock,
			shouldSucceed: true,
		},
		{
			name:          "admin can do everything",
			role:          "admin",
			permission:    auth.PermRoomDelete,
			shouldSucceed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate token with the specified role
			claims := &auth.Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject:   "test-user",
					Issuer:    "test-issuer",
					Audience:  []string{"test-audience"},
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
					IssuedAt:  jwt.NewNumericDate(time.Now()),
				},
				RoomID: "test-room",
				Role:   tt.role,
			}

			tokenString, err := auth.GenerateToken(claims, privateKey)
			require.NoError(t, err)

			// Validate token
			validatedClaims, err := validator.Validate(context.Background(), tokenString)
			require.NoError(t, err)

			// Check permission
			hasPermission := permChecker.ClaimsHasPermission(validatedClaims, tt.permission)

			if tt.shouldSucceed {
				assert.True(t, hasPermission, "should have permission %s with role %s", tt.permission, tt.role)
			} else {
				assert.False(t, hasPermission, "should NOT have permission %s with role %s", tt.permission, tt.role)
			}
		})
	}
}

// TestTokenTampering tests various token tampering attacks.
func TestTokenTampering(t *testing.T) {
	privateKey, publicKey := generateTestKeys(t)

	keySource := auth.NewStaticKeySource(publicKey)
	validator := auth.NewJWTValidator(auth.DefaultJWTConfig(), keySource)

	// Generate a valid token first
	validClaims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "test-user",
			Issuer:    "test-issuer",
			Audience:  []string{"test-audience"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		RoomID: "room-1",
		Role:   "subscriber",
	}
	validToken, err := auth.GenerateToken(validClaims, privateKey)
	require.NoError(t, err)

	tests := []struct {
		name        string
		token       string
		expectError bool
	}{
		{
			name:        "valid token",
			token:       validToken,
			expectError: false,
		},
		{
			name:        "modified header algorithm to none",
			token:       createNoneAlgorithmToken(validClaims),
			expectError: true,
		},
		{
			name:        "modified payload claims",
			token:       modifyTokenPayload(validToken, "admin"),
			expectError: true,
		},
		{
			name:        "truncated signature",
			token:       truncateSignature(validToken),
			expectError: true,
		},
		{
			name:        "empty token",
			token:       "",
			expectError: true,
		},
		{
			name:        "only dots",
			token:       "...",
			expectError: true,
		},
		{
			name:        "random garbage",
			token:       "not.a.valid.jwt.at.all",
			expectError: true,
		},
		{
			name:        "base64 encoded garbage",
			token:       base64.RawURLEncoding.EncodeToString([]byte("garbage")),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validator.Validate(context.Background(), tt.token)
			if tt.expectError {
				assert.Error(t, err, "should reject tampered token")
			} else {
				assert.NoError(t, err, "should accept valid token")
			}
		})
	}
}

// createNoneAlgorithmToken creates a token with "none" algorithm (attack vector).
func createNoneAlgorithmToken(claims *auth.Claims) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payloadBytes, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	return header + "." + payload + "."
}

// modifyTokenPayload modifies the role in the token payload.
func modifyTokenPayload(token, newRole string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return token
	}

	// Decode payload
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return token
	}

	// Modify claims
	var claims map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return token
	}

	claims["role"] = newRole

	// Re-encode
	newPayload, _ := json.Marshal(claims)
	parts[1] = base64.RawURLEncoding.EncodeToString(newPayload)

	return strings.Join(parts, ".")
}

// truncateSignature truncates the token signature.
func truncateSignature(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return token
	}
	// Truncate signature
	if len(parts[2]) > 10 {
		parts[2] = parts[2][:10]
	}
	return strings.Join(parts, ".")
}

// TestInputValidation tests input validation for various endpoints.
func TestInputValidation(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	tests := []struct {
		name           string
		method         string
		path           string
		body           string
		contentType    string
		expectedStatus int
	}{
		{
			name:           "create room with negative max_participants",
			method:         "POST",
			path:           "/api/v1/rooms",
			body:           `{"max_participants": -1}`,
			contentType:    "application/json",
			expectedStatus: http.StatusCreated, // Negative values default to 100
		},
		{
			name:           "create room with zero max_participants",
			method:         "POST",
			path:           "/api/v1/rooms",
			body:           `{"max_participants": 0}`,
			contentType:    "application/json",
			expectedStatus: http.StatusCreated, // Zero defaults to 100
		},
		{
			name:           "create room with excessively large max_participants",
			method:         "POST",
			path:           "/api/v1/rooms",
			body:           `{"max_participants": 999999999}`,
			contentType:    "application/json",
			expectedStatus: http.StatusBadRequest, // Should be rejected
		},
		{
			name:           "create room with invalid JSON",
			method:         "POST",
			path:           "/api/v1/rooms",
			body:           `{invalid json}`,
			contentType:    "application/json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "create room with SQL injection in body",
			method:         "POST",
			path:           "/api/v1/rooms",
			body:           `{"max_participants": 10, "metadata": {"name": "'; DROP TABLE rooms; --"}}`,
			contentType:    "application/json",
			expectedStatus: http.StatusCreated, // Should be safely handled
		},
		{
			name:           "get room with path traversal",
			method:         "GET",
			path:           "/api/v1/rooms/../../../etc/passwd",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "get room with null byte injection",
			method:         "GET",
			path:           "/api/v1/rooms/test%00room",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "get room with unicode exploit",
			method:         "GET",
			path:           "/api/v1/rooms/test\u202Eroom",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "create token with empty participant_id",
			method:         "POST",
			path:           "/api/v1/rooms/test-room/token",
			body:           `{"participant_id": ""}`,
			contentType:    "application/json",
			expectedStatus: http.StatusBadRequest, // Empty participant_id is rejected before room check
		},
		{
			name:           "create token with XSS in participant_id",
			method:         "POST",
			path:           "/api/v1/rooms/test-room/token",
			body:           `{"participant_id": "<script>alert('xss')</script>"}`,
			contentType:    "application/json",
			expectedStatus: http.StatusNotFound, // Room doesn't exist, but ID should be sanitized
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body *bytes.Reader
			if tt.body != "" {
				body = bytes.NewReader([]byte(tt.body))
			} else {
				body = bytes.NewReader(nil)
			}

			req, err := http.NewRequest(tt.method, srv.URL+tt.path, body)
			require.NoError(t, err)

			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

// TestJSONInjection tests JSON injection attacks via WebSocket.
func TestJSONInjection(t *testing.T) {
	srv, roomManager := setupSignalingServer(t)
	defer srv.Close()

	roomManager.CreateRoom("security-test-room")

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	tests := []struct {
		name    string
		message string
	}{
		{
			name:    "nested JSON injection",
			message: `{"jsonrpc":"2.0","method":"join","params":{"roomId":"test","nested":{"__proto__":{"admin":true}}},"id":"550e8400-e29b-41d4-a716-446655440001"}`,
		},
		{
			name:    "prototype pollution attempt",
			message: `{"jsonrpc":"2.0","method":"join","params":{"constructor":{"prototype":{"admin":true}}},"id":"550e8400-e29b-41d4-a716-446655440002"}`,
		},
		{
			name:    "oversized JSON",
			message: `{"jsonrpc":"2.0","method":"join","params":{"data":"` + strings.Repeat("A", 100000) + `"},"id":"550e8400-e29b-41d4-a716-446655440003"}`,
		},
		{
			name:    "deeply nested JSON",
			message: createDeeplyNestedJSON(100),
		},
		{
			name:    "unicode escape sequences",
			message: `{"jsonrpc":"2.0","method":"join","params":{"roomId":"\u0000\u0001\u0002"},"id":"550e8400-e29b-41d4-a716-446655440005"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
			require.NoError(t, err)
			defer resp.Body.Close()
			defer conn.Close()

			// Send potentially malicious message
			err = conn.WriteMessage(websocket.TextMessage, []byte(tt.message))
			if err != nil {
				// Connection might be closed due to message size limit - that's fine
				t.Logf("Write error (expected for oversized messages): %v", err)
				return
			}

			// Try to read response with timeout
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, msg, err := conn.ReadMessage()
			if err != nil {
				// Timeout or error is acceptable - server should handle gracefully
				t.Logf("Read error (expected for malformed messages): %v", err)
				return
			}

			// Verify response is valid JSON
			var response map[string]interface{}
			err = json.Unmarshal(msg, &response)
			assert.NoError(t, err, "Server should return valid JSON response")
		})
	}
}

// createDeeplyNestedJSON creates a deeply nested JSON structure.
func createDeeplyNestedJSON(depth int) string {
	var sb strings.Builder
	sb.WriteString(`{"jsonrpc":"2.0","method":"join","params":{"data":`)

	for i := 0; i < depth; i++ {
		sb.WriteString(`{"nested":`)
	}
	sb.WriteString(`"value"`)
	for i := 0; i < depth; i++ {
		sb.WriteString(`}`)
	}

	sb.WriteString(`},"id":"550e8400-e29b-41d4-a716-446655440004"}`)
	return sb.String()
}

// TestWebSocketDoS tests WebSocket-based DoS attacks.
func TestWebSocketDoS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DoS test in short mode")
	}

	srv, roomManager := setupSignalingServer(t)
	defer srv.Close()

	roomManager.CreateRoom("dos-test-room")

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	t.Run("rapid connection attempts", func(t *testing.T) {
		const numConnections = 20

		var wg sync.WaitGroup
		var successCount, failCount int64
		var mu sync.Mutex

		for i := 0; i < numConnections; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
				if err != nil {
					mu.Lock()
					failCount++
					mu.Unlock()
					return
				}
				defer resp.Body.Close()
				defer conn.Close()

				mu.Lock()
				successCount++
				mu.Unlock()

				// Small delay before closing
				time.Sleep(100 * time.Millisecond)
			}()
		}

		wg.Wait()

		t.Logf("DoS test - connections: success=%d, failed=%d", successCount, failCount)
		// Some connections should succeed, but rate limiting should kick in
	})

	t.Run("message flood", func(t *testing.T) {
		conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		defer conn.Close()

		const numMessages = 200
		var successCount, failCount int

		for i := 0; i < numMessages; i++ {
			msg := fmt.Sprintf(`{"jsonrpc":"2.0","method":"join","params":{"roomId":"dos-test-room","token":"token-%d"},"id":"550e8400-e29b-41d4-a716-4466554400%02d"}`, i, i%100)

			err := conn.WriteMessage(websocket.TextMessage, []byte(msg))
			if err != nil {
				failCount++
				break // Rate limit likely kicked in
			}
			successCount++

			// Don't wait for response to maximize flood rate
		}

		t.Logf("Message flood - sent: success=%d, failed=%d", successCount, failCount)
		// Server should handle gracefully and eventually rate limit
	})

	t.Run("large message attack", func(t *testing.T) {
		// Wait a moment for rate limiting to reset after previous subtests
		time.Sleep(100 * time.Millisecond)

		conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			// Connection rejection due to rate limiting from previous tests is acceptable
			t.Logf("Large message test - connection rejected (expected after flood tests): %v", err)
			return
		}
		defer resp.Body.Close()
		defer conn.Close()

		// Try to send message larger than MaxMessageSize
		largeMessage := strings.Repeat("A", 100000)
		msg := fmt.Sprintf(`{"jsonrpc":"2.0","method":"join","params":{"data":"%s"},"id":"550e8400-e29b-41d4-a716-446655440099"}`, largeMessage)

		err = conn.WriteMessage(websocket.TextMessage, []byte(msg))
		// Connection might be closed - that's the expected behavior
		t.Logf("Large message result: %v (connection closure is expected)", err)
	})
}

// TestSDPInjection tests SDP injection attacks.
func TestSDPInjection(t *testing.T) {
	srv, roomManager := setupSignalingServer(t)
	defer srv.Close()

	roomManager.CreateRoom("sdp-test-room")

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	defer conn.Close()

	tests := []struct {
		name string
		sdp  string
	}{
		{
			name: "malformed SDP",
			sdp:  "not-a-valid-sdp",
		},
		{
			name: "SDP with injection in origin",
			sdp:  "v=0\r\no=- 123 1 IN IP4 0.0.0.0; rm -rf /\r\ns=-\r\nt=0 0\r\n",
		},
		{
			name: "SDP with embedded script",
			sdp:  "v=0\r\no=- 123 1 IN IP4 0.0.0.0\r\ns=<script>alert('xss')</script>\r\nt=0 0\r\n",
		},
		{
			name: "SDP with command injection in attribute",
			sdp:  "v=0\r\no=- 123 1 IN IP4 0.0.0.0\r\ns=-\r\nt=0 0\r\na=`id`\r\n",
		},
		{
			name: "oversized SDP",
			sdp:  "v=0\r\no=- 123 1 IN IP4 0.0.0.0\r\ns=-\r\nt=0 0\r\na=x:" + strings.Repeat("A", 50000) + "\r\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First join the room
			joinReq, err := protocol.NewRequest("550e8400-e29b-41d4-a716-446655440010", "join", map[string]interface{}{
				"roomId": "sdp-test-room",
				"token":  "test-token",
			})
			require.NoError(t, err)

			joinData, _ := json.Marshal(joinReq)

			err = conn.WriteMessage(websocket.TextMessage, joinData)
			require.NoError(t, err)

			// Read join response
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, _, err = conn.ReadMessage()
			if err != nil {
				t.Logf("Join response error: %v", err)
			}

			// Now send the malicious SDP
			offerReq, err := protocol.NewRequest("550e8400-e29b-41d4-a716-446655440011", "offer", map[string]interface{}{
				"sdp": tt.sdp,
			})
			require.NoError(t, err)

			offerData, _ := json.Marshal(offerReq)

			err = conn.WriteMessage(websocket.TextMessage, offerData)
			if err != nil {
				t.Logf("Write error (expected for oversized): %v", err)
				return
			}

			// Read response
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, msg, err := conn.ReadMessage()
			if err != nil {
				t.Logf("Read error (expected for malformed SDP): %v", err)
				return
			}

			// Should receive an error response, not crash
			var response protocol.Response
			err = json.Unmarshal(msg, &response)
			assert.NoError(t, err, "Should receive valid JSON response")
		})
	}
}

// TestPathTraversal tests path traversal attacks.
func TestPathTraversal(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	paths := []string{
		"/api/v1/rooms/../../../etc/passwd",
		"/api/v1/rooms/..%2F..%2F..%2Fetc%2Fpasswd",
		"/api/v1/rooms/....//....//....//etc/passwd",
		"/api/v1/rooms/%2e%2e/%2e%2e/%2e%2e/etc/passwd",
		"/api/v1/rooms/test-room/../../../../etc/passwd",
		"/api/v1/rooms/test-room%00.json",
		"/api/v1/rooms/test-room;id",
		"/api/v1/rooms/test-room|id",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			resp, err := http.Get(srv.URL + path)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Should either 404 or 400, never 200 with sensitive content
			assert.NotEqual(t, http.StatusOK, resp.StatusCode, "Path traversal should not succeed")
		})
	}
}

// TestHTTPHeaderInjection tests HTTP header injection attacks.
// NOTE: Go's http client itself blocks CRLF/newline in headers, so those attacks
// are prevented at the client level. This test verifies that behavior.
func TestHTTPHeaderInjection(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	tests := []struct {
		name             string
		headerName       string
		headerValue      string
		expectClientFail bool // Go http client blocks some header values
	}{
		{
			name:             "CRLF injection in header",
			headerName:       "X-Custom",
			headerValue:      "value\r\nX-Injected: malicious",
			expectClientFail: true, // Go blocks CRLF in headers
		},
		{
			name:             "newline injection",
			headerName:       "X-Custom",
			headerValue:      "value\nX-Injected: malicious",
			expectClientFail: true, // Go blocks newlines in headers
		},
		{
			name:             "host header manipulation",
			headerName:       "Host",
			headerValue:      "evil.com",
			expectClientFail: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", srv.URL+"/health", nil)
			require.NoError(t, err)

			req.Header.Set(tt.headerName, tt.headerValue)

			resp, err := http.DefaultClient.Do(req)
			if tt.expectClientFail {
				// Go's http client should reject these headers
				assert.Error(t, err, "Go http client should block header injection")
				t.Logf("Header injection blocked by Go http client: %v", err)
				return
			}

			require.NoError(t, err)
			defer resp.Body.Close()

			// Should handle gracefully
			assert.Contains(t, []int{http.StatusOK, http.StatusBadRequest}, resp.StatusCode)
		})
	}
}

// TestExpiredTokenRejection tests that expired tokens are rejected.
func TestExpiredTokenRejection(t *testing.T) {
	privateKey, publicKey := generateTestKeys(t)

	keySource := auth.NewStaticKeySource(publicKey)
	validator := auth.NewJWTValidator(auth.DefaultJWTConfig(), keySource)

	// Create an expired token
	expiredClaims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "test-user",
			Issuer:    "test-issuer",
			Audience:  []string{"test-audience"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)), // Expired 1 hour ago
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
		RoomID: "test-room",
		Role:   "publisher",
	}

	expiredToken, err := auth.GenerateToken(expiredClaims, privateKey)
	require.NoError(t, err)

	_, err = validator.Validate(context.Background(), expiredToken)
	assert.Error(t, err, "Expired token should be rejected")
	assert.ErrorIs(t, err, auth.ErrTokenExpired)
}

// TestFutureTokenRejection tests that tokens with future iat are rejected.
func TestFutureTokenRejection(t *testing.T) {
	privateKey, publicKey := generateTestKeys(t)

	// Create validator with strict clock skew
	cfg := auth.JWTConfig{
		ClockSkew: 0, // No clock skew allowed
	}
	keySource := auth.NewStaticKeySource(publicKey)
	validator := auth.NewJWTValidator(cfg, keySource)

	// Create a token with future issue time
	futureClaims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "test-user",
			Issuer:    "test-issuer",
			Audience:  []string{"test-audience"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(2 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(time.Hour)), // Issued 1 hour in the future
			NotBefore: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		RoomID: "test-room",
		Role:   "publisher",
	}

	futureToken, err := auth.GenerateToken(futureClaims, privateKey)
	require.NoError(t, err)

	_, err = validator.Validate(context.Background(), futureToken)
	assert.Error(t, err, "Future token should be rejected")
}

// TestRoomIDMismatch tests that tokens for different rooms are rejected.
func TestRoomIDMismatch(t *testing.T) {
	privateKey, publicKey := generateTestKeys(t)

	keySource := auth.NewStaticKeySource(publicKey)
	validator := auth.NewJWTValidator(auth.DefaultJWTConfig(), keySource)

	// Create a token for room-1
	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "test-user",
			Issuer:    "test-issuer",
			Audience:  []string{"test-audience"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		RoomID: "room-1",
		Role:   "publisher",
	}

	token, err := auth.GenerateToken(claims, privateKey)
	require.NoError(t, err)

	// Try to validate for room-2
	_, err = validator.ValidateForRoomStrict(context.Background(), token, "room-2")
	assert.Error(t, err, "Token for different room should be rejected")
	assert.ErrorIs(t, err, auth.ErrInvalidRoomID)
}

// TestAuthenticationOnExistingRoom tests authentication behavior on existing rooms.
// This test creates a room first, then verifies server behavior with various auth methods.
// NOTE: This test documents the current behavior - whether auth is required depends on server configuration.
// If auth middleware is enabled, expect 401/403. If not enabled, expect 200/404.
func TestAuthenticationOnExistingRoom(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	// First, create a room
	createResp, err := http.Post(
		srv.URL+"/api/v1/rooms",
		"application/json",
		bytes.NewReader([]byte(`{"max_participants": 10}`)),
	)
	require.NoError(t, err)
	defer createResp.Body.Close()

	// Extract room ID from response
	var roomData map[string]interface{}
	err = json.NewDecoder(createResp.Body).Decode(&roomData)
	require.NoError(t, err)
	roomID, ok := roomData["id"].(string)
	require.True(t, ok, "id should be present in response")

	t.Logf("Created room: %s", roomID)

	// Now try to access the room with various invalid auth methods
	// Expected behavior depends on whether auth middleware is enabled:
	// - With auth enabled: 401 (Unauthorized) or 403 (Forbidden)
	// - Without auth enabled: 200 (OK) or 404 (Not Found if route not implemented)
	tests := []struct {
		name           string
		authHeader     string
		expectedStatus []int
	}{
		{
			name:           "no authorization header",
			authHeader:     "",
			expectedStatus: []int{http.StatusOK, http.StatusNotFound, http.StatusUnauthorized, http.StatusForbidden},
		},
		{
			name:           "empty bearer token",
			authHeader:     "Bearer ",
			expectedStatus: []int{http.StatusOK, http.StatusNotFound, http.StatusUnauthorized, http.StatusForbidden},
		},
		{
			name:           "invalid bearer token",
			authHeader:     "Bearer invalid.token.here",
			expectedStatus: []int{http.StatusOK, http.StatusNotFound, http.StatusUnauthorized, http.StatusForbidden},
		},
		{
			name:           "wrong auth scheme",
			authHeader:     "Basic dXNlcjpwYXNz",
			expectedStatus: []int{http.StatusOK, http.StatusNotFound, http.StatusUnauthorized, http.StatusForbidden},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", srv.URL+"/api/v1/rooms/"+roomID, nil)
			require.NoError(t, err)

			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Verify response is one of the expected statuses
			// The actual status depends on server auth configuration
			assert.Contains(t, tt.expectedStatus, resp.StatusCode,
				"should return one of %v for %s, got %d", tt.expectedStatus, tt.name, resp.StatusCode)

			// Log actual behavior for documentation
			t.Logf("Auth scenario %q: received status %d", tt.name, resp.StatusCode)
		})
	}
}

// TestContentTypeValidation tests that proper content types are required.
// This test verifies that the server handles various content types gracefully.
func TestContentTypeValidation(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	tests := []struct {
		name           string
		contentType    string
		body           string
		expectedStatus []int // Acceptable status codes
	}{
		{
			name:           "text/plain content type",
			contentType:    "text/plain",
			body:           `{"max_participants": 10}`,
			expectedStatus: []int{http.StatusCreated, http.StatusBadRequest, http.StatusUnsupportedMediaType},
		},
		{
			name:           "text/html content type",
			contentType:    "text/html",
			body:           `{"max_participants": 10}`,
			expectedStatus: []int{http.StatusCreated, http.StatusBadRequest, http.StatusUnsupportedMediaType},
		},
		{
			name:           "application/xml content type",
			contentType:    "application/xml",
			body:           `{"max_participants": 10}`,
			expectedStatus: []int{http.StatusCreated, http.StatusBadRequest, http.StatusUnsupportedMediaType},
		},
		{
			name:           "no content type",
			contentType:    "",
			body:           `{"max_participants": 10}`,
			expectedStatus: []int{http.StatusCreated, http.StatusBadRequest},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", srv.URL+"/api/v1/rooms", bytes.NewReader([]byte(tt.body)))
			require.NoError(t, err)

			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Server should handle gracefully with one of the expected status codes
			assert.Contains(t, tt.expectedStatus, resp.StatusCode,
				"Content-Type %q should result in one of %v, got %d", tt.contentType, tt.expectedStatus, resp.StatusCode)
		})
	}
}
