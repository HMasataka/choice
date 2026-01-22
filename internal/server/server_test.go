package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/HMasataka/choice/pkg/config"
	"github.com/HMasataka/choice/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	cfg := config.DefaultConfig()
	log, err := logger.New(logger.DefaultConfig())
	require.NoError(t, err)

	s := New(cfg, log)

	assert.NotNil(t, s)
	assert.NotNil(t, s.router)
	assert.NotNil(t, s.roomManager)
	assert.NotNil(t, s.sessionStore)
	assert.NotNil(t, s.logger)
}

func TestNewWithMetrics(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Metrics.Enabled = true
	cfg.Metrics.Path = "/metrics"

	log, err := logger.New(logger.DefaultConfig())
	require.NoError(t, err)

	s := New(cfg, log)

	assert.NotNil(t, s)
	assert.NotNil(t, s.metrics)
}

func TestNewWithRecording(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Recording.Enabled = true
	cfg.Recording.Storage.Type = "local"
	cfg.Recording.TempDir = t.TempDir()
	cfg.Recording.Format = "webm"

	log, err := logger.New(logger.DefaultConfig())
	require.NoError(t, err)

	s := New(cfg, log)

	assert.NotNil(t, s)
	assert.NotNil(t, s.recordingService)
}

func TestNewWithRecordingInvalidStorageType(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Recording.Enabled = true
	cfg.Recording.Storage.Type = "invalid"
	cfg.Recording.TempDir = t.TempDir()

	log, err := logger.New(logger.DefaultConfig())
	require.NoError(t, err)

	// Should not panic and fall back to local storage
	s := New(cfg, log)
	assert.NotNil(t, s)
}

func TestServer_Router(t *testing.T) {
	cfg := config.DefaultConfig()
	log, err := logger.New(logger.DefaultConfig())
	require.NoError(t, err)

	s := New(cfg, log)

	router := s.Router()
	assert.NotNil(t, router)
	assert.Equal(t, s.router, router)
}

func TestServer_SetHandler(t *testing.T) {
	cfg := config.DefaultConfig()
	log, err := logger.New(logger.DefaultConfig())
	require.NoError(t, err)

	s := New(cfg, log)

	customHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	s.SetHandler(customHandler)

	// Verify by making a request and checking response
	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	w := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTeapot, w.Code)
}

func TestServer_SetTokenGenerator(t *testing.T) {
	cfg := config.DefaultConfig()
	log, err := logger.New(logger.DefaultConfig())
	require.NoError(t, err)

	s := New(cfg, log)

	mockTG := &mockTokenGenerator{}
	s.SetTokenGenerator(mockTG)

	assert.Equal(t, mockTG, s.tokenGenerator)
}

func TestServer_Shutdown(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.HTTP.Port = 0 // Use any available port

	log, err := logger.New(logger.DefaultConfig())
	require.NoError(t, err)

	s := New(cfg, log)

	// Start server in a goroutine
	serverStarted := make(chan struct{})
	go func() {
		close(serverStarted)
		s.Start()
	}()

	<-serverStarted
	// Give the server a moment to actually start
	time.Sleep(50 * time.Millisecond)

	// Shutdown should not error
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = s.Shutdown(ctx)
	assert.NoError(t, err)
}

func TestHandleStartRecordingNotEnabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Recording.Enabled = false

	log, err := logger.New(logger.DefaultConfig())
	require.NoError(t, err)

	s := New(cfg, log)

	// Create a room first
	_, err = s.roomManager.CreateRoom("test-room")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/test-room/recording", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleStartRecordingRoomNotFound(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Recording.Enabled = true
	cfg.Recording.Storage.Type = "local"
	cfg.Recording.TempDir = t.TempDir()

	log, err := logger.New(logger.DefaultConfig())
	require.NoError(t, err)

	s := New(cfg, log)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/nonexistent/recording", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleStopRecordingNotEnabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Recording.Enabled = false

	log, err := logger.New(logger.DefaultConfig())
	require.NoError(t, err)

	s := New(cfg, log)

	// Create a room first
	_, err = s.roomManager.CreateRoom("test-room")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/rooms/test-room/recording", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleStopRecordingRoomNotFound(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Recording.Enabled = true
	cfg.Recording.Storage.Type = "local"
	cfg.Recording.TempDir = t.TempDir()

	log, err := logger.New(logger.DefaultConfig())
	require.NoError(t, err)

	s := New(cfg, log)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/rooms/nonexistent/recording", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleGetRecordingNotEnabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Recording.Enabled = false

	log, err := logger.New(logger.DefaultConfig())
	require.NoError(t, err)

	s := New(cfg, log)

	// Create a room first
	_, err = s.roomManager.CreateRoom("test-room")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/test-room/recording", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleGetRecordingRoomNotFound(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Recording.Enabled = true
	cfg.Recording.Storage.Type = "local"
	cfg.Recording.TempDir = t.TempDir()

	log, err := logger.New(logger.DefaultConfig())
	require.NoError(t, err)

	s := New(cfg, log)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/nonexistent/recording", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleStartRecordingSuccess(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Recording.Enabled = true
	cfg.Recording.Storage.Type = "local"
	cfg.Recording.TempDir = t.TempDir()
	cfg.Recording.Format = "webm"

	log, err := logger.New(logger.DefaultConfig())
	require.NoError(t, err)

	s := New(cfg, log)

	// Create a room first
	_, err = s.roomManager.CreateRoom("test-room")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/test-room/recording", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	// Should succeed (201 Created for starting a new recording)
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestHandleGetRecordingWhenRecording(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Recording.Enabled = true
	cfg.Recording.Storage.Type = "local"
	cfg.Recording.TempDir = t.TempDir()
	cfg.Recording.Format = "webm"

	log, err := logger.New(logger.DefaultConfig())
	require.NoError(t, err)

	s := New(cfg, log)

	// Create a room first
	_, err = s.roomManager.CreateRoom("test-room")
	require.NoError(t, err)

	// Start recording
	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/test-room/recording", nil)
	startW := httptest.NewRecorder()
	s.router.ServeHTTP(startW, startReq)
	require.Equal(t, http.StatusCreated, startW.Code)

	// Get recording status
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/test-room/recording", nil)
	getW := httptest.NewRecorder()
	s.router.ServeHTTP(getW, getReq)

	assert.Equal(t, http.StatusOK, getW.Code)
}

func TestHandleStopRecordingWhenRecording(t *testing.T) {
	// Use a separate temp directory that we manually clean up
	tempDir, err := os.MkdirTemp("", "recording-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	cfg := config.DefaultConfig()
	cfg.Recording.Enabled = true
	cfg.Recording.Storage.Type = "local"
	cfg.Recording.TempDir = tempDir
	cfg.Recording.Format = "webm"

	log, err := logger.New(logger.DefaultConfig())
	require.NoError(t, err)

	s := New(cfg, log)
	defer s.Shutdown(context.Background()) // Ensure clean shutdown

	// Create a room first
	_, err = s.roomManager.CreateRoom("test-room")
	require.NoError(t, err)

	// Start recording
	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/test-room/recording", nil)
	startW := httptest.NewRecorder()
	s.router.ServeHTTP(startW, startReq)
	require.Equal(t, http.StatusCreated, startW.Code)

	// Stop recording
	stopReq := httptest.NewRequest(http.MethodDelete, "/api/v1/rooms/test-room/recording", nil)
	stopW := httptest.NewRecorder()
	s.router.ServeHTTP(stopW, stopReq)

	assert.Equal(t, http.StatusOK, stopW.Code)

	// Wait for upload to complete
	time.Sleep(100 * time.Millisecond)
}

func TestHandleMetricsEndpoint(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Metrics.Enabled = true
	cfg.Metrics.Path = "/metrics"

	log, err := logger.New(logger.DefaultConfig())
	require.NoError(t, err)

	s := New(cfg, log)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	// Prometheus metrics endpoint should return 200
	assert.Equal(t, http.StatusOK, w.Code)
}
