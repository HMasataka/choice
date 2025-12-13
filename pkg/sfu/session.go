package sfu

import (
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HMasataka/choice/pkg/relay"
	"github.com/pion/webrtc/v4"
	"github.com/samber/lo"
)

const (
	AudioLevelsMethod = "audioLevels"
)

/*
Sessionはsfu内でメディアを共有するための抽象化されたインターフェースです。
Sessionは複数のPeerを保持し、Peer間でメディアを交換します。
*/
type Session interface {
	ID() string
	Publish(router Router, r Receiver)
	Subscribe(peer Peer)
	AddPeer(peer Peer)
	GetPeer(peerID string) Peer
	RemovePeer(peer Peer)
	AddRelayPeer(peerID string, signalData []byte) ([]byte, error)
	AudioObserver() *AudioObserver
	AddDatachannel(owner string, dc *webrtc.DataChannel)
	GetDCMiddlewares() []*Datachannel
	GetFanOutDataChannelLabels() []string
	GetDataChannels(peerID, label string) (dcs []*webrtc.DataChannel)
	FanOutMessage(origin, label string, msg webrtc.DataChannelMessage)
	Peers() []Peer
	RelayPeers() []*RelayPeer
	OnClose(f func())
}

var _ Session = (*sessionLocal)(nil)

func NewSession(id string, dcs []*Datachannel, cfg WebRTCTransportConfig) Session {
	s := &sessionLocal{
		id:           id,
		peers:        make(map[string]Peer),
		relayPeers:   make(map[string]*RelayPeer),
		datachannels: dcs,
		config:       cfg,
		audioObs:     NewAudioObserver(cfg.RouterConfig.AudioLevelThreshold, cfg.RouterConfig.AudioLevelInterval, cfg.RouterConfig.AudioLevelFilter),
	}

	go s.audioLevelObserver(cfg.RouterConfig.AudioLevelInterval)

	return s
}

type sessionLocal struct {
	id             string
	mu             sync.RWMutex
	config         WebRTCTransportConfig
	peers          map[string]Peer
	relayPeers     map[string]*RelayPeer
	closed         atomic.Bool
	audioObs       *AudioObserver
	fanOutDCs      []string
	datachannels   []*Datachannel
	onCloseHandler func()
}

func (s *sessionLocal) ID() string {
	return s.id
}

func (s *sessionLocal) AudioObserver() *AudioObserver {
	return s.audioObs
}

func (s *sessionLocal) GetDCMiddlewares() []*Datachannel {
	return s.datachannels
}

func (s *sessionLocal) GetFanOutDataChannelLabels() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fanout := make([]string, len(s.fanOutDCs))
	copy(fanout, s.fanOutDCs)
	return fanout
}

func (s *sessionLocal) AddPeer(peer Peer) {
	s.mu.Lock()
	s.peers[peer.UserID()] = peer
	s.mu.Unlock()
}

func (s *sessionLocal) GetPeer(peerID string) Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.peers[peerID]
}

func (s *sessionLocal) AddRelayPeer(peerID string, signalData []byte) ([]byte, error) {
	p, err := relay.NewPeer(relay.PeerMeta{
		PeerID:    peerID,
		SessionID: s.id,
	}, &relay.PeerConfig{
		SettingEngine: s.config.Setting,
		ICEServers:    s.config.Configuration.ICEServers,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.Answer(signalData)
	if err != nil {
		return nil, err
	}

	p.OnReady(func() {
		rp := NewRelayPeer(p, s, &s.config)
		s.mu.Lock()
		s.relayPeers[peerID] = rp
		s.mu.Unlock()
	})

	p.OnClose(func() {
		s.mu.Lock()
		delete(s.relayPeers, peerID)
		s.mu.Unlock()
	})

	return resp, nil
}

func (s *sessionLocal) GetRelayPeer(peerID string) *RelayPeer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.relayPeers[peerID]
}

// RemovePeer removes Peer from the SessionLocal
func (s *sessionLocal) RemovePeer(p Peer) {
	pid := p.UserID()
	s.mu.Lock()
	if s.peers[pid] == p {
		delete(s.peers, pid)
	}
	peerCount := len(s.peers) + len(s.relayPeers)
	s.mu.Unlock()

	// Close SessionLocal if no peers
	if peerCount == 0 {
		s.Close()
	}
}

func (s *sessionLocal) AddDatachannel(owner string, dc *webrtc.DataChannel) {
	l := dc.Label()

	s.mu.Lock()
	label, found := lo.Find(s.fanOutDCs, func(lbl string) bool {
		return l == lbl
	})
	if found {
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			s.FanOutMessage(owner, label, msg)
		})
		s.mu.Unlock()
		return
	}

	s.fanOutDCs = append(s.fanOutDCs, label)
	peerOwner := s.peers[owner]
	s.mu.Unlock()
	peers := s.Peers()
	peerOwner.Subscriber().RegisterDatachannel(label, dc)

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		s.FanOutMessage(owner, label, msg)
	})

	for _, peer := range peers {
		if peer.UserID() == owner || peer.Subscriber() == nil {
			continue
		}

		ndc, err := peer.Subscriber().AddDataChannel(label)
		if err != nil {
			continue
		}

		if peer.Publisher() != nil && peer.Publisher().Relayed() {
			peer.Publisher().AddRelayFanOutDataChannel(label)
		}

		userID := peer.UserID()
		ndc.OnMessage(func(msg webrtc.DataChannelMessage) {
			s.FanOutMessage(userID, label, msg)

			if peer.Publisher().Relayed() {
				for _, rdc := range peer.Publisher().GetRelayedDataChannels(label) {
					if msg.IsString {
						if err = rdc.SendText(string(msg.Data)); err != nil {
							slog.Error("send relay text error", err)
						}
					} else {
						if err = rdc.Send(msg.Data); err != nil {
							slog.Error("send relay error", err)
						}
					}
				}
			}
		})

		peer.Subscriber().Negotiate()
	}
}

// Publish will add a Sender to all peers in current SessionLocal from given
// Receiver
func (s *sessionLocal) Publish(router Router, r Receiver) {
	for _, p := range s.Peers() {
		// Don't sub to self
		if router.UserID() == p.UserID() || p.Subscriber() == nil {
			continue
		}

		if err := router.AddDownTracks(p.Subscriber(), r); err != nil {
			continue
		}
	}
}

// Subscribe will create a Sender for every other Receiver in the SessionLocal
func (s *sessionLocal) Subscribe(peer Peer) {
	s.mu.RLock()
	fdc := make([]string, len(s.fanOutDCs))
	copy(fdc, s.fanOutDCs)
	peers := make([]Peer, 0, len(s.peers))
	for _, p := range s.peers {
		if p == peer || p.Publisher() == nil {
			continue
		}
		peers = append(peers, p)
	}
	s.mu.RUnlock()

	// Subscribe to fan out data channels
	for _, label := range fdc {
		dc, err := peer.Subscriber().AddDataChannel(label)
		if err != nil {
			continue
		}
		l := label
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			s.FanOutMessage(peer.UserID(), l, msg)

			if peer.Publisher().Relayed() {
				for _, rdc := range peer.Publisher().GetRelayedDataChannels(l) {
					if msg.IsString {
						if err = rdc.SendText(string(msg.Data)); err != nil {
							slog.Error("send relay text error", "error", err)
						}
					} else {
						if err = rdc.Send(msg.Data); err != nil {
							slog.Error("send relay error", "error", err)
						}
					}

				}
			}
		})
	}

	// Subscribe to publisher streams
	for _, p := range peers {
		err := p.Publisher().GetRouter().AddDownTracks(peer.Subscriber(), nil)
		if err != nil {
			slog.Error("subscribe to publisher stream error", "error", err)
			continue
		}
	}

	// Subscribe to relay streams
	for _, p := range s.RelayPeers() {
		err := p.GetRouter().AddDownTracks(peer.Subscriber(), nil)
		if err != nil {
			slog.Error("subscribe to relay stream error", "error", err)
			continue
		}
	}

	peer.Subscriber().Negotiate()
}

// Peers returns peers in this SessionLocal
func (s *sessionLocal) Peers() []Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p := make([]Peer, 0, len(s.peers))
	for _, peer := range s.peers {
		p = append(p, peer)
	}
	return p
}

// RelayPeers returns relay peers in this SessionLocal
func (s *sessionLocal) RelayPeers() []*RelayPeer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p := make([]*RelayPeer, 0, len(s.peers))
	for _, peer := range s.relayPeers {
		p = append(p, peer)
	}
	return p
}

// OnClose is called when the SessionLocal is closed
func (s *sessionLocal) OnClose(f func()) {
	s.onCloseHandler = f
}

func (s *sessionLocal) Close() {
	s.closed.Store(true)

	if s.onCloseHandler != nil {
		s.onCloseHandler()
	}
}

func (s *sessionLocal) FanOutMessage(origin, label string, msg webrtc.DataChannelMessage) {
	dcs := s.GetDataChannels(origin, label)
	for _, dc := range dcs {
		if msg.IsString {
			if err := dc.SendText(string(msg.Data)); err != nil {
				// TODO log
			}
		} else {
			if err := dc.Send(msg.Data); err != nil {
				// TODO log
			}
		}
	}
}

func (s *sessionLocal) GetDataChannels(peerID, label string) []*webrtc.DataChannel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dcs := make([]*webrtc.DataChannel, 0, len(s.peers))
	for pid, p := range s.peers {
		if peerID == pid {
			continue
		}

		if p.Subscriber() != nil {
			if dc := p.Subscriber().DataChannel(label); dc != nil && dc.ReadyState() == webrtc.DataChannelStateOpen {
				dcs = append(dcs, dc)
			}
		}

	}
	for _, rp := range s.relayPeers {
		if dc := rp.DataChannel(label); dc != nil {
			dcs = append(dcs, dc)
		}
	}

	return dcs
}

func (s *sessionLocal) audioLevelObserver(audioLevelInterval int) {
	if audioLevelInterval <= 50 {
		// TODO log
	}
	if audioLevelInterval == 0 {
		audioLevelInterval = 1000
	}
	for {
		time.Sleep(time.Duration(audioLevelInterval) * time.Millisecond)
		if s.closed.Load() {
			return
		}

		levels := s.audioObs.Calc()
		if levels == nil {
			continue
		}

		msg := ChannelAPIMessage{
			Method: AudioLevelsMethod,
			Params: levels,
		}

		l, err := json.Marshal(&msg)
		if err != nil {
			continue
		}

		sl := string(l)
		dcs := s.GetDataChannels("", APIChannelLabel)

		for _, ch := range dcs {
			if err = ch.SendText(sl); err != nil {
				// TODO log
			}
		}
	}
}
