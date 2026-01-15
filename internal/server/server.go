package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/HMasataka/choice/pkg/config"
	"github.com/HMasataka/choice/pkg/logger"
)

// Server represents the HTTP server.
type Server struct {
	httpServer *http.Server
	router     *http.ServeMux
	config     config.ServerConfig
	logger     *logger.Logger
}

// New creates a new Server instance.
func New(cfg config.ServerConfig, log *logger.Logger) *Server {
	router := http.NewServeMux()

	s := &Server{
		router: router,
		config: cfg,
		logger: log,
	}

	s.setupRoutes()

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.HTTP.Host, cfg.HTTP.Port),
		Handler:      router,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
	}

	return s
}

// setupRoutes configures the HTTP routes.
func (s *Server) setupRoutes() {
	// Health check endpoints
	s.router.HandleFunc("GET /health", s.handleHealth)
	s.router.HandleFunc("GET /ready", s.handleReady)

	// API v1 routes
	s.router.HandleFunc("GET /api/v1/rooms/{id}", s.handleGetRoom)
	s.router.HandleFunc("POST /api/v1/rooms", s.handleCreateRoom)
	s.router.HandleFunc("DELETE /api/v1/rooms/{id}", s.handleDeleteRoom)
	s.router.HandleFunc("GET /api/v1/rooms/{id}/participants", s.handleGetParticipants)
	s.router.HandleFunc("POST /api/v1/rooms/{id}/token", s.handleCreateToken)
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

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("failed to shutdown HTTP server: %w", err)
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
