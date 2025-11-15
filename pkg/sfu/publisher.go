package sfu

import "github.com/pion/webrtc/v4"

// Publisherはclientがメディアを送信するための抽象化された構造体です。
// ClientとPublisherは1対1の関係にあり、ClientはPublisherを使用してメディアストリームをsfuに送信します。
type Publisher interface {
}

type publisher struct {
	userID string
	pc     *webrtc.PeerConnection

	router  Router
	session Session

	cfg *WebRTCTransportConfig
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

	return &publisher{
		userID:  userID,
		pc:      pc,
		router:  router,
		session: session,
		cfg:     cfg,
	}, nil
}
