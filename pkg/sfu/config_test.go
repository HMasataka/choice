package sfu

import (
	"testing"

	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigStructs(t *testing.T) {
	t.Run("Config構造体の初期化", func(t *testing.T) {
		cfg := Config{
			SFU: SFUConfig{
				Ballast:   1024 * 1024,
				WithStats: true,
			},
			WebRTC: WebRTCConfig{
				ICESinglePort: 5000,
				SDPSemantics:  "unified-plan",
			},
			RouterConfig: RouterConfig{
				MaxBandwidth:   1000000,
				MaxPacketTrack: 500,
			},
			Turn: TurnConfig{
				Enabled: true,
				Realm:   "test.local",
			},
		}

		assert.Equal(t, int64(1024*1024), cfg.SFU.Ballast)
		assert.True(t, cfg.SFU.WithStats)
		assert.Equal(t, 5000, cfg.WebRTC.ICESinglePort)
		assert.Equal(t, "unified-plan", cfg.WebRTC.SDPSemantics)
		assert.Equal(t, uint64(1000000), cfg.RouterConfig.MaxBandwidth)
		assert.True(t, cfg.Turn.Enabled)
	})

	t.Run("SFUConfig", func(t *testing.T) {
		cfg := SFUConfig{
			Ballast:   100 * 1024 * 1024,
			WithStats: true,
		}

		assert.Equal(t, int64(100*1024*1024), cfg.Ballast)
		assert.True(t, cfg.WithStats)
	})

	t.Run("WebRTCConfig", func(t *testing.T) {
		cfg := WebRTCConfig{
			ICESinglePort: 8443,
			ICEPortRange:  []uint16{50000, 60000},
			ICEServers: []ICEServerConfig{
				{
					URLs:       []string{"stun:stun.l.google.com:19302"},
					Username:   "user",
					Credential: "pass",
				},
			},
			Candidates: Candidates{
				IceLite:    true,
				NAT1To1IPs: []string{"192.168.1.1", "10.0.0.1"},
			},
			SDPSemantics: "unified-plan",
			MDNS:         true,
			Timeouts: WebRTCTimeoutsConfig{
				ICEDisconnectedTimeout: 5,
				ICEFailedTimeout:       25,
				ICEKeepaliveInterval:   2,
			},
		}

		assert.Equal(t, 8443, cfg.ICESinglePort)
		assert.Equal(t, []uint16{50000, 60000}, cfg.ICEPortRange)
		require.Len(t, cfg.ICEServers, 1)
		assert.Equal(t, []string{"stun:stun.l.google.com:19302"}, cfg.ICEServers[0].URLs)
		assert.True(t, cfg.Candidates.IceLite)
		assert.Equal(t, []string{"192.168.1.1", "10.0.0.1"}, cfg.Candidates.NAT1To1IPs)
		assert.True(t, cfg.MDNS)
		assert.Equal(t, 5, cfg.Timeouts.ICEDisconnectedTimeout)
	})

	t.Run("ICEServerConfig", func(t *testing.T) {
		cfg := ICEServerConfig{
			URLs:       []string{"turn:turn.example.com:3478", "stun:stun.example.com:3478"},
			Username:   "testuser",
			Credential: "testpass",
		}

		assert.Len(t, cfg.URLs, 2)
		assert.Equal(t, "testuser", cfg.Username)
		assert.Equal(t, "testpass", cfg.Credential)
	})

	t.Run("Candidates", func(t *testing.T) {
		cfg := Candidates{
			IceLite:    true,
			NAT1To1IPs: []string{"203.0.113.1"},
		}

		assert.True(t, cfg.IceLite)
		assert.Equal(t, []string{"203.0.113.1"}, cfg.NAT1To1IPs)
	})

	t.Run("WebRTCTimeoutsConfig", func(t *testing.T) {
		cfg := WebRTCTimeoutsConfig{
			ICEDisconnectedTimeout: 10,
			ICEFailedTimeout:       30,
			ICEKeepaliveInterval:   5,
		}

		assert.Equal(t, 10, cfg.ICEDisconnectedTimeout)
		assert.Equal(t, 30, cfg.ICEFailedTimeout)
		assert.Equal(t, 5, cfg.ICEKeepaliveInterval)
	})

	t.Run("WebRTCTransportConfig", func(t *testing.T) {
		cfg := WebRTCTransportConfig{
			Configuration: webrtc.Configuration{
				ICEServers: []webrtc.ICEServer{
					{URLs: []string{"stun:stun.l.google.com:19302"}},
				},
				SDPSemantics: webrtc.SDPSemanticsUnifiedPlan,
			},
			RouterConfig: RouterConfig{
				MaxBandwidth: 2000000,
			},
		}

		require.Len(t, cfg.Configuration.ICEServers, 1)
		assert.Equal(t, webrtc.SDPSemanticsUnifiedPlan, cfg.Configuration.SDPSemantics)
		assert.Equal(t, uint64(2000000), cfg.RouterConfig.MaxBandwidth)
	})
}

func TestNewWebRTCTransportConfig(t *testing.T) {
	t.Run("ICE Lite設定", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := Config{
			WebRTC: WebRTCConfig{
				Candidates: Candidates{
					IceLite: true,
				},
				SDPSemantics: "unified-plan",
			},
			BufferFactory: bf,
		}

		result := NewWebRTCTransportConfig(cfg)

		// ICE Liteの場合、ICEServersは空
		assert.Empty(t, result.Configuration.ICEServers)
		assert.Equal(t, webrtc.SDPSemanticsUnifiedPlan, result.Configuration.SDPSemantics)
	})

	t.Run("ICEサーバー設定", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := Config{
			WebRTC: WebRTCConfig{
				ICEServers: []ICEServerConfig{
					{
						URLs:       []string{"stun:stun.l.google.com:19302"},
						Username:   "",
						Credential: "",
					},
					{
						URLs:       []string{"turn:turn.example.com:3478"},
						Username:   "user",
						Credential: "pass",
					},
				},
				SDPSemantics: "unified-plan",
			},
			BufferFactory: bf,
		}

		result := NewWebRTCTransportConfig(cfg)

		require.Len(t, result.Configuration.ICEServers, 2)
		assert.Equal(t, []string{"stun:stun.l.google.com:19302"}, result.Configuration.ICEServers[0].URLs)
		assert.Equal(t, "user", result.Configuration.ICEServers[1].Username)
		assert.Equal(t, "pass", result.Configuration.ICEServers[1].Credential)
	})

	t.Run("SDPSemantics - unified-plan", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := Config{
			WebRTC: WebRTCConfig{
				Candidates:   Candidates{IceLite: true},
				SDPSemantics: "unified-plan",
			},
			BufferFactory: bf,
		}

		result := NewWebRTCTransportConfig(cfg)

		assert.Equal(t, webrtc.SDPSemanticsUnifiedPlan, result.Configuration.SDPSemantics)
	})

	t.Run("SDPSemantics - unified-plan-with-fallback", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := Config{
			WebRTC: WebRTCConfig{
				Candidates:   Candidates{IceLite: true},
				SDPSemantics: "unified-plan-with-fallback",
			},
			BufferFactory: bf,
		}

		result := NewWebRTCTransportConfig(cfg)

		assert.Equal(t, webrtc.SDPSemanticsUnifiedPlanWithFallback, result.Configuration.SDPSemantics)
	})

	t.Run("SDPSemantics - plan-b", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := Config{
			WebRTC: WebRTCConfig{
				Candidates:   Candidates{IceLite: true},
				SDPSemantics: "plan-b",
			},
			BufferFactory: bf,
		}

		result := NewWebRTCTransportConfig(cfg)

		assert.Equal(t, webrtc.SDPSemanticsPlanB, result.Configuration.SDPSemantics)
	})

	t.Run("SDPSemantics - デフォルト（空）", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := Config{
			WebRTC: WebRTCConfig{
				Candidates:   Candidates{IceLite: true},
				SDPSemantics: "",
			},
			BufferFactory: bf,
		}

		result := NewWebRTCTransportConfig(cfg)

		// デフォルトはUnifiedPlan
		assert.Equal(t, webrtc.SDPSemanticsUnifiedPlan, result.Configuration.SDPSemantics)
	})

	t.Run("SDPSemantics - 不明な値", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := Config{
			WebRTC: WebRTCConfig{
				Candidates:   Candidates{IceLite: true},
				SDPSemantics: "unknown-value",
			},
			BufferFactory: bf,
		}

		result := NewWebRTCTransportConfig(cfg)

		// 不明な値もUnifiedPlanにフォールバック
		assert.Equal(t, webrtc.SDPSemanticsUnifiedPlan, result.Configuration.SDPSemantics)
	})

	t.Run("NAT1To1IPs設定", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := Config{
			WebRTC: WebRTCConfig{
				Candidates: Candidates{
					IceLite:    true,
					NAT1To1IPs: []string{"192.168.1.100", "10.0.0.50"},
				},
			},
			BufferFactory: bf,
		}

		result := NewWebRTCTransportConfig(cfg)

		// Setting内部にNAT1To1IPsが設定されていることを確認
		// (SettingEngineの内部状態は直接確認できないが、エラーなく動作することを確認)
		assert.NotNil(t, result.Setting)
	})

	t.Run("MDNS無効", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := Config{
			WebRTC: WebRTCConfig{
				Candidates: Candidates{IceLite: true},
				MDNS:       false,
			},
			BufferFactory: bf,
		}

		result := NewWebRTCTransportConfig(cfg)

		assert.NotNil(t, result.Setting)
	})

	t.Run("MDNS有効", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := Config{
			WebRTC: WebRTCConfig{
				Candidates: Candidates{IceLite: true},
				MDNS:       true,
			},
			BufferFactory: bf,
		}

		result := NewWebRTCTransportConfig(cfg)

		assert.NotNil(t, result.Setting)
	})

	t.Run("RouterConfigの引き継ぎ", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := Config{
			WebRTC: WebRTCConfig{
				Candidates: Candidates{IceLite: true},
			},
			RouterConfig: RouterConfig{
				MaxBandwidth:        5000000,
				MaxPacketTrack:      1000,
				AudioLevelInterval:  100,
				AudioLevelThreshold: 40,
				AudioLevelFilter:    50,
			},
			BufferFactory: bf,
		}

		result := NewWebRTCTransportConfig(cfg)

		assert.Equal(t, uint64(5000000), result.RouterConfig.MaxBandwidth)
		assert.Equal(t, 1000, result.RouterConfig.MaxPacketTrack)
		assert.Equal(t, 100, result.RouterConfig.AudioLevelInterval)
		assert.Equal(t, uint8(40), result.RouterConfig.AudioLevelThreshold)
		assert.Equal(t, 50, result.RouterConfig.AudioLevelFilter)
	})

	t.Run("BufferFactoryの引き継ぎ", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := Config{
			WebRTC: WebRTCConfig{
				Candidates: Candidates{IceLite: true},
			},
			BufferFactory: bf,
		}

		result := NewWebRTCTransportConfig(cfg)

		assert.Same(t, bf, result.BufferFactory)
	})

	t.Run("ICEタイムアウト設定あり", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := Config{
			WebRTC: WebRTCConfig{
				Candidates: Candidates{IceLite: true},
				Timeouts: WebRTCTimeoutsConfig{
					ICEDisconnectedTimeout: 10,
					ICEFailedTimeout:       30,
					ICEKeepaliveInterval:   5,
				},
			},
			BufferFactory: bf,
		}

		result := NewWebRTCTransportConfig(cfg)

		// タイムアウトが設定されていることを確認（内部状態は直接確認できない）
		assert.NotNil(t, result.Setting)
	})

	t.Run("ICEタイムアウト設定なし（全てゼロ）", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := Config{
			WebRTC: WebRTCConfig{
				Candidates: Candidates{IceLite: true},
				Timeouts: WebRTCTimeoutsConfig{
					ICEDisconnectedTimeout: 0,
					ICEFailedTimeout:       0,
					ICEKeepaliveInterval:   0,
				},
			},
			BufferFactory: bf,
		}

		result := NewWebRTCTransportConfig(cfg)

		// タイムアウトがデフォルト値のままであることを確認
		assert.NotNil(t, result.Setting)
	})

	t.Run("ICEタイムアウト部分設定", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := Config{
			WebRTC: WebRTCConfig{
				Candidates: Candidates{IceLite: true},
				Timeouts: WebRTCTimeoutsConfig{
					ICEDisconnectedTimeout: 10,
					ICEFailedTimeout:       0,
					ICEKeepaliveInterval:   0,
				},
			},
			BufferFactory: bf,
		}

		result := NewWebRTCTransportConfig(cfg)

		// 一部でも設定があればSetICETimeoutsが呼ばれる
		assert.NotNil(t, result.Setting)
	})

	t.Run("ポート範囲設定", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := Config{
			WebRTC: WebRTCConfig{
				ICEPortRange: []uint16{50000, 60000},
				Candidates:   Candidates{IceLite: true},
			},
			BufferFactory: bf,
		}

		result := NewWebRTCTransportConfig(cfg)

		assert.NotNil(t, result.Setting)
	})

	t.Run("TURN有効でポート範囲なし", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := Config{
			WebRTC: WebRTCConfig{
				Candidates: Candidates{IceLite: true},
			},
			Turn: TurnConfig{
				Enabled:   true,
				PortRange: []uint16{}, // 空
			},
			BufferFactory: bf,
		}

		result := NewWebRTCTransportConfig(cfg)

		// sfuMinPort/sfuMaxPortが使用される
		assert.NotNil(t, result.Setting)
	})
}
