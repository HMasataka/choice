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

// Sessionはsfu内でメディアを共有するための抽象化されたインターフェースです。
// Sessionは複数のPeerを保持し、Peer間でメディアを交換します。
//
//go:generate mockgen -source session.go -destination mock/session.go
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
	label := dc.Label()

	if s.registerExistingFanOut(owner, label, dc) {
		return
	}

	s.registerNewFanOut(owner, label, dc)
}

// registerExistingFanOut は既存のファンアウトチャネルにメッセージハンドラを登録する
func (s *sessionLocal) registerExistingFanOut(owner, label string, dc *webrtc.DataChannel) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, found := lo.Find(s.fanOutDCs, func(lbl string) bool {
		return label == lbl
	})
	if !found {
		return false
	}

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		s.FanOutMessage(owner, label, msg)
	})
	return true
}

// registerNewFanOut は新しいファンアウトチャネルを登録し、全ピアに配信する
func (s *sessionLocal) registerNewFanOut(owner, label string, dc *webrtc.DataChannel) {
	s.mu.Lock()
	s.fanOutDCs = append(s.fanOutDCs, label)
	peerOwner := s.peers[owner]
	s.mu.Unlock()

	peerOwner.Subscriber().RegisterDatachannel(label, dc)
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		s.FanOutMessage(owner, label, msg)
	})

	for _, peer := range s.Peers() {
		if peer.UserID() == owner || peer.Subscriber() == nil {
			continue
		}
		s.setupPeerDataChannel(peer, label)
	}
}

// setupPeerDataChannel は個別ピアにDataChannelを設定する
func (s *sessionLocal) setupPeerDataChannel(peer Peer, label string) {
	ndc, err := peer.Subscriber().AddDataChannel(label)
	if err != nil {
		return
	}

	if peer.Publisher() != nil && peer.Publisher().Relayed() {
		peer.Publisher().AddRelayFanOutDataChannel(label)
	}

	userID := peer.UserID()
	ndc.OnMessage(func(msg webrtc.DataChannelMessage) {
		s.FanOutMessage(userID, label, msg)
		s.relayMessageIfNeeded(peer, label, msg)
	})

	peer.Subscriber().Negotiate()
}

// relayMessageIfNeeded はリレーが有効な場合にメッセージを転送する
func (s *sessionLocal) relayMessageIfNeeded(peer Peer, label string, msg webrtc.DataChannelMessage) {
	if peer.Publisher() == nil || !peer.Publisher().Relayed() {
		return
	}
	for _, rdc := range peer.Publisher().GetRelayedDataChannels(label) {
		s.sendToDataChannel(rdc, msg)
	}
}

// Publish will add a Sender to all peers in current SessionLocal from given
// Receiver
func (s *sessionLocal) Publish(router Router, r Receiver) {
	for _, p := range s.Peers() {
		if p.Subscriber() == nil {
			continue
		}

		if router.UserID() == p.UserID() && !s.config.RouterConfig.AllowSelfSubscribe {
			continue
		}

		if err := router.AddDownTracks(p.Subscriber(), r); err != nil {
			continue
		}
	}
}

// Subscribe will create a Sender for every other Receiver in the SessionLocal
func (s *sessionLocal) Subscribe(peer Peer) {
	fdc, peers := s.getSubscriptionTargets(peer)
	s.subscribeToFanOutChannels(peer, fdc)
	s.subscribeToPublisherStreams(peer, peers)
	s.subscribeToRelayStreams(peer)
}

// getSubscriptionTargets は購読対象のチャネルとピアを取得する
func (s *sessionLocal) getSubscriptionTargets(peer Peer) ([]string, []Peer) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	fdc := make([]string, len(s.fanOutDCs))
	copy(fdc, s.fanOutDCs)

	peers := make([]Peer, 0, len(s.peers))
	for _, p := range s.peers {
		if p == peer || p.Publisher() == nil {
			continue
		}
		peers = append(peers, p)
	}
	return fdc, peers
}

// subscribeToFanOutChannels はファンアウトチャネルを購読する
func (s *sessionLocal) subscribeToFanOutChannels(peer Peer, labels []string) {
	for _, label := range labels {
		dc, err := peer.Subscriber().AddDataChannel(label)
		if err != nil {
			continue
		}
		s.setupFanOutMessageHandler(peer, dc, label)
	}
}

// setupFanOutMessageHandler はDataChannelにファンアウトメッセージハンドラを設定する
func (s *sessionLocal) setupFanOutMessageHandler(peer Peer, dc *webrtc.DataChannel, label string) {
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		s.FanOutMessage(peer.UserID(), label, msg)
		s.relayMessageIfNeeded(peer, label, msg)
	})
}

// subscribeToPublisherStreams はパブリッシャーストリームを購読する
func (s *sessionLocal) subscribeToPublisherStreams(peer Peer, peers []Peer) {
	for _, p := range peers {
		if err := p.Publisher().GetRouter().AddDownTracks(peer.Subscriber(), nil); err != nil {
			slog.Error("subscribe to publisher stream error", "error", err)
		}
	}
}

// subscribeToRelayStreams はリレーストリームを購読する
func (s *sessionLocal) subscribeToRelayStreams(peer Peer) {
	for _, p := range s.RelayPeers() {
		if err := p.GetRouter().AddDownTracks(peer.Subscriber(), nil); err != nil {
			slog.Error("subscribe to relay stream error", "error", err)
		}
	}
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
	for _, dc := range s.GetDataChannels(origin, label) {
		s.sendToDataChannel(dc, msg)
	}
}

// sendToDataChannel はDataChannelにメッセージを送信する
func (s *sessionLocal) sendToDataChannel(dc *webrtc.DataChannel, msg webrtc.DataChannelMessage) {
	var err error
	if msg.IsString {
		err = dc.SendText(string(msg.Data))
	} else {
		err = dc.Send(msg.Data)
	}
	if err != nil {
		slog.Error("datachannel send failed", "error", err, "label", dc.Label())
	}
}

func (s *sessionLocal) GetDataChannels(peerID, label string) []*webrtc.DataChannel {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dcs := make([]*webrtc.DataChannel, 0, len(s.peers)+len(s.relayPeers))
	s.collectPeerDataChannels(&dcs, peerID, label)
	s.collectRelayDataChannels(&dcs, label)
	return dcs
}

// collectPeerDataChannels はピアからDataChannelを収集する
func (s *sessionLocal) collectPeerDataChannels(dcs *[]*webrtc.DataChannel, excludePeerID, label string) {
	for pid, p := range s.peers {
		if pid == excludePeerID || p.Subscriber() == nil {
			continue
		}
		if dc := p.Subscriber().DataChannel(label); dc != nil && dc.ReadyState() == webrtc.DataChannelStateOpen {
			*dcs = append(*dcs, dc)
		}
	}
}

// collectRelayDataChannels はリレーピアからDataChannelを収集する
func (s *sessionLocal) collectRelayDataChannels(dcs *[]*webrtc.DataChannel, label string) {
	for _, rp := range s.relayPeers {
		if dc := rp.DataChannel(label); dc != nil {
			*dcs = append(*dcs, dc)
		}
	}
}

func (s *sessionLocal) audioLevelObserver(audioLevelInterval int) {
	interval := s.normalizeAudioLevelInterval(audioLevelInterval)

	for {
		time.Sleep(time.Duration(interval) * time.Millisecond)
		if s.closed.Load() {
			return
		}
		s.broadcastAudioLevels()
	}
}

// normalizeAudioLevelInterval はオーディオレベル間隔を正規化する
func (s *sessionLocal) normalizeAudioLevelInterval(interval int) int {
	if interval <= 50 {
		slog.Warn("audio level interval too low; clamping recommended minimum", "interval_ms", interval)
	}
	if interval == 0 {
		return 1000
	}
	return interval
}

// broadcastAudioLevels はオーディオレベルを全クライアントに配信する
func (s *sessionLocal) broadcastAudioLevels() {
	levels := s.audioObs.Calc()
	if levels == nil {
		return
	}

	msg, err := s.buildAudioLevelMessage(levels)
	if err != nil {
		return
	}

	for _, ch := range s.GetDataChannels("", APIChannelLabel) {
		if err := ch.SendText(msg); err != nil {
			slog.Error("failed to send audio levels", "error", err, "channel_label", ch.Label())
		}
	}
}

// buildAudioLevelMessage はオーディオレベルメッセージをJSON文字列として構築する
func (s *sessionLocal) buildAudioLevelMessage(levels []string) (string, error) {
	msg := ChannelAPIMessage{
		Method: AudioLevelsMethod,
		Params: levels,
	}

	data, err := json.Marshal(&msg)
	if err != nil {
		return "", err
	}

	return string(data), nil
}
