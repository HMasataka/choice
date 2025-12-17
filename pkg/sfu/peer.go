package sfu

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/pion/webrtc/v4"
)

type ChannelAPIMessage struct {
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

/*
Peerはsfuに参加しているclientを抽象化したインターフェースです。
clientはPub/Subモデルでメディアを送受信するため、PeerはPublisherおよびSubscriberを保持します。
clientはPublisherおよびSubscriberの2つのコネクションを持ちます。
*/
type Peer interface {
	UserID() string
	Join(ctx context.Context, sessionID, userID string, config JoinConfig) error
	Publisher() Publisher
	Subscriber() Subscriber
	SetOnOffer(f func(*webrtc.SessionDescription))
	SetOnIceCandidate(f func(*webrtc.ICECandidateInit, ConnectionType))
	SetOnIceConnectionStateChange(f func(webrtc.ICEConnectionState))
	SetRemoteDescription(sdp webrtc.SessionDescription) error
}

var _ Peer = (*peerLocal)(nil)

func NewPeer(sessionProvider SessionProvider) Peer {
	return &peerLocal{
		sessionProvider: sessionProvider,
	}
}

type peerLocal struct {
	mu sync.RWMutex

	closed atomic.Bool

	userID          string
	session         Session
	sessionProvider SessionProvider

	publisher  Publisher
	subscriber Subscriber

	OnOffer                    func(*webrtc.SessionDescription)
	OnIceCandidate             func(*webrtc.ICECandidateInit, ConnectionType)
	OnICEConnectionStateChange func(webrtc.ICEConnectionState)

	remoteAnswerPending bool
	negotiationPending  bool
}

type ConnectionType string

const (
	ConnectionTypePublisher  ConnectionType = "publisher"
	ConnectionTypeSubscriber ConnectionType = "subscriber"
)

var (
	// ErrTransportExists join is called after a peerconnection is established
	ErrTransportExists = errors.New("rtc transport already exists for this connection")
	// ErrNoTransportEstablished cannot signal before join
	ErrNoTransportEstablished = errors.New("no rtc transport exists for this Peer")
	// ErrOfferIgnored if offer received in unstable state
	ErrOfferIgnored = errors.New("offered ignored")
)

func (p *peerLocal) UserID() string {
	return p.userID
}

type JoinConfig struct {
	NoPublish     bool
	NoSubscribe   bool
	AutoSubscribe bool
}

func (p *peerLocal) Join(ctx context.Context, sessionID, userID string, config JoinConfig) error {
	p.userID = userID
	p.session = p.sessionProvider.GetSession(sessionID)

	cfg := p.sessionProvider.GetTransportConfig()

	if !config.NoSubscribe {
		if err := p.setupSubscriber(userID, &cfg, config); err != nil {
			return err
		}
	}

	if !config.NoPublish {
		if err := p.setupPublisher(ctx, userID, p.session, config, &cfg); err != nil {
			return err
		}
	}

	p.session.AddPeer(p)

	if !config.NoSubscribe {
		p.session.Subscribe(p)
	}

	// メディアトラックが追加された後にDataChannelを追加することで、negotiationの順序を最適化
	if !config.NoSubscribe && !config.NoPublish {
		for _, dc := range p.session.GetDCMiddlewares() {
			if err := p.subscriber.AddDatachannel(ctx, p, dc); err != nil {
				return fmt.Errorf("setting subscriber default dc datachannel: %w", err)
			}
		}
	}

	// すべての準備が整った後、明示的にNegotiateを呼ぶ
	// これにより、メディアトラックとDataChannelがすべて追加された状態でOfferが作成される
	// ただし、メディアトラックがある場合のみ（空のOfferを作成しないため）
	if !config.NoSubscribe && p.subscriber != nil {
		downtrackCount := len(p.subscriber.DownTracks())

		// メディアトラックがある場合のみNegotiateを実行
		// 最初のピアが参加した時は他にピアがいないため、トラックは0個
		// この場合はOfferを作成しない（空のSDPを避けるため）
		if downtrackCount > 0 {
			p.subscriber.Negotiate()
		}
	}

	return nil
}

func (p *peerLocal) setupSubscriber(userID string, wcfg *WebRTCTransportConfig, config JoinConfig) error {
	s := NewSubscriber(userID, config.AutoSubscribe, wcfg)
	s.userID = userID
	p.subscriber = s

	p.subscriber.OnNegotiationNeeded(func() {
		p.mu.Lock()
		defer p.mu.Unlock()

		if p.remoteAnswerPending {
			p.negotiationPending = true
			return
		}

		offer, err := p.subscriber.CreateOffer()
		if err != nil {
			return
		}

		p.remoteAnswerPending = true
		if p.OnOffer != nil && !p.closed.Load() {
			p.OnOffer(&offer)
		} else {
			p.remoteAnswerPending = false
		}
	})

	p.subscriber.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}

		if p.OnIceCandidate != nil && !p.closed.Load() {
			json := c.ToJSON()
			p.OnIceCandidate(&json, ConnectionTypeSubscriber)
		}
	})

	return nil
}

func (p *peerLocal) setupPublisher(ctx context.Context, userID string, session Session, joinConfig JoinConfig, webrtcConfig *WebRTCTransportConfig) error {
	publisher, err := NewPublisher(userID, session, webrtcConfig)
	if err != nil {
		return err
	}

	publisher.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}

		if p.OnIceCandidate != nil && !p.closed.Load() {
			json := c.ToJSON()
			p.OnIceCandidate(&json, ConnectionTypePublisher)
		}
	})

	publisher.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		if p.OnICEConnectionStateChange != nil && !p.closed.Load() {
			p.OnICEConnectionStateChange(s)
		}
	})

	p.publisher = publisher

	return nil
}

func (p *peerLocal) Trickle(candidate webrtc.ICECandidateInit, target ConnectionType) error {
	if p.subscriber == nil || p.publisher == nil {
		return ErrNoTransportEstablished
	}

	switch target {
	case ConnectionTypePublisher:
		if err := p.publisher.AddICECandidate(candidate); err != nil {
			return fmt.Errorf("setting ice candidate: %w", err)
		}
	case ConnectionTypeSubscriber:
		if err := p.subscriber.AddICECandidate(candidate); err != nil {
			return fmt.Errorf("setting ice candidate: %w", err)
		}
	}
	return nil
}

func (p *peerLocal) Publisher() Publisher {
	return p.publisher
}

func (p *peerLocal) Subscriber() Subscriber {
	return p.subscriber
}

func (p *peerLocal) SetOnOffer(f func(*webrtc.SessionDescription)) {
	p.OnOffer = f
}

func (p *peerLocal) SetOnIceCandidate(f func(*webrtc.ICECandidateInit, ConnectionType)) {
	p.OnIceCandidate = f
}

func (p *peerLocal) SetOnIceConnectionStateChange(f func(webrtc.ICEConnectionState)) {
	p.OnICEConnectionStateChange = f
}

// SetRemoteDescription when receiving an answer from client for the subscriber PC
func (p *peerLocal) SetRemoteDescription(sdp webrtc.SessionDescription) error {
	if p.subscriber == nil {
		return ErrNoTransportEstablished
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.subscriber.SetRemoteDescription(sdp); err != nil {
		return fmt.Errorf("setting remote description: %w", err)
	}

	p.remoteAnswerPending = false
	if p.negotiationPending {
		p.negotiationPending = false
		p.subscriber.Negotiate()
	}

	return nil
}
