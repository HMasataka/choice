package sfu

import (
	"net"
	"time"

	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
)

type Config struct {
	SFU           SFUConfig    `toml:"sfu"`
	WebRTC        WebRTCConfig `toml:"webrtc"`
	RouterConfig  RouterConfig `toml:"Router"`
	Turn          TurnConfig   `toml:"turn"`
	BufferFactory *buffer.Factory
}

type SFUConfig struct {
	Ballast   int64 `toml:"ballast"`
	WithStats bool  `toml:"withstats"`
}

type WebRTCConfig struct {
	ICESinglePort int                  `toml:"singleport"`
	ICEPortRange  []uint16             `toml:"portrange"`
	ICEServers    []ICEServerConfig    `toml:"iceserver"`
	Candidates    Candidates           `toml:"candidates"`
	SDPSemantics  string               `toml:"sdpsemantics"`
	MDNS          bool                 `toml:"mdns"`
	Timeouts      WebRTCTimeoutsConfig `toml:"timeouts"`
}

type ICEServerConfig struct {
	URLs       []string `toml:"urls"`
	Username   string   `toml:"username"`
	Credential string   `toml:"credential"`
}

type Candidates struct {
	IceLite    bool     `toml:"icelite"`
	NAT1To1IPs []string `toml:"nat1to1"`
}

type WebRTCTimeoutsConfig struct {
	ICEDisconnectedTimeout int `toml:"disconnected"`
	ICEFailedTimeout       int `toml:"failed"`
	ICEKeepaliveInterval   int `toml:"keepalive"`
}

type WebRTCTransportConfig struct {
	Configuration webrtc.Configuration
	Setting       webrtc.SettingEngine
	RouterConfig  RouterConfig
	BufferFactory *buffer.Factory
}

func NewWebRTCTransportConfig(c Config) WebRTCTransportConfig {
	se := webrtc.SettingEngine{}
	se.DisableMediaEngineCopy(true)

	if c.WebRTC.ICESinglePort != 0 {
		udpListener, err := net.ListenUDP("udp", &net.UDPAddr{
			IP:   net.IP{0, 0, 0, 0},
			Port: c.WebRTC.ICESinglePort,
		})
		if err != nil {
			panic(err)
		}
		se.SetICEUDPMux(webrtc.NewICEUDPMux(nil, udpListener))
	} else {
		var icePortStart, icePortEnd uint16

		if c.Turn.Enabled && len(c.Turn.PortRange) == 0 {
			icePortStart = sfuMinPort
			icePortEnd = sfuMaxPort
		} else if len(c.WebRTC.ICEPortRange) == 2 {
			icePortStart = c.WebRTC.ICEPortRange[0]
			icePortEnd = c.WebRTC.ICEPortRange[1]
		}
		if icePortStart != 0 || icePortEnd != 0 {
			if err := se.SetEphemeralUDPPortRange(icePortStart, icePortEnd); err != nil {
				panic(err)
			}
		}
	}

	var iceServers []webrtc.ICEServer
	if c.WebRTC.Candidates.IceLite {
		se.SetLite(c.WebRTC.Candidates.IceLite)
	} else {
		for _, iceServer := range c.WebRTC.ICEServers {
			s := webrtc.ICEServer{
				URLs:       iceServer.URLs,
				Username:   iceServer.Username,
				Credential: iceServer.Credential,
			}
			iceServers = append(iceServers, s)
		}
	}

	se.BufferFactory = c.BufferFactory.GetOrNew

	sdpSemantics := webrtc.SDPSemanticsUnifiedPlan
	switch c.WebRTC.SDPSemantics {
	case "unified-plan-with-fallback":
		sdpSemantics = webrtc.SDPSemanticsUnifiedPlanWithFallback
	case "plan-b":
		sdpSemantics = webrtc.SDPSemanticsPlanB
	}

	if c.WebRTC.Timeouts.ICEDisconnectedTimeout == 0 &&
		c.WebRTC.Timeouts.ICEFailedTimeout == 0 &&
		c.WebRTC.Timeouts.ICEKeepaliveInterval == 0 {
	} else {
		se.SetICETimeouts(
			time.Duration(c.WebRTC.Timeouts.ICEDisconnectedTimeout)*time.Second,
			time.Duration(c.WebRTC.Timeouts.ICEFailedTimeout)*time.Second,
			time.Duration(c.WebRTC.Timeouts.ICEKeepaliveInterval)*time.Second,
		)
	}

	w := WebRTCTransportConfig{
		Configuration: webrtc.Configuration{
			ICEServers:   iceServers,
			SDPSemantics: sdpSemantics,
		},
		Setting:       se,
		RouterConfig:  c.RouterConfig,
		BufferFactory: c.BufferFactory,
	}

	if len(c.WebRTC.Candidates.NAT1To1IPs) > 0 {
		w.Setting.SetNAT1To1IPs(c.WebRTC.Candidates.NAT1To1IPs, webrtc.ICECandidateTypeHost)
	}

	if !c.WebRTC.MDNS {
		w.Setting.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	}

	return w
}
