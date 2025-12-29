package sfu_test

import (
	"testing"

	"github.com/HMasataka/choice/pkg/sfu"
	mock_sfu "github.com/HMasataka/choice/pkg/sfu/mock"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestChannelAPIMessage(t *testing.T) {
	t.Run("構造体の初期化", func(t *testing.T) {
		msg := sfu.ChannelAPIMessage{
			Method: "audioLevels",
			Params: map[string]interface{}{"level": 50},
		}

		assert.Equal(t, "audioLevels", msg.Method)
		assert.NotNil(t, msg.Params)
	})

	t.Run("Paramsなし", func(t *testing.T) {
		msg := sfu.ChannelAPIMessage{
			Method: "ping",
		}

		assert.Equal(t, "ping", msg.Method)
		assert.Nil(t, msg.Params)
	})
}

func TestConnectionType(t *testing.T) {
	t.Run("定数の値", func(t *testing.T) {
		assert.Equal(t, sfu.ConnectionType("publisher"), sfu.ConnectionTypePublisher)
		assert.Equal(t, sfu.ConnectionType("subscriber"), sfu.ConnectionTypeSubscriber)
	})

	t.Run("文字列比較", func(t *testing.T) {
		ct := sfu.ConnectionTypePublisher
		assert.Equal(t, "publisher", string(ct))

		ct = sfu.ConnectionTypeSubscriber
		assert.Equal(t, "subscriber", string(ct))
	})
}

func TestPeerErrors(t *testing.T) {
	t.Run("ErrTransportExists", func(t *testing.T) {
		assert.NotNil(t, sfu.ErrTransportExists)
		assert.Equal(t, "rtc transport already exists for this connection", sfu.ErrTransportExists.Error())
	})

	t.Run("ErrNoTransportEstablished", func(t *testing.T) {
		assert.NotNil(t, sfu.ErrNoTransportEstablished)
		assert.Equal(t, "no rtc transport exists for this Peer", sfu.ErrNoTransportEstablished.Error())
	})

	t.Run("ErrOfferIgnored", func(t *testing.T) {
		assert.NotNil(t, sfu.ErrOfferIgnored)
		assert.Equal(t, "offered ignored", sfu.ErrOfferIgnored.Error())
	})
}

func TestJoinConfig(t *testing.T) {
	t.Run("デフォルト値", func(t *testing.T) {
		cfg := sfu.JoinConfig{}

		assert.False(t, cfg.NoPublish)
		assert.False(t, cfg.NoSubscribe)
		assert.False(t, cfg.AutoSubscribe)
	})

	t.Run("値を設定", func(t *testing.T) {
		cfg := sfu.JoinConfig{
			NoPublish:     true,
			NoSubscribe:   true,
			AutoSubscribe: true,
		}

		assert.True(t, cfg.NoPublish)
		assert.True(t, cfg.NoSubscribe)
		assert.True(t, cfg.AutoSubscribe)
	})

	t.Run("Publish専用設定", func(t *testing.T) {
		cfg := sfu.JoinConfig{
			NoPublish:   false,
			NoSubscribe: true,
		}

		assert.False(t, cfg.NoPublish)
		assert.True(t, cfg.NoSubscribe)
	})

	t.Run("Subscribe専用設定", func(t *testing.T) {
		cfg := sfu.JoinConfig{
			NoPublish:     true,
			NoSubscribe:   false,
			AutoSubscribe: true,
		}

		assert.True(t, cfg.NoPublish)
		assert.False(t, cfg.NoSubscribe)
		assert.True(t, cfg.AutoSubscribe)
	})
}

func TestNewPeer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("正常に初期化される", func(t *testing.T) {
		provider := mock_sfu.NewMockSessionProvider(ctrl)

		peer := sfu.NewPeer(provider)

		require.NotNil(t, peer)
	})
}

func TestPeerLocal_UserID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	provider := mock_sfu.NewMockSessionProvider(ctrl)
	peer := sfu.NewPeer(provider)

	t.Run("初期値は空", func(t *testing.T) {
		assert.Equal(t, "", peer.UserID())
	})
}

func TestPeerLocal_Publisher(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	provider := mock_sfu.NewMockSessionProvider(ctrl)
	peer := sfu.NewPeer(provider)

	t.Run("初期値はnil", func(t *testing.T) {
		assert.Nil(t, peer.Publisher())
	})
}

func TestPeerLocal_Subscriber(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	provider := mock_sfu.NewMockSessionProvider(ctrl)
	peer := sfu.NewPeer(provider)

	t.Run("初期値はnil", func(t *testing.T) {
		assert.Nil(t, peer.Subscriber())
	})
}

func TestPeerLocal_SetOnOffer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	provider := mock_sfu.NewMockSessionProvider(ctrl)
	peer := sfu.NewPeer(provider)

	t.Run("コールバックを設定", func(t *testing.T) {
		// パニックしないことを確認
		peer.SetOnOffer(func(sdp *webrtc.SessionDescription) {})
	})

	t.Run("nilで設定", func(t *testing.T) {
		// パニックしないことを確認
		peer.SetOnOffer(nil)
	})
}

func TestPeerLocal_SetOnIceCandidate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	provider := mock_sfu.NewMockSessionProvider(ctrl)
	peer := sfu.NewPeer(provider)

	t.Run("コールバックを設定", func(t *testing.T) {
		// パニックしないことを確認
		peer.SetOnIceCandidate(func(candidate *webrtc.ICECandidateInit, ct sfu.ConnectionType) {})
	})
}

func TestPeerLocal_SetOnIceConnectionStateChange(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	provider := mock_sfu.NewMockSessionProvider(ctrl)
	peer := sfu.NewPeer(provider)

	t.Run("コールバックを設定", func(t *testing.T) {
		// パニックしないことを確認
		peer.SetOnIceConnectionStateChange(func(state webrtc.ICEConnectionState) {})
	})
}

func TestPeerLocal_Trickle(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	provider := mock_sfu.NewMockSessionProvider(ctrl)
	peer := sfu.NewPeer(provider)

	t.Run("トランスポート未確立でエラー", func(t *testing.T) {
		candidate := webrtc.ICECandidateInit{
			Candidate: "candidate:1 1 UDP 2130706431 192.168.1.1 12345 typ host",
		}

		err := peer.Trickle(candidate, sfu.ConnectionTypePublisher)

		assert.ErrorIs(t, err, sfu.ErrNoTransportEstablished)
	})
}

func TestPeerLocal_SetRemoteDescription(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	provider := mock_sfu.NewMockSessionProvider(ctrl)
	peer := sfu.NewPeer(provider)

	t.Run("subscriberなしでエラー", func(t *testing.T) {
		sdp := webrtc.SessionDescription{
			Type: webrtc.SDPTypeAnswer,
			SDP:  "v=0\r\n",
		}

		err := peer.SetRemoteDescription(sdp)

		assert.ErrorIs(t, err, sfu.ErrNoTransportEstablished)
	})
}

func TestPeerInterface(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("peerLocal型がPeerインターフェースを実装している", func(t *testing.T) {
		provider := mock_sfu.NewMockSessionProvider(ctrl)
		var _ sfu.Peer = sfu.NewPeer(provider)
	})
}
