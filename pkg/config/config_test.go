package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.HTTP.Host != "0.0.0.0" {
		t.Errorf("expected HTTP host to be '0.0.0.0', got '%s'", cfg.Server.HTTP.Host)
	}
	if cfg.Server.HTTP.Port != 8080 {
		t.Errorf("expected HTTP port to be 8080, got %d", cfg.Server.HTTP.Port)
	}
	if !cfg.WebRTC.ICELite {
		t.Error("expected ICE Lite to be enabled by default")
	}
	if cfg.Room.MaxParticipants != 100 {
		t.Errorf("expected max participants to be 100, got %d", cfg.Room.MaxParticipants)
	}
	if cfg.Room.MaxTracksPerRoom != 500 {
		t.Errorf("expected max tracks per room to be 500, got %d", cfg.Room.MaxTracksPerRoom)
	}
	if !cfg.Media.Simulcast.Enabled {
		t.Error("expected simulcast to be enabled by default")
	}
	if len(cfg.Media.Simulcast.Layers) != 3 {
		t.Errorf("expected 3 simulcast layers, got %d", len(cfg.Media.Simulcast.Layers))
	}
}

func TestLoad(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  http:
    host: "127.0.0.1"
    port: 9090
    read_timeout: 60s
    write_timeout: 60s

webrtc:
  ice_lite: false
  port_range:
    min: 20000
    max: 30000

room:
  max_participants: 50
  empty_timeout: 10m
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write temp config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Check overridden values
	if cfg.Server.HTTP.Host != "127.0.0.1" {
		t.Errorf("expected HTTP host to be '127.0.0.1', got '%s'", cfg.Server.HTTP.Host)
	}
	if cfg.Server.HTTP.Port != 9090 {
		t.Errorf("expected HTTP port to be 9090, got %d", cfg.Server.HTTP.Port)
	}
	if cfg.Server.HTTP.ReadTimeout != 60*time.Second {
		t.Errorf("expected read timeout to be 60s, got %v", cfg.Server.HTTP.ReadTimeout)
	}
	if cfg.WebRTC.ICELite {
		t.Error("expected ICE Lite to be disabled")
	}
	if cfg.WebRTC.PortRange.Min != 20000 {
		t.Errorf("expected port range min to be 20000, got %d", cfg.WebRTC.PortRange.Min)
	}
	if cfg.Room.MaxParticipants != 50 {
		t.Errorf("expected max participants to be 50, got %d", cfg.Room.MaxParticipants)
	}
	if cfg.Room.EmptyTimeout != 10*time.Minute {
		t.Errorf("expected empty timeout to be 10m, got %v", cfg.Room.EmptyTimeout)
	}

	// Check default values are preserved for unspecified fields
	if cfg.Metrics.Path != "/metrics" {
		t.Errorf("expected metrics path to be '/metrics', got '%s'", cfg.Metrics.Path)
	}
	if !cfg.Metrics.Enabled {
		t.Error("expected metrics to be enabled by default")
	}
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("expected error when loading nonexistent config file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	invalidContent := `
server:
  http:
    port: "not a number"
`

	if err := os.WriteFile(configPath, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("failed to write temp config file: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error when loading invalid YAML")
	}
}

func TestEnvOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  http:
    host: "0.0.0.0"
    port: 8080
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write temp config file: %v", err)
	}

	// Set environment variables
	t.Setenv("SFU_HTTP_HOST", "192.168.1.1")
	t.Setenv("SFU_HTTP_PORT", "3000")
	t.Setenv("SFU_PUBLIC_IP", "1.2.3.4")
	t.Setenv("SFU_LOG_LEVEL", "DEBUG")
	t.Setenv("SFU_REDIS_ADDRESS", "redis:6379")
	t.Setenv("SFU_STORE_TYPE", "redis")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Server.HTTP.Host != "192.168.1.1" {
		t.Errorf("expected HTTP host to be overridden to '192.168.1.1', got '%s'", cfg.Server.HTTP.Host)
	}
	if cfg.Server.HTTP.Port != 3000 {
		t.Errorf("expected HTTP port to be overridden to 3000, got %d", cfg.Server.HTTP.Port)
	}
	if cfg.WebRTC.PublicIP != "1.2.3.4" {
		t.Errorf("expected public IP to be '1.2.3.4', got '%s'", cfg.WebRTC.PublicIP)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected log level to be 'debug', got '%s'", cfg.Logging.Level)
	}
	if cfg.Store.Redis.Address != "redis:6379" {
		t.Errorf("expected redis address to be 'redis:6379', got '%s'", cfg.Store.Redis.Address)
	}
	if cfg.Store.Type != "redis" {
		t.Errorf("expected store type to be 'redis', got '%s'", cfg.Store.Type)
	}
}
