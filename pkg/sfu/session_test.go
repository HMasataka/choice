package sfu_test

import (
	"sync"
	"testing"

	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/HMasataka/choice/pkg/sfu"
	mock_sfu "github.com/HMasataka/choice/pkg/sfu/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestAudioLevelsMethodConstant(t *testing.T) {
	assert.Equal(t, "audioLevels", sfu.AudioLevelsMethod)
}

func TestNewSession(t *testing.T) {
	t.Run("正常に初期化される", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := sfu.WebRTCTransportConfig{
			BufferFactory: bf,
			RouterConfig: sfu.RouterConfig{
				AudioLevelInterval:  100,
				AudioLevelThreshold: 40,
				AudioLevelFilter:    50,
			},
		}

		session := sfu.NewSession("session-123", nil, cfg)

		require.NotNil(t, session)
		assert.Equal(t, "session-123", session.ID())
		assert.NotNil(t, session.AudioObserver())
	})

	t.Run("Datachannelを設定", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := sfu.WebRTCTransportConfig{BufferFactory: bf}
		dcs := []*sfu.Datachannel{
			{Label: "chat"},
			{Label: "control"},
		}

		session := sfu.NewSession("session-123", dcs, cfg)

		assert.Len(t, session.GetDCMiddlewares(), 2)
	})
}

func TestSessionLocal_ID(t *testing.T) {
	bf := buffer.NewBufferFactory(500)
	cfg := sfu.WebRTCTransportConfig{BufferFactory: bf}

	session := sfu.NewSession("test-session-id", nil, cfg)

	assert.Equal(t, "test-session-id", session.ID())
}

func TestSessionLocal_AudioObserver(t *testing.T) {
	bf := buffer.NewBufferFactory(500)
	cfg := sfu.WebRTCTransportConfig{
		BufferFactory: bf,
		RouterConfig: sfu.RouterConfig{
			AudioLevelThreshold: 50,
			AudioLevelInterval:  100,
			AudioLevelFilter:    50,
		},
	}

	session := sfu.NewSession("test-session", nil, cfg)

	ao := session.AudioObserver()
	require.NotNil(t, ao)
}

func TestSessionLocal_GetDCMiddlewares(t *testing.T) {
	bf := buffer.NewBufferFactory(500)
	cfg := sfu.WebRTCTransportConfig{BufferFactory: bf}

	t.Run("Datachannelなし", func(t *testing.T) {
		session := sfu.NewSession("session-1", nil, cfg)

		dcs := session.GetDCMiddlewares()
		assert.Nil(t, dcs)
	})

	t.Run("Datachannelあり", func(t *testing.T) {
		dcs := []*sfu.Datachannel{
			{Label: "chat"},
		}
		session := sfu.NewSession("session-1", dcs, cfg)

		result := session.GetDCMiddlewares()
		require.Len(t, result, 1)
		assert.Equal(t, "chat", result[0].Label)
	})
}

func TestSessionLocal_GetFanOutDataChannelLabels(t *testing.T) {
	bf := buffer.NewBufferFactory(500)
	cfg := sfu.WebRTCTransportConfig{BufferFactory: bf}

	t.Run("初期状態は空", func(t *testing.T) {
		session := sfu.NewSession("session-1", nil, cfg)

		labels := session.GetFanOutDataChannelLabels()
		assert.Empty(t, labels)
	})
}

func TestSessionLocal_AddPeer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	bf := buffer.NewBufferFactory(500)
	cfg := sfu.WebRTCTransportConfig{BufferFactory: bf}

	t.Run("Peerを追加", func(t *testing.T) {
		session := sfu.NewSession("session-1", nil, cfg)
		peer := mock_sfu.NewMockPeer(ctrl)
		peer.EXPECT().UserID().Return("user-1").AnyTimes()

		session.AddPeer(peer)

		result := session.GetPeer("user-1")
		assert.Equal(t, peer, result)
	})

	t.Run("複数のPeerを追加", func(t *testing.T) {
		session := sfu.NewSession("session-1", nil, cfg)
		peer1 := mock_sfu.NewMockPeer(ctrl)
		peer1.EXPECT().UserID().Return("user-1").AnyTimes()
		peer2 := mock_sfu.NewMockPeer(ctrl)
		peer2.EXPECT().UserID().Return("user-2").AnyTimes()

		session.AddPeer(peer1)
		session.AddPeer(peer2)

		peers := session.Peers()
		assert.Len(t, peers, 2)
	})
}

func TestSessionLocal_GetPeer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	bf := buffer.NewBufferFactory(500)
	cfg := sfu.WebRTCTransportConfig{BufferFactory: bf}

	t.Run("存在するPeerを取得", func(t *testing.T) {
		session := sfu.NewSession("session-1", nil, cfg)
		peer := mock_sfu.NewMockPeer(ctrl)
		peer.EXPECT().UserID().Return("user-1").AnyTimes()
		session.AddPeer(peer)

		result := session.GetPeer("user-1")

		assert.Equal(t, peer, result)
	})

	t.Run("存在しないPeer", func(t *testing.T) {
		session := sfu.NewSession("session-1", nil, cfg)

		result := session.GetPeer("nonexistent")

		assert.Nil(t, result)
	})
}

func TestSessionLocal_RemovePeer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	bf := buffer.NewBufferFactory(500)
	cfg := sfu.WebRTCTransportConfig{BufferFactory: bf}

	t.Run("Peerを削除", func(t *testing.T) {
		session := sfu.NewSession("session-1", nil, cfg)
		peer := mock_sfu.NewMockPeer(ctrl)
		peer.EXPECT().UserID().Return("user-1").AnyTimes()
		session.AddPeer(peer)

		session.RemovePeer(peer)

		assert.Empty(t, session.Peers())
	})

	t.Run("最後のPeerを削除するとセッションがクローズ", func(t *testing.T) {
		session := sfu.NewSession("session-1", nil, cfg)
		peer := mock_sfu.NewMockPeer(ctrl)
		peer.EXPECT().UserID().Return("user-1").AnyTimes()
		session.AddPeer(peer)

		closed := false
		session.OnClose(func() {
			closed = true
		})

		session.RemovePeer(peer)

		assert.True(t, closed)
	})
}

func TestSessionLocal_Peers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	bf := buffer.NewBufferFactory(500)
	cfg := sfu.WebRTCTransportConfig{BufferFactory: bf}

	t.Run("空のPeers", func(t *testing.T) {
		session := sfu.NewSession("session-1", nil, cfg)

		peers := session.Peers()

		assert.Empty(t, peers)
	})

	t.Run("複数のPeers", func(t *testing.T) {
		session := sfu.NewSession("session-1", nil, cfg)
		peer1 := mock_sfu.NewMockPeer(ctrl)
		peer1.EXPECT().UserID().Return("user-1").AnyTimes()
		peer2 := mock_sfu.NewMockPeer(ctrl)
		peer2.EXPECT().UserID().Return("user-2").AnyTimes()
		session.AddPeer(peer1)
		session.AddPeer(peer2)

		peers := session.Peers()

		assert.Len(t, peers, 2)
	})
}

func TestSessionLocal_RelayPeers(t *testing.T) {
	bf := buffer.NewBufferFactory(500)
	cfg := sfu.WebRTCTransportConfig{BufferFactory: bf}

	t.Run("空のRelayPeers", func(t *testing.T) {
		session := sfu.NewSession("session-1", nil, cfg)

		peers := session.RelayPeers()

		assert.Empty(t, peers)
	})
}

func TestSessionLocal_OnClose(t *testing.T) {
	bf := buffer.NewBufferFactory(500)
	cfg := sfu.WebRTCTransportConfig{BufferFactory: bf}

	t.Run("コールバックを設定", func(t *testing.T) {
		session := sfu.NewSession("session-1", nil, cfg)

		called := false
		session.OnClose(func() {
			called = true
		})

		// OnCloseが設定されたことを確認（Closeは直接呼べないのでコールバック登録のみ確認）
		assert.False(t, called) // まだ呼ばれていない
	})
}

func TestSessionInterface(t *testing.T) {
	t.Run("sessionLocal型がSessionインターフェースを実装している", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := sfu.WebRTCTransportConfig{BufferFactory: bf}

		var _ sfu.Session = sfu.NewSession("session-1", nil, cfg)
	})
}

func TestSessionLocal_ConcurrentAccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	bf := buffer.NewBufferFactory(500)
	cfg := sfu.WebRTCTransportConfig{BufferFactory: bf}
	session := sfu.NewSession("session-1", nil, cfg)

	var wg sync.WaitGroup

	// 並行してPeerを追加
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			peer := mock_sfu.NewMockPeer(ctrl)
			peer.EXPECT().UserID().Return("user-" + string(rune('0'+id))).AnyTimes()
			session.AddPeer(peer)
		}(i)
	}

	// 並行してPeersを取得
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = session.Peers()
		}()
	}

	// 並行してGetPeerを呼び出し
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_ = session.GetPeer("user-" + string(rune('0'+id)))
		}(i)
	}

	wg.Wait()
	// パニックなく完了すればテスト成功
}
