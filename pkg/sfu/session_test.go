package sfu

import (
	"context"
	"sync"
	"testing"

	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAudioLevelsMethodConstant(t *testing.T) {
	assert.Equal(t, "audioLevels", AudioLevelsMethod)
}

func TestNewSession(t *testing.T) {
	t.Run("正常に初期化される", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := WebRTCTransportConfig{
			BufferFactory: bf,
			RouterConfig: RouterConfig{
				AudioLevelInterval:  100,
				AudioLevelThreshold: 40,
				AudioLevelFilter:    50,
			},
		}

		session := NewSession("session-123", nil, cfg)

		require.NotNil(t, session)
		assert.Equal(t, "session-123", session.ID())
		assert.NotNil(t, session.AudioObserver())
	})

	t.Run("Datachannelを設定", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := WebRTCTransportConfig{BufferFactory: bf}
		dcs := []*Datachannel{
			{Label: "chat"},
			{Label: "control"},
		}

		session := NewSession("session-123", dcs, cfg)

		assert.Len(t, session.GetDCMiddlewares(), 2)
	})
}

func TestSessionLocal_ID(t *testing.T) {
	bf := buffer.NewBufferFactory(500)
	cfg := WebRTCTransportConfig{BufferFactory: bf}

	session := NewSession("test-session-id", nil, cfg)

	assert.Equal(t, "test-session-id", session.ID())
}

func TestSessionLocal_AudioObserver(t *testing.T) {
	bf := buffer.NewBufferFactory(500)
	cfg := WebRTCTransportConfig{
		BufferFactory: bf,
		RouterConfig: RouterConfig{
			AudioLevelThreshold: 50,
			AudioLevelInterval:  100,
			AudioLevelFilter:    50,
		},
	}

	session := NewSession("test-session", nil, cfg)

	ao := session.AudioObserver()
	require.NotNil(t, ao)
}

func TestSessionLocal_GetDCMiddlewares(t *testing.T) {
	bf := buffer.NewBufferFactory(500)
	cfg := WebRTCTransportConfig{BufferFactory: bf}

	t.Run("Datachannelなし", func(t *testing.T) {
		session := NewSession("session-1", nil, cfg)

		dcs := session.GetDCMiddlewares()
		assert.Nil(t, dcs)
	})

	t.Run("Datachannelあり", func(t *testing.T) {
		dcs := []*Datachannel{
			{Label: "chat"},
		}
		session := NewSession("session-1", dcs, cfg)

		result := session.GetDCMiddlewares()
		require.Len(t, result, 1)
		assert.Equal(t, "chat", result[0].Label)
	})
}

func TestSessionLocal_GetFanOutDataChannelLabels(t *testing.T) {
	bf := buffer.NewBufferFactory(500)
	cfg := WebRTCTransportConfig{BufferFactory: bf}

	t.Run("初期状態は空", func(t *testing.T) {
		session := NewSession("session-1", nil, cfg).(*sessionLocal)

		labels := session.GetFanOutDataChannelLabels()
		assert.Empty(t, labels)
	})

	t.Run("ラベルが追加された後", func(t *testing.T) {
		session := NewSession("session-1", nil, cfg).(*sessionLocal)
		session.fanOutDCs = []string{"chat", "control"}

		labels := session.GetFanOutDataChannelLabels()
		assert.Equal(t, []string{"chat", "control"}, labels)
	})

	t.Run("コピーが返される", func(t *testing.T) {
		session := NewSession("session-1", nil, cfg).(*sessionLocal)
		session.fanOutDCs = []string{"chat"}

		labels := session.GetFanOutDataChannelLabels()
		labels[0] = "modified"

		// 元のスライスは変更されない
		assert.Equal(t, []string{"chat"}, session.fanOutDCs)
	})
}

func TestSessionLocal_AddPeer(t *testing.T) {
	bf := buffer.NewBufferFactory(500)
	cfg := WebRTCTransportConfig{BufferFactory: bf}

	t.Run("Peerを追加", func(t *testing.T) {
		session := NewSession("session-1", nil, cfg).(*sessionLocal)
		peer := &mockPeer{userID: "user-1"}

		session.AddPeer(peer)

		assert.Len(t, session.peers, 1)
		assert.Equal(t, peer, session.peers["user-1"])
	})

	t.Run("複数のPeerを追加", func(t *testing.T) {
		session := NewSession("session-1", nil, cfg).(*sessionLocal)
		peer1 := &mockPeer{userID: "user-1"}
		peer2 := &mockPeer{userID: "user-2"}

		session.AddPeer(peer1)
		session.AddPeer(peer2)

		assert.Len(t, session.peers, 2)
	})

	t.Run("同じIDで上書き", func(t *testing.T) {
		session := NewSession("session-1", nil, cfg).(*sessionLocal)
		peer1 := &mockPeer{userID: "user-1"}
		peer2 := &mockPeer{userID: "user-1"}

		session.AddPeer(peer1)
		session.AddPeer(peer2)

		assert.Len(t, session.peers, 1)
		assert.Equal(t, peer2, session.peers["user-1"])
	})
}

func TestSessionLocal_GetPeer(t *testing.T) {
	bf := buffer.NewBufferFactory(500)
	cfg := WebRTCTransportConfig{BufferFactory: bf}

	t.Run("存在するPeerを取得", func(t *testing.T) {
		session := NewSession("session-1", nil, cfg).(*sessionLocal)
		peer := &mockPeer{userID: "user-1"}
		session.AddPeer(peer)

		result := session.GetPeer("user-1")

		assert.Equal(t, peer, result)
	})

	t.Run("存在しないPeer", func(t *testing.T) {
		session := NewSession("session-1", nil, cfg).(*sessionLocal)

		result := session.GetPeer("nonexistent")

		assert.Nil(t, result)
	})
}

func TestSessionLocal_RemovePeer(t *testing.T) {
	bf := buffer.NewBufferFactory(500)
	cfg := WebRTCTransportConfig{BufferFactory: bf}

	t.Run("Peerを削除", func(t *testing.T) {
		session := NewSession("session-1", nil, cfg).(*sessionLocal)
		peer := &mockPeer{userID: "user-1"}
		session.AddPeer(peer)

		session.RemovePeer(peer)

		assert.Empty(t, session.peers)
	})

	t.Run("異なるPeerインスタンスでは削除されない", func(t *testing.T) {
		session := NewSession("session-1", nil, cfg).(*sessionLocal)
		peer1 := &mockPeer{userID: "user-1"}
		peer2 := &mockPeer{userID: "user-1"} // 同じIDだが異なるインスタンス
		session.AddPeer(peer1)

		session.RemovePeer(peer2)

		// peer1はまだ存在する
		assert.Len(t, session.peers, 1)
	})

	t.Run("最後のPeerを削除するとセッションがクローズ", func(t *testing.T) {
		session := NewSession("session-1", nil, cfg).(*sessionLocal)
		peer := &mockPeer{userID: "user-1"}
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
	bf := buffer.NewBufferFactory(500)
	cfg := WebRTCTransportConfig{BufferFactory: bf}

	t.Run("空のPeers", func(t *testing.T) {
		session := NewSession("session-1", nil, cfg)

		peers := session.Peers()

		assert.Empty(t, peers)
	})

	t.Run("複数のPeers", func(t *testing.T) {
		session := NewSession("session-1", nil, cfg).(*sessionLocal)
		session.AddPeer(&mockPeer{userID: "user-1"})
		session.AddPeer(&mockPeer{userID: "user-2"})

		peers := session.Peers()

		assert.Len(t, peers, 2)
	})
}

func TestSessionLocal_RelayPeers(t *testing.T) {
	bf := buffer.NewBufferFactory(500)
	cfg := WebRTCTransportConfig{BufferFactory: bf}

	t.Run("空のRelayPeers", func(t *testing.T) {
		session := NewSession("session-1", nil, cfg)

		peers := session.RelayPeers()

		assert.Empty(t, peers)
	})
}

func TestSessionLocal_OnClose(t *testing.T) {
	bf := buffer.NewBufferFactory(500)
	cfg := WebRTCTransportConfig{BufferFactory: bf}

	t.Run("コールバックを設定", func(t *testing.T) {
		session := NewSession("session-1", nil, cfg).(*sessionLocal)

		called := false
		session.OnClose(func() {
			called = true
		})

		session.Close()

		assert.True(t, called)
		assert.True(t, session.closed.Load())
	})

	t.Run("コールバックなしでClose", func(t *testing.T) {
		session := NewSession("session-1", nil, cfg).(*sessionLocal)

		// パニックしないことを確認
		session.Close()

		assert.True(t, session.closed.Load())
	})
}

func TestSessionLocal_NormalizeAudioLevelInterval(t *testing.T) {
	bf := buffer.NewBufferFactory(500)
	cfg := WebRTCTransportConfig{BufferFactory: bf}
	session := NewSession("session-1", nil, cfg).(*sessionLocal)

	t.Run("0の場合は1000を返す", func(t *testing.T) {
		result := session.normalizeAudioLevelInterval(0)
		assert.Equal(t, 1000, result)
	})

	t.Run("正常な値はそのまま返す", func(t *testing.T) {
		result := session.normalizeAudioLevelInterval(100)
		assert.Equal(t, 100, result)
	})

	t.Run("50以下でも値は変わらない（警告のみ）", func(t *testing.T) {
		result := session.normalizeAudioLevelInterval(30)
		assert.Equal(t, 30, result)
	})
}

func TestSessionLocal_BuildAudioLevelMessage(t *testing.T) {
	bf := buffer.NewBufferFactory(500)
	cfg := WebRTCTransportConfig{BufferFactory: bf}
	session := NewSession("session-1", nil, cfg).(*sessionLocal)

	t.Run("正常なメッセージ構築", func(t *testing.T) {
		levels := []string{"stream-1", "stream-2"}

		msg, err := session.buildAudioLevelMessage(levels)

		require.NoError(t, err)
		assert.Contains(t, msg, `"method":"audioLevels"`)
		assert.Contains(t, msg, `"params":["stream-1","stream-2"]`)
	})

	t.Run("空のレベル", func(t *testing.T) {
		levels := []string{}

		msg, err := session.buildAudioLevelMessage(levels)

		require.NoError(t, err)
		assert.Contains(t, msg, `"method":"audioLevels"`)
		assert.Contains(t, msg, `"params":[]`)
	})
}

func TestSessionInterface(t *testing.T) {
	t.Run("sessionLocal型がSessionインターフェースを実装している", func(t *testing.T) {
		bf := buffer.NewBufferFactory(500)
		cfg := WebRTCTransportConfig{BufferFactory: bf}

		var _ Session = NewSession("session-1", nil, cfg)
	})
}

func TestSessionLocal_ConcurrentAccess(t *testing.T) {
	bf := buffer.NewBufferFactory(500)
	cfg := WebRTCTransportConfig{BufferFactory: bf}
	session := NewSession("session-1", nil, cfg).(*sessionLocal)

	var wg sync.WaitGroup

	// 並行してPeerを追加
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			peer := &mockPeer{userID: "user-" + string(rune('0'+id))}
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

// mockPeer はテスト用のPeerモック
type mockPeer struct {
	userID     string
	publisher  Publisher
	subscriber Subscriber
}

func (m *mockPeer) UserID() string         { return m.userID }
func (m *mockPeer) Publisher() Publisher   { return m.publisher }
func (m *mockPeer) Subscriber() Subscriber { return m.subscriber }
func (m *mockPeer) Join(ctx context.Context, sessionID, userID string, config JoinConfig) error {
	return nil
}
func (m *mockPeer) SetOnOffer(f func(*webrtc.SessionDescription))                      {}
func (m *mockPeer) SetOnIceCandidate(f func(*webrtc.ICECandidateInit, ConnectionType)) {}
func (m *mockPeer) SetOnIceConnectionStateChange(f func(webrtc.ICEConnectionState))    {}
func (m *mockPeer) SetRemoteDescription(sdp webrtc.SessionDescription) error           { return nil }
func (m *mockPeer) Trickle(candidate webrtc.ICECandidateInit, target ConnectionType) error {
	return nil
}
