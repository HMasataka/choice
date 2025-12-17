package sfu

import (
	"math/rand"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/pion/turn/v2"
)

var (
	packetFactory *sync.Pool
)

func init() {
	packetFactory = &sync.Pool{
		New: func() any {
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
		transportConfig:       webrtcConfig,
		sessions:              make(map[string]Session),
	}

	if c.Turn.Enabled {
		ts, err := InitTurnServer(c.Turn)
		if err != nil {
			os.Exit(1)
		}
		sfu.turn = ts
	}

	runtime.KeepAlive(ballast)
	return sfu
}

type SFU struct {
	mu       sync.RWMutex
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
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]

	return session, ok
}

func (s *SFU) newSession(id string) Session {
	session := NewSession(id, s.datachannels, s.webrtcTransportConfig)

	session.OnClose(func() {
		s.mu.Lock()
		delete(s.sessions, id)
		s.mu.Unlock()
	})

	s.mu.Lock()
	s.sessions[id] = session
	s.mu.Unlock()

	return session
}

func (s *SFU) GetTransportConfig() WebRTCTransportConfig {
	return s.transportConfig
}
