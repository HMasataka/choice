package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete application configuration.
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	WebRTC    WebRTCConfig    `yaml:"webrtc"`
	Room      RoomConfig      `yaml:"room"`
	Media     MediaConfig     `yaml:"media"`
	Auth      AuthConfig      `yaml:"auth"`
	Recording RecordingConfig `yaml:"recording"`
	Store     StoreConfig     `yaml:"store"`
	Logging   LoggingConfig   `yaml:"logging"`
	Metrics   MetricsConfig   `yaml:"metrics"`
}

// ServerConfig contains HTTP and WebSocket server settings.
type ServerConfig struct {
	HTTP      HTTPConfig      `yaml:"http"`
	WebSocket WebSocketConfig `yaml:"websocket"`
}

// HTTPConfig contains HTTP server settings.
type HTTPConfig struct {
	Host         string        `yaml:"host"`
	Port         int           `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

// WebSocketConfig contains WebSocket settings.
type WebSocketConfig struct {
	ReadBufferSize   int           `yaml:"read_buffer_size"`
	WriteBufferSize  int           `yaml:"write_buffer_size"`
	HandshakeTimeout time.Duration `yaml:"handshake_timeout"`
	PingInterval     time.Duration `yaml:"ping_interval"`
}

// WebRTCConfig contains WebRTC related settings.
type WebRTCConfig struct {
	ICELite   bool              `yaml:"ice_lite"`
	ICEServer []ICEServerConfig `yaml:"ice_servers"`
	PortRange PortRangeConfig   `yaml:"port_range"`
	PublicIP  string            `yaml:"public_ip"`
}

// ICEServerConfig contains ICE server settings.
type ICEServerConfig struct {
	URLs       []string `yaml:"urls"`
	Username   string   `yaml:"username"`
	Credential string   `yaml:"credential"`
}

// PortRangeConfig contains UDP port range settings.
type PortRangeConfig struct {
	Min uint16 `yaml:"min"`
	Max uint16 `yaml:"max"`
}

// RoomConfig contains room settings.
type RoomConfig struct {
	MaxParticipants         int                     `yaml:"max_participants"`
	EmptyTimeout            time.Duration           `yaml:"empty_timeout"`
	MaxTracksPerParticipant MaxTracksPerParticipant `yaml:"max_tracks_per_participant"`
	MaxTracksPerRoom        int                     `yaml:"max_tracks_per_room"`
}

// MaxTracksPerParticipant contains per-participant track limits.
type MaxTracksPerParticipant struct {
	Video int `yaml:"video"`
	Audio int `yaml:"audio"`
}

// MediaConfig contains media processing settings.
type MediaConfig struct {
	Simulcast SimulcastConfig `yaml:"simulcast"`
	Codecs    CodecsConfig    `yaml:"codecs"`
}

// SimulcastConfig contains Simulcast settings.
type SimulcastConfig struct {
	Enabled bool                `yaml:"enabled"`
	Layers  []SimulcastLayerDef `yaml:"layers"`
}

// SimulcastLayerDef defines a Simulcast layer.
type SimulcastLayerDef struct {
	RID        string `yaml:"rid"`
	MaxBitrate int    `yaml:"max_bitrate"`
	MaxFPS     int    `yaml:"max_fps"`
}

// CodecsConfig contains codec settings.
type CodecsConfig struct {
	Video []VideoCodecConfig `yaml:"video"`
	Audio []AudioCodecConfig `yaml:"audio"`
}

// VideoCodecConfig contains video codec settings.
type VideoCodecConfig struct {
	Name     string           `yaml:"name"`
	Priority int              `yaml:"priority"`
	Profiles []H264ProfileDef `yaml:"profiles,omitempty"`
}

// H264ProfileDef defines H.264 profile parameters.
type H264ProfileDef struct {
	ProfileLevelID        string `yaml:"profile_level_id"`
	PacketizationMode     int    `yaml:"packetization_mode"`
	LevelAsymmetryAllowed int    `yaml:"level_asymmetry_allowed"`
}

// AudioCodecConfig contains audio codec settings.
type AudioCodecConfig struct {
	Name      string         `yaml:"name"`
	Priority  int            `yaml:"priority"`
	Channels  int            `yaml:"channels"`
	ClockRate int            `yaml:"clock_rate"`
	FMTP      map[string]int `yaml:"fmtp,omitempty"`
}

// AuthConfig contains authentication settings.
type AuthConfig struct {
	JWT       JWTConfig       `yaml:"jwt"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
}

// JWTConfig contains JWT settings.
type JWTConfig struct {
	Algorithm string        `yaml:"algorithm"`
	JWKSURL   string        `yaml:"jwks_url"`
	Issuer    string        `yaml:"issuer"`
	Audience  string        `yaml:"audience"`
	CacheTTL  time.Duration `yaml:"cache_ttl"`
}

// RateLimitConfig contains rate limiting settings.
type RateLimitConfig struct {
	WebSocketConnections int `yaml:"websocket_connections"`
	SignalingMessages    int `yaml:"signaling_messages"`
	APIRequests          int `yaml:"api_requests"`
}

// RecordingConfig contains recording settings.
type RecordingConfig struct {
	Enabled         bool          `yaml:"enabled"`
	Format          string        `yaml:"format"`
	Storage         StorageConfig `yaml:"storage"`
	TempDir         string        `yaml:"temp_dir"`
	SegmentDuration time.Duration `yaml:"segment_duration"`
	MaxSegmentSize  string        `yaml:"max_segment_size"`
	RetentionDays   int           `yaml:"retention_days"`
}

// StorageConfig contains storage settings.
type StorageConfig struct {
	Type      string `yaml:"type"`
	Bucket    string `yaml:"bucket"`
	ProjectID string `yaml:"project_id"`
}

// StoreConfig contains session store settings.
type StoreConfig struct {
	Type  string      `yaml:"type"`
	Redis RedisConfig `yaml:"redis"`
}

// RedisConfig contains Redis connection settings.
type RedisConfig struct {
	Address  string `yaml:"address"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	PoolSize int    `yaml:"pool_size"`
}

// LoggingConfig contains logging settings.
type LoggingConfig struct {
	Level      string `yaml:"level"`
	Format     string `yaml:"format"`
	Output     string `yaml:"output"`
	PIIMasking bool   `yaml:"pii_masking"`
}

// MetricsConfig contains metrics settings.
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			HTTP: HTTPConfig{
				Host:         "0.0.0.0",
				Port:         8080,
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 30 * time.Second,
			},
			WebSocket: WebSocketConfig{
				ReadBufferSize:   1024,
				WriteBufferSize:  1024,
				HandshakeTimeout: 10 * time.Second,
				PingInterval:     30 * time.Second,
			},
		},
		WebRTC: WebRTCConfig{
			ICELite: true,
			PortRange: PortRangeConfig{
				Min: 10000,
				Max: 20000,
			},
		},
		Room: RoomConfig{
			MaxParticipants: 100,
			EmptyTimeout:    5 * time.Minute,
			MaxTracksPerParticipant: MaxTracksPerParticipant{
				Video: 3,
				Audio: 2,
			},
			MaxTracksPerRoom: 500,
		},
		Media: MediaConfig{
			Simulcast: SimulcastConfig{
				Enabled: true,
				Layers: []SimulcastLayerDef{
					{RID: "h", MaxBitrate: 2500000, MaxFPS: 30},
					{RID: "m", MaxBitrate: 500000, MaxFPS: 30},
					{RID: "l", MaxBitrate: 150000, MaxFPS: 15},
				},
			},
			Codecs: CodecsConfig{
				Video: []VideoCodecConfig{
					{Name: "VP8", Priority: 1},
					{
						Name:     "H264",
						Priority: 2,
						Profiles: []H264ProfileDef{
							{ProfileLevelID: "640032", PacketizationMode: 1, LevelAsymmetryAllowed: 1},
							{ProfileLevelID: "42e01f", PacketizationMode: 1, LevelAsymmetryAllowed: 1},
						},
					},
					{Name: "VP9", Priority: 3},
				},
				Audio: []AudioCodecConfig{
					{
						Name:      "opus",
						Priority:  1,
						Channels:  2,
						ClockRate: 48000,
						FMTP: map[string]int{
							"minptime":     10,
							"useinbandfec": 1,
							"stereo":       1,
						},
					},
				},
			},
		},
		Auth: AuthConfig{
			JWT: JWTConfig{
				Algorithm: "RS256",
				CacheTTL:  1 * time.Hour,
			},
			RateLimit: RateLimitConfig{
				WebSocketConnections: 10,
				SignalingMessages:    100,
				APIRequests:          100,
			},
		},
		Recording: RecordingConfig{
			Enabled:         false,
			Format:          "webm",
			TempDir:         "/tmp/recordings",
			SegmentDuration: 1 * time.Hour,
			MaxSegmentSize:  "1GB",
			RetentionDays:   30,
		},
		Store: StoreConfig{
			Type: "memory",
			Redis: RedisConfig{
				Address:  "localhost:6379",
				DB:       0,
				PoolSize: 10,
			},
		},
		Logging: LoggingConfig{
			Level:      "info",
			Format:     "json",
			Output:     "stdout",
			PIIMasking: true,
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Path:    "/metrics",
		},
	}
}

// Load loads configuration from a YAML file.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	cfg.applyEnvOverrides()

	return cfg, nil
}

// applyEnvOverrides applies environment variable overrides to the configuration.
func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("SFU_HTTP_HOST"); v != "" {
		c.Server.HTTP.Host = v
	}
	if v := os.Getenv("SFU_HTTP_PORT"); v != "" {
		var port int
		if _, err := fmt.Sscanf(v, "%d", &port); err == nil {
			c.Server.HTTP.Port = port
		}
	}
	if v := os.Getenv("SFU_PUBLIC_IP"); v != "" {
		c.WebRTC.PublicIP = v
	}
	if v := os.Getenv("SFU_LOG_LEVEL"); v != "" {
		c.Logging.Level = strings.ToLower(v)
	}
	if v := os.Getenv("SFU_REDIS_ADDRESS"); v != "" {
		c.Store.Redis.Address = v
	}
	if v := os.Getenv("SFU_REDIS_PASSWORD"); v != "" {
		c.Store.Redis.Password = v
	}
	if v := os.Getenv("SFU_JWT_JWKS_URL"); v != "" {
		c.Auth.JWT.JWKSURL = v
	}
	if v := os.Getenv("SFU_JWT_ISSUER"); v != "" {
		c.Auth.JWT.Issuer = v
	}
	if v := os.Getenv("SFU_JWT_AUDIENCE"); v != "" {
		c.Auth.JWT.Audience = v
	}
	if v := os.Getenv("SFU_STORE_TYPE"); v != "" {
		c.Store.Type = strings.ToLower(v)
	}
}
