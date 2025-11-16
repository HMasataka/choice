package sfu

import (
	"math/rand"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/pion/ice/v4"
	"github.com/pion/turn/v2"
	"github.com/pion/webrtc/v4"
)

var (
	packetFactory *sync.Pool
)

func init() {
	packetFactory = &sync.Pool{
		New: func() interface{} {
			b := make([]byte, 1460)
			return &b
		},
	}
}

type SessionProvider interface {
	GetTransportConfig() WebRTCTransportConfig
	GetSession(id string) Session
}

var _ SessionProvider = (*SFU)(nil)

func NewSFU(c Config) *SFU {
	rand.Seed(time.Now().UnixNano())
	ballast := make([]byte, c.SFU.Ballast*1024*1024)

	if c.BufferFactory == nil {
		c.BufferFactory = buffer.NewBufferFactory(c.RouterConfig.MaxPacketTrack)
	}

	webrtcConfig := NewWebRTCTransportConfig(c)

	sfu := &SFU{
		webrtcTransportConfig: webrtcConfig,
		sessions:              make(map[string]Session),
	}

	if c.Turn.Enabled {
		ts, err := InitTurnServer(c.Turn, c.TurnAuth)
		if err != nil {
			os.Exit(1)
		}
		sfu.turn = ts
	}

	runtime.KeepAlive(ballast)
	return sfu
}

type SFU struct {
	lock     sync.RWMutex
	sessions map[string]Session

	webrtcTransportConfig WebRTCTransportConfig

	transportConfig WebRTCTransportConfig
	turn            *turn.Server
	datachannels    []*Datachannel
}

func (s *SFU) GetSession(id string) Session {
	session, ok := s.getSession(id)
	if ok {
		return session
	}

	return s.newSession(id)
}

func (s *SFU) getSession(id string) (Session, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	session, ok := s.sessions[id]

	return session, ok
}

func (s *SFU) newSession(id string) Session {
	session := NewSession(id, s.datachannels, s.webrtcTransportConfig)

	session.OnClose(func() {
		s.lock.Lock()
		delete(s.sessions, id)
		s.lock.Unlock()
	})

	s.lock.Lock()
	s.sessions[id] = session
	s.lock.Unlock()

	return session
}

func (s *SFU) GetTransportConfig() WebRTCTransportConfig {
	return s.transportConfig
}

type Config struct {
	SFU struct {
		Ballast   int64 `mapstructure:"ballast"`
		WithStats bool  `mapstructure:"withstats"`
	} `mapstructure:"sfu"`
	WebRTC        WebRTCConfig `mapstructure:"webrtc"`
	RouterConfig  RouterConfig `mapstructure:"Router"`
	Turn          TurnConfig   `mapstructure:"turn"`
	BufferFactory *buffer.Factory
	TurnAuth      func(username string, realm string, srcAddr net.Addr) ([]byte, bool)
}

type ICEServerConfig struct {
	URLs       []string `mapstructure:"urls"`
	Username   string   `mapstructure:"username"`
	Credential string   `mapstructure:"credential"`
}

type Candidates struct {
	IceLite    bool     `mapstructure:"icelite"`
	NAT1To1IPs []string `mapstructure:"nat1to1"`
}

type WebRTCTimeoutsConfig struct {
	ICEDisconnectedTimeout int `mapstructure:"disconnected"`
	ICEFailedTimeout       int `mapstructure:"failed"`
	ICEKeepaliveInterval   int `mapstructure:"keepalive"`
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

type WebRTCConfig struct {
	ICESinglePort int                  `mapstructure:"singleport"`
	ICEPortRange  []uint16             `mapstructure:"portrange"`
	ICEServers    []ICEServerConfig    `mapstructure:"iceserver"`
	Candidates    Candidates           `mapstructure:"candidates"`
	SDPSemantics  string               `mapstructure:"sdpsemantics"`
	MDNS          bool                 `mapstructure:"mdns"`
	Timeouts      WebRTCTimeoutsConfig `mapstructure:"timeouts"`
}
