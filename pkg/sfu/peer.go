package sfu

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
	userID          string
	session         Session
	sessionProvider SessionProvider
}

func (p *peerLocal) UserID() string {
	return p.userID
}

func (p *peerLocal) Join(sessionID, userID string) error {
	p.userID = userID
	p.session = p.sessionProvider.GetSession(sessionID)
	return nil
}
