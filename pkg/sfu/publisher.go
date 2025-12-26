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
	// Route RTCP feedback from subscribers back to this publisher PC
	router.SetRTCPWriter(pc.WriteRTCP)

	p := &publisher{
		userID:  userID,
		pc:      pc,
		router:  router,
		session: session,
		cfg:     cfg,
	}

	// When an upstream media track arrives from the client, register it with the Router and publish to current subscribers.
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		if track == nil || receiver == nil {
			return
		}
		recv, pub := router.AddReceiver(receiver, track, track.ID(), track.StreamID())
		if pub {
			recv.SetTrackMeta(track.ID(), track.StreamID())
			session.Publish(router, recv)
		}
		// Track bookkeeping and optional callback
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
	// This will be used in the future for tracks that will be relayed as clients or servers
	// This is for SVC and Simulcast where you will be able to chose if the relayed peer just
	// want a single track (for recording/ processing) or get all the tracks (for load balancing)
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
	lrp := &relayPeer{}
	for _, o := range options {
		o(lrp)
	}

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

	lrp.peer = rp

	rp.OnReady(func() {
		peer := p.session.GetPeer(p.userID)

		p.relayed.Store(true)

		if lrp.relayFanOutDataChannels {
			for _, lbl := range p.session.GetFanOutDataChannelLabels() {
				dc, err := rp.CreateDataChannel(lbl)
				if err != nil {
					continue
				}

				dc.OnMessage(func(msg webrtc.DataChannelMessage) {
					if peer == nil || peer.Subscriber() == nil {
						return
					}

					if sdc := peer.Subscriber().DataChannel(lbl); sdc != nil {
						if msg.IsString {
							if err = sdc.SendText(string(msg.Data)); err != nil {
								slog.Error("subscriber send text error", "error", err)
							}
						} else {
							if err = sdc.Send(msg.Data); err != nil {
								slog.Error("subscriber send data error", "error", err)
							}
						}
					}
				})
			}
		}

		p.mu.Lock()
		for _, tp := range p.tracks {
			if !tp.clientRelay {
				// simulcast will just relay client track for now
				continue
			}

			if err = p.createRelayTrack(tp.Track, tp.Receiver, rp); err != nil {
				slog.Error("create relay track error", "error", err)
			}
		}
		p.relayPeers = append(p.relayPeers, lrp)
		p.mu.Unlock()

		if lrp.withSRReports {
			go p.relayReports(rp)
		}
	})

	rp.OnDataChannel(func(channel *webrtc.DataChannel) {
		if !lrp.relayFanOutDataChannels {
			return
		}
		p.mu.Lock()
		lrp.dcs = append(lrp.dcs, channel)
		p.mu.Unlock()

		p.session.AddDatachannel("", channel)
	})

	if err = rp.Offer(signalFn); err != nil {
		return nil, fmt.Errorf("relay: %w", err)
	}

	return rp, nil
}

func (p *publisher) createRelayTrack(track *webrtc.TrackRemote, receiver Receiver, rp *relay.Peer) error {
	codec := track.Codec()

	downTrack, err := NewDownTrack(webrtc.RTPCodecCapability{
		MimeType:    codec.MimeType,
		ClockRate:   codec.ClockRate,
		Channels:    codec.Channels,
		SDPFmtpLine: codec.SDPFmtpLine,
		RTCPFeedback: []webrtc.RTCPFeedback{
			{Type: "nack", Parameter: ""}, {Type: "nack", Parameter: "pli"},
		},
	},
		receiver,
		p.cfg.BufferFactory,
		p.userID,
		p.cfg.RouterConfig.MaxPacketTrack,
	)
	if err != nil {
		return err
	}

	rtpSender, err := rp.AddTrack(receiver.(*WebRTCReceiver).receiver, track, downTrack)
	if err != nil {
		return fmt.Errorf("relay: %w", err)
	}

	p.cfg.BufferFactory.GetOrNew(packetio.RTCPBufferPacket,
		uint32(rtpSender.GetParameters().Encodings[0].SSRC)).(*buffer.RTCPReader).OnPacket(func(bytes []byte) {
		pkts, err := rtcp.Unmarshal(bytes)
		if err != nil {
			return
		}
		var rpkts []rtcp.Packet
		for _, pkt := range pkts {
			switch pk := pkt.(type) {
			case *rtcp.PictureLossIndication:
				rpkts = append(rpkts, &rtcp.PictureLossIndication{
					SenderSSRC: pk.MediaSSRC,
					MediaSSRC:  uint32(track.SSRC()),
				})
			}
		}

		if len(rpkts) > 0 {
			if err := p.pc.WriteRTCP(rpkts); err != nil {
				slog.Error("write rtcp error", "error", err)
			}
		}

	})

	downTrack.OnCloseHandler(func() {
		if err = rtpSender.Stop(); err != nil {
			slog.Error("stop rtp sender error", "error", err)
		}
	})

	receiver.AddDownTrack(downTrack, true)
	return nil
}

func (p *publisher) relayReports(rp *relay.Peer) {
	for {
		time.Sleep(5 * time.Second)

		var r []rtcp.Packet
		for _, t := range rp.LocalTracks() {
			if dt, ok := t.(*downTrack); ok {
				if !dt.Bound() {
					continue
				}

				if sr := dt.CreateSenderReport(); sr != nil {
					r = append(r, sr)
				}
			}
		}

		if len(r) == 0 {
			continue
		}

		if err := rp.WriteRTCP(r); err != nil {
			if err == io.EOF || err == io.ErrClosedPipe {
				return
			}
		}
	}
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
