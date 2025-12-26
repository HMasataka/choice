package sfu

import (
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/HMasataka/choice/pkg/relay"
	"github.com/pion/rtcp"
	"github.com/pion/transport/v3/packetio"
	"github.com/pion/webrtc/v4"
)

// Publisherはclientがメディアを送信するための抽象化された構造体です。
// ClientとPublisherは1対1の関係にあり、ClientはPublisherを使用してメディアストリームをsfuに送信します。
type Publisher interface {
	Answer(offer webrtc.SessionDescription) (webrtc.SessionDescription, error)
	GetRouter() Router
	Close()
	OnPublisherTrack(f func(track PublisherTrack))
	OnICECandidate(f func(c *webrtc.ICECandidate))
	OnICEConnectionStateChange(f func(connectionState webrtc.ICEConnectionState))
	SignalingState() webrtc.SignalingState
	PeerConnection() *webrtc.PeerConnection
	Relay(signalFn func(meta relay.PeerMeta, signal []byte) ([]byte, error), options ...func(r *relayPeer)) (*relay.Peer, error)
	PublisherTracks() []PublisherTrack
	AddRelayFanOutDataChannel(label string)
	GetRelayedDataChannels(label string) []*webrtc.DataChannel
	Relayed() bool
	Tracks() []*webrtc.TrackRemote
	AddICECandidate(candidate webrtc.ICECandidateInit) error
}

var _ Publisher = (*publisher)(nil)

type publisher struct {
	mu sync.RWMutex

	userID string

	pc  *webrtc.PeerConnection
	cfg *WebRTCTransportConfig

	router     Router
	session    Session
	tracks     []PublisherTrack
	relayed    atomic.Bool
	relayPeers []*relayPeer
	candidates []webrtc.ICECandidateInit

	onICEConnectionStateChangeHandler atomic.Value // func(webrtc.ICEConnectionState)
	onPublisherTrack                  atomic.Value // func(PublisherTrack)

	closeOnce sync.Once
}

func NewPublisher(userID string, session Session, cfg *WebRTCTransportConfig) (*publisher, error) {
	mediaEngine, err := getPublisherMediaEngine()
	if err != nil {
		return nil, err
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine), webrtc.WithSettingEngine(cfg.Setting))
	pc, err := api.NewPeerConnection(cfg.Configuration)
	if err != nil {
		return nil, err
	}

	router := NewRouter(userID, session, cfg)
	// サブスクライバからの RTCP フィードバックをこの Publisher の PeerConnection に転送する
	router.SetRTCPWriter(pc.WriteRTCP)

	p := &publisher{
		userID:  userID,
		pc:      pc,
		router:  router,
		session: session,
		cfg:     cfg,
	}

	// クライアントから上りのメディアトラックが届いた際、Router に登録して現在のサブスクライバへ配信する。
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		if track == nil || receiver == nil {
			return
		}
		recv, pub := router.AddReceiver(receiver, track, track.ID(), track.StreamID())
		if pub {
			recv.SetTrackMeta(track.ID(), track.StreamID())
			session.Publish(router, recv)
		}
		// トラックの管理と任意のコールバック
		p.mu.Lock()
		p.tracks = append(p.tracks, PublisherTrack{Track: track, Receiver: recv})
		p.mu.Unlock()
		if f, ok := p.onPublisherTrack.Load().(func(PublisherTrack)); ok && f != nil {
			f(PublisherTrack{Track: track, Receiver: recv})
		}
	})

	return p, nil
}

type relayPeer struct {
	peer                    *relay.Peer
	dcs                     []*webrtc.DataChannel
	withSRReports           bool
	relayFanOutDataChannels bool
}

type PublisherTrack struct {
	Track    *webrtc.TrackRemote
	Receiver Receiver
	// 将来的に、クライアントまたはサーバとしてリレーされるトラックで使用予定
	// これは SVC や Simulcast 向けで、リレーピアが単一トラック（録画・処理用）のみを
	// 受け取るか、全トラック（負荷分散用）を受け取るかを選択できるようにするためのもの
	clientRelay bool
}

func (p *publisher) Answer(offer webrtc.SessionDescription) (webrtc.SessionDescription, error) {
	if err := p.pc.SetRemoteDescription(offer); err != nil {
		return webrtc.SessionDescription{}, err
	}

	for _, c := range p.candidates {
		if err := p.pc.AddICECandidate(c); err != nil {
			slog.Error("publisher add ice candidate error", "error", err)
			continue
		}
	}
	p.candidates = nil

	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}

	if err := p.pc.SetLocalDescription(answer); err != nil {
		return webrtc.SessionDescription{}, err
	}

	return answer, nil
}

func (p *publisher) GetRouter() Router {
	return p.router
}

func (p *publisher) Close() {
	p.closeOnce.Do(func() {
		if len(p.relayPeers) > 0 {
			p.mu.Lock()
			for _, rp := range p.relayPeers {
				if err := rp.peer.Close(); err != nil {
					slog.Error("publisher relay peer close error", "error", err)
				}
			}
			p.mu.Unlock()
		}
		p.router.Stop()
		if err := p.pc.Close(); err != nil {
			slog.Error("publisher peer connection close error", "error", err)
		}
	})
}

func (p *publisher) OnPublisherTrack(f func(track PublisherTrack)) {
	p.onPublisherTrack.Store(f)
}

func (p *publisher) OnICECandidate(f func(c *webrtc.ICECandidate)) {
	p.pc.OnICECandidate(f)
}

func (p *publisher) OnICEConnectionStateChange(f func(connectionState webrtc.ICEConnectionState)) {
	p.onICEConnectionStateChangeHandler.Store(f)
}

func (p *publisher) SignalingState() webrtc.SignalingState {
	return p.pc.SignalingState()
}

func (p *publisher) PeerConnection() *webrtc.PeerConnection {
	return p.pc
}

func (p *publisher) Relay(signalFn func(meta relay.PeerMeta, signal []byte) ([]byte, error), options ...func(r *relayPeer)) (*relay.Peer, error) {
	lrp := p.applyRelayOptions(options)

	rp, err := p.createRelayPeer()
	if err != nil {
		return nil, err
	}
	lrp.peer = rp

	p.setupRelayCallbacks(rp, lrp)

	if err = rp.Offer(signalFn); err != nil {
		return nil, fmt.Errorf("relay: %w", err)
	}

	return rp, nil
}

// applyRelayOptions はリレーピアにオプションを適用する
func (p *publisher) applyRelayOptions(options []func(r *relayPeer)) *relayPeer {
	lrp := &relayPeer{}
	for _, o := range options {
		o(lrp)
	}
	return lrp
}

// createRelayPeer は新しいリレーピアを作成する
func (p *publisher) createRelayPeer() (*relay.Peer, error) {
	rp, err := relay.NewPeer(
		relay.PeerMeta{
			PeerID:    p.userID,
			SessionID: p.session.ID(),
		},
		&relay.PeerConfig{
			SettingEngine: p.cfg.Setting,
			ICEServers:    p.cfg.Configuration.ICEServers,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("relay: %w", err)
	}
	return rp, nil
}

// setupRelayCallbacks はリレーピアのコールバックを設定する
func (p *publisher) setupRelayCallbacks(rp *relay.Peer, lrp *relayPeer) {
	rp.OnReady(func() {
		p.handleRelayReady(rp, lrp)
	})

	rp.OnDataChannel(func(channel *webrtc.DataChannel) {
		p.handleRelayDataChannel(channel, lrp)
	})
}

// handleRelayReady はリレー接続準備完了時の処理を行う
func (p *publisher) handleRelayReady(rp *relay.Peer, lrp *relayPeer) {
	p.relayed.Store(true)

	if lrp.relayFanOutDataChannels {
		p.setupFanOutDataChannels(rp)
	}

	p.relayExistingTracks(rp, lrp)

	if lrp.withSRReports {
		go p.relayReports(rp)
	}
}

// setupFanOutDataChannels はファンアウト用DataChannelを設定する
func (p *publisher) setupFanOutDataChannels(rp *relay.Peer) {
	peer := p.session.GetPeer(p.userID)

	for _, lbl := range p.session.GetFanOutDataChannelLabels() {
		dc, err := rp.CreateDataChannel(lbl)
		if err != nil {
			continue
		}
		p.setupDataChannelMessageHandler(dc, lbl, peer)
	}
}

// setupDataChannelMessageHandler はDataChannelのメッセージハンドラを設定する
func (p *publisher) setupDataChannelMessageHandler(dc *webrtc.DataChannel, label string, peer Peer) {
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		p.forwardDataChannelMessage(label, peer, msg)
	})
}

// forwardDataChannelMessage はDataChannelメッセージをサブスクライバに転送する
func (p *publisher) forwardDataChannelMessage(label string, peer Peer, msg webrtc.DataChannelMessage) {
	if peer == nil || peer.Subscriber() == nil {
		return
	}

	sdc := peer.Subscriber().DataChannel(label)
	if sdc == nil {
		return
	}

	if msg.IsString {
		if err := sdc.SendText(string(msg.Data)); err != nil {
			slog.Error("subscriber send text error", "error", err)
		}
	} else {
		if err := sdc.Send(msg.Data); err != nil {
			slog.Error("subscriber send data error", "error", err)
		}
	}
}

// relayExistingTracks は既存のトラックをリレーに追加する
func (p *publisher) relayExistingTracks(rp *relay.Peer, lrp *relayPeer) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, tp := range p.tracks {
		if !tp.clientRelay {
			continue
		}

		if err := p.createRelayTrack(tp.Track, tp.Receiver, rp); err != nil {
			slog.Error("create relay track error", "error", err)
		}
	}
	p.relayPeers = append(p.relayPeers, lrp)
}

// handleRelayDataChannel はリレーからのDataChannel受信を処理する
func (p *publisher) handleRelayDataChannel(channel *webrtc.DataChannel, lrp *relayPeer) {
	if !lrp.relayFanOutDataChannels {
		return
	}

	p.mu.Lock()
	lrp.dcs = append(lrp.dcs, channel)
	p.mu.Unlock()

	p.session.AddDatachannel("", channel)
}

func (p *publisher) createRelayTrack(track *webrtc.TrackRemote, receiver Receiver, rp *relay.Peer) error {
	downTrack, err := p.newRelayDownTrack(track, receiver)
	if err != nil {
		return err
	}

	rtpSender, err := rp.AddTrack(receiver.(*WebRTCReceiver).receiver, track, downTrack)
	if err != nil {
		return fmt.Errorf("relay: %w", err)
	}

	p.setupRelayRTCPHandler(rtpSender, track)
	p.setupRelayCloseHandler(downTrack, rtpSender)

	receiver.AddDownTrack(downTrack, true)
	return nil
}

// newRelayDownTrack はリレー用のDownTrackを作成する
func (p *publisher) newRelayDownTrack(track *webrtc.TrackRemote, receiver Receiver) (*downTrack, error) {
	codec := track.Codec()
	capability := webrtc.RTPCodecCapability{
		MimeType:    codec.MimeType,
		ClockRate:   codec.ClockRate,
		Channels:    codec.Channels,
		SDPFmtpLine: codec.SDPFmtpLine,
		RTCPFeedback: []webrtc.RTCPFeedback{
			{Type: "nack", Parameter: ""},
			{Type: "nack", Parameter: "pli"},
		},
	}

	return NewDownTrack(
		capability,
		receiver,
		p.cfg.BufferFactory,
		p.userID,
		p.cfg.RouterConfig.MaxPacketTrack,
	)
}

// setupRelayRTCPHandler はリレートラックのRTCPフィードバックハンドラを設定する
func (p *publisher) setupRelayRTCPHandler(rtpSender *webrtc.RTPSender, track *webrtc.TrackRemote) {
	ssrc := uint32(rtpSender.GetParameters().Encodings[0].SSRC)
	rtcpReader := p.cfg.BufferFactory.GetOrNew(packetio.RTCPBufferPacket, ssrc).(*buffer.RTCPReader)

	rtcpReader.OnPacket(func(bytes []byte) {
		p.handleRelayRTCPPacket(bytes, track)
	})
}

// handleRelayRTCPPacket はRTCPパケットを処理し、PLIをパブリッシャに転送する
func (p *publisher) handleRelayRTCPPacket(bytes []byte, track *webrtc.TrackRemote) {
	packets, err := rtcp.Unmarshal(bytes)
	if err != nil {
		return
	}

	rtcpPackets := p.extractPLIPackets(packets, track)
	if len(rtcpPackets) == 0 {
		return
	}

	if err := p.pc.WriteRTCP(rtcpPackets); err != nil {
		slog.Error("write rtcp error", "error", err)
	}
}

// extractPLIPackets はRTCPパケットからPLIを抽出し、適切なSSRCに変換する
func (p *publisher) extractPLIPackets(packets []rtcp.Packet, track *webrtc.TrackRemote) []rtcp.Packet {
	var rtcpPackets []rtcp.Packet

	for _, pkt := range packets {
		if pli, ok := pkt.(*rtcp.PictureLossIndication); ok {
			rtcpPackets = append(rtcpPackets, &rtcp.PictureLossIndication{
				SenderSSRC: pli.MediaSSRC,
				MediaSSRC:  uint32(track.SSRC()),
			})
		}
	}

	return rtcpPackets
}

// setupRelayCloseHandler はリレートラックのクローズハンドラを設定する
func (p *publisher) setupRelayCloseHandler(downTrack *downTrack, rtpSender *webrtc.RTPSender) {
	downTrack.OnCloseHandler(func() {
		if err := rtpSender.Stop(); err != nil {
			slog.Error("stop rtp sender error", "error", err)
		}
	})
}

// senderReportInterval はSenderReportの送信間隔
const senderReportInterval = 5 * time.Second

// relayReports はリレーピアに定期的にSenderReportを送信する
func (p *publisher) relayReports(rp *relay.Peer) {
	ticker := time.NewTicker(senderReportInterval)
	defer ticker.Stop()

	for range ticker.C {
		if !p.sendSenderReports(rp) {
			return
		}
	}
}

// sendSenderReports はリレーピアにSenderReportを送信する。
// 接続が閉じられた場合はfalseを返す。
func (p *publisher) sendSenderReports(rp *relay.Peer) bool {
	reports := p.collectSenderReports(rp)
	if len(reports) == 0 {
		return true
	}

	if err := rp.WriteRTCP(reports); err != nil {
		if err == io.EOF || err == io.ErrClosedPipe {
			return false
		}
	}

	return true
}

// collectSenderReports はリレーピアの全トラックからSenderReportを収集する
func (p *publisher) collectSenderReports(rp *relay.Peer) []rtcp.Packet {
	var reports []rtcp.Packet

	for _, track := range rp.LocalTracks() {
		if sr := p.createSenderReportIfBound(track); sr != nil {
			reports = append(reports, sr)
		}
	}

	return reports
}

// createSenderReportIfBound はトラックがバインド済みの場合にSenderReportを作成する
func (p *publisher) createSenderReportIfBound(track webrtc.TrackLocal) rtcp.Packet {
	dt, ok := track.(*downTrack)
	if !ok || !dt.Bound() {
		return nil
	}

	return dt.CreateSenderReport()
}

func (p *publisher) PublisherTracks() []PublisherTrack {
	p.mu.Lock()
	defer p.mu.Unlock()

	tracks := make([]PublisherTrack, len(p.tracks))
	copy(tracks, p.tracks)

	return tracks
}

func (p *publisher) AddRelayFanOutDataChannel(label string) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, rp := range p.relayPeers {
		for _, dc := range rp.dcs {
			if dc.Label() == label {
				continue
			}
		}

		dc, err := rp.peer.CreateDataChannel(label)
		if err != nil {
			slog.Error("create data channel error", "error", err)
		}
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			p.session.FanOutMessage("", label, msg)
		})
	}
}

func (p *publisher) GetRelayedDataChannels(label string) []*webrtc.DataChannel {
	p.mu.RLock()
	defer p.mu.RUnlock()

	dcs := make([]*webrtc.DataChannel, 0, len(p.relayPeers))
	for _, rp := range p.relayPeers {
		for _, dc := range rp.dcs {
			if dc.Label() == label {
				dcs = append(dcs, dc)
				break
			}
		}
	}
	return dcs
}

func (p *publisher) Relayed() bool {
	return p.relayed.Load()
}

func (p *publisher) Tracks() []*webrtc.TrackRemote {
	p.mu.RLock()
	defer p.mu.RUnlock()

	tracks := make([]*webrtc.TrackRemote, len(p.tracks))
	for i := range p.tracks {
		tracks[i] = p.tracks[i].Track
	}

	return tracks
}

func (p *publisher) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	if p.pc.RemoteDescription() != nil {
		return p.pc.AddICECandidate(candidate)
	}

	p.candidates = append(p.candidates, candidate)

	return nil
}
