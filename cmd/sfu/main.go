package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/HMasataka/choice/internal/server"
	"github.com/HMasataka/choice/pkg/config"
	"github.com/HMasataka/choice/pkg/logger"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		fmt.Printf("Received signal: %v\n", sig)
		cancel()
	}()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Use default config if file not found
			cfg = config.DefaultConfig()
		} else {
			return fmt.Errorf("failed to load config: %w", err)
		}
	}

	// Initialize logger
	log, err := logger.New(logger.Config{
		Level:      cfg.Logging.Level,
		Format:     cfg.Logging.Format,
		Output:     cfg.Logging.Output,
		PIIMasking: cfg.Logging.PIIMasking,
	})
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	logger.SetDefault(log)

	log.Info("starting SFU server",
		"version", "0.1.0",
		"config", *configPath,
	)

	// Create and start HTTP server
	srv := server.New(cfg.Server, log)

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil {
			errCh <- err
		}
	}()

	// Wait for shutdown signal or error
	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	// Graceful shutdown
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("failed to shutdown server", "error", err)
	}

	log.Info("SFU server stopped")
	return nil
}
