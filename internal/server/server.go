package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/HMasataka/choice/internal/recording"
	"github.com/HMasataka/choice/internal/recording/storage"
	"github.com/HMasataka/choice/internal/room"
	"github.com/HMasataka/choice/internal/store"
	"github.com/HMasataka/choice/pkg/config"
	"github.com/HMasataka/choice/pkg/logger"
	"github.com/HMasataka/choice/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// TokenGenerator generates JWT tokens for room access.
type TokenGenerator interface {
	GenerateToken(roomID, participantID, role string, expiresInSeconds int) (string, error)
}

// Server represents the HTTP server.
type Server struct {
	httpServer       *http.Server
	router           *http.ServeMux
	config           config.ServerConfig
	metricsConfig    config.MetricsConfig
	recordingConfig  config.RecordingConfig
	logger           *logger.Logger
	webrtcComponents *WebRTCComponents
	roomManager      *room.Manager
	sessionStore     store.SessionStore
	tokenGenerator   TokenGenerator
	recordingService *recording.RecordingService
	// metrics holds the Prometheus metrics instance for future integration
	// with room/connection lifecycle events. Currently exposed via /metrics endpoint.
	metrics *metrics.Metrics
}

// New creates a new Server instance.
func New(cfg *config.Config, log *logger.Logger) *Server {
	router := http.NewServeMux()

	// Initialize Room Manager
	roomManager := room.NewManager(log)

	// Initialize Session Store (use in-memory for now)
	sessionStore := store.NewMemoryStore()

	// Initialize metrics if enabled
	var m *metrics.Metrics
	if cfg.Metrics.Enabled {
		m = metrics.GetInstance()
	}

	// Initialize recording service if enabled
	var recordingService *recording.RecordingService
	if cfg.Recording.Enabled {
		var recordingStorage storage.Storage
		var storageErr error
		// Initialize storage based on config
		switch cfg.Recording.Storage.Type {
		case "gcs":
			recordingStorage, storageErr = storage.NewGCSStorage(context.Background(), storage.GCSConfig{
				Bucket:    cfg.Recording.Storage.Bucket,
				ProjectID: cfg.Recording.Storage.ProjectID,
			})
			if storageErr != nil {
				log.Error("failed to initialize GCS storage for recording", "error", storageErr)
			}
		case "local":
			recordingStorage, storageErr = storage.NewLocalStorage(cfg.Recording.TempDir)
			if storageErr != nil {
				log.Error("failed to initialize local storage for recording", "error", storageErr)
			}
		default:
			log.Warn("unknown recording storage type, using local storage", "type", cfg.Recording.Storage.Type)
			recordingStorage, storageErr = storage.NewLocalStorage(cfg.Recording.TempDir)
			if storageErr != nil {
				log.Error("failed to initialize local storage for recording", "error", storageErr)
			}
		}

		// If storage initialization failed, disable recording
		recordingEnabled := cfg.Recording.Enabled
		if storageErr != nil {
			log.Warn("recording disabled due to storage initialization failure")
			recordingEnabled = false
		}

		recordingService = recording.NewRecordingService(recording.RecordingServiceConfig{
			Enabled: recordingEnabled,
			TempDir: cfg.Recording.TempDir,
			Format:  cfg.Recording.Format,
			Storage: recordingStorage,
			Logger:  log,
		})
		if recordingEnabled {
			log.Info("recording service initialized", "format", cfg.Recording.Format, "temp_dir", cfg.Recording.TempDir)
		}
	}

	s := &Server{
		router:           router,
		config:           cfg.Server,
		metricsConfig:    cfg.Metrics,
		recordingConfig:  cfg.Recording,
		logger:           log,
		roomManager:      roomManager,
		sessionStore:     sessionStore,
		recordingService: recordingService,
		metrics:          m,
	}

	// Initialize WebRTC components
	webrtcComponents, err := InitializeWebRTC(cfg, log)
	if err != nil {
		log.Error("failed to initialize WebRTC components", "error", err)
		// Continue without WebRTC components for graceful degradation
		// Handlers will use stub mode
	} else {
		s.webrtcComponents = webrtcComponents
		log.Info("WebRTC components initialized successfully")
	}

	s.setupRoutes()

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.HTTP.Host, cfg.Server.HTTP.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.HTTP.ReadTimeout,
		WriteTimeout: cfg.Server.HTTP.WriteTimeout,
	}

	return s
}

// setupRoutes configures the HTTP routes.
func (s *Server) setupRoutes() {
	// Health check endpoints
	s.router.HandleFunc("GET /health", s.handleHealth)
	s.router.HandleFunc("GET /ready", s.handleReady)

	// WebSocket signaling endpoint
	if s.webrtcComponents != nil && s.webrtcComponents.Handler != nil {
		s.router.HandleFunc("GET /ws", s.webrtcComponents.Handler.ServeHTTP)
	}

	// API v1 routes
	s.router.HandleFunc("GET /api/v1/rooms/{id}", s.handleGetRoom)
	s.router.HandleFunc("POST /api/v1/rooms", s.handleCreateRoom)
	s.router.HandleFunc("DELETE /api/v1/rooms/{id}", s.handleDeleteRoom)
	s.router.HandleFunc("GET /api/v1/rooms/{id}/participants", s.handleGetParticipants)
	s.router.HandleFunc("POST /api/v1/rooms/{id}/token", s.handleCreateToken)
	s.router.HandleFunc("POST /api/v1/rooms/{id}/lock", s.handleLockRoom)
	s.router.HandleFunc("DELETE /api/v1/rooms/{id}/lock", s.handleUnlockRoom)

	// Recording API routes
	s.router.HandleFunc("POST /api/v1/rooms/{id}/recording", s.handleStartRecording)
	s.router.HandleFunc("DELETE /api/v1/rooms/{id}/recording", s.handleStopRecording)
	s.router.HandleFunc("GET /api/v1/rooms/{id}/recording", s.handleGetRecording)

	// Metrics endpoint
	// Per tasks.md 4.1.2: GET /metrics - Prometheus metrics endpoint
	if s.metricsConfig.Enabled {
		metricsPath := s.metricsConfig.Path
		if metricsPath == "" {
			metricsPath = "/metrics"
		}
		s.router.Handle("GET "+metricsPath, promhttp.Handler())
		s.logger.Info("metrics endpoint enabled", "path", metricsPath)
	}
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	s.logger.Info("starting HTTP server",
		"addr", s.httpServer.Addr,
	)

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the server.
// Note: Uses a fresh context to ensure graceful shutdown even if the caller's
// context is already canceled.
func (s *Server) Shutdown(_ context.Context) error {
	s.logger.Info("shutting down HTTP server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown HTTP server
	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("failed to shutdown HTTP server: %w", err)
	}

	// Shutdown recording service
	if s.recordingService != nil {
		if err := s.recordingService.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("failed to shutdown recording service", "error", err)
		}
	}

	// Close room manager
	if s.roomManager != nil {
		if err := s.roomManager.Close(); err != nil {
			s.logger.Error("failed to close room manager", "error", err)
		}
	}

	// Close session store
	if s.sessionStore != nil {
		if err := s.sessionStore.Close(); err != nil {
			s.logger.Error("failed to close session store", "error", err)
		}
	}

	return nil
}

// Router returns the HTTP router for external middleware setup.
func (s *Server) Router() *http.ServeMux {
	return s.router
}

// SetHandler sets a custom handler (useful for wrapping with middleware).
func (s *Server) SetHandler(handler http.Handler) {
	s.httpServer.Handler = handler
}

// SetTokenGenerator sets the token generator for the server.
func (s *Server) SetTokenGenerator(tg TokenGenerator) {
	s.tokenGenerator = tg
}
