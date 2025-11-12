package sfu

// RouterはReceiverから受信したメディアを適切なDowntrackにルーティングするための抽象化された構造体です。
type Router interface {
}

var _ Router = (*router)(nil)

type router struct {
	userID string

	session Session

	config RouterConfig
}

func NewRouter(userID string, session Session, cfg *WebRTCTransportConfig) *router {
	return &router{
		userID:  userID,
		session: session,
		config:  cfg.RouterConfig,
	}
}

type RouterConfig struct {
	WithStats           bool            `mapstructure:"withstats"`
	MaxBandwidth        uint64          `mapstructure:"maxbandwidth"`
	MaxPacketTrack      int             `mapstructure:"maxpackettrack"`
	AudioLevelInterval  int             `mapstructure:"audiolevelinterval"`
	AudioLevelThreshold uint8           `mapstructure:"audiolevelthreshold"`
	AudioLevelFilter    int             `mapstructure:"audiolevelfilter"`
	Simulcast           SimulcastConfig `mapstructure:"simulcast"`
}
