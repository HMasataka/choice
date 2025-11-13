package sfu

import (
	"sync"
)

/*
Peerはsfuに参加しているclientを抽象化したインターフェースです。
clientはPub/Subモデルでメディアを送受信するため、PeerはPublisherおよびSubscriberを保持します。
clientはPublisherおよびSubscriberの2つのコネクションを持ちます。
*/
type Peer interface {
	UserID() string
	Join(sessionID, userID string) error
}

var _ Peer = (*peerLocal)(nil)

func NewPeer(sessionProvider SessionProvider) Peer {
	return &peerLocal{
		sessionProvider: sessionProvider,
	}
}

type peerLocal struct {
	mu sync.RWMutex

	userID          string
	session         Session
	sessionProvider SessionProvider

	publisher  *Publisher
	subscriber *Subscriber
}

func (p *peerLocal) UserID() string {
	return p.userID
}

func (p *peerLocal) Join(sessionID, userID string) error {
	p.userID = userID
	p.session = p.sessionProvider.GetSession(sessionID)

	cfg := NewWebRTCTransportConfig()

	if err := p.setupPublisher(userID, p.session, cfg); err != nil {
		return err
	}

	if err := p.setupSubscriber(); err != nil {
		return err
	}

	return nil
}

func (p *peerLocal) setupSubscriber() error {
	s := NewSubscriber()
	p.subscriber = s

	return nil
}

func (p *peerLocal) setupPublisher(userID string, session Session, cfg *WebRTCTransportConfig) error {
	s, err := NewPublisher(userID, session, cfg)
	if err != nil {
		return err
	}

	p.publisher = s

	return nil
}
