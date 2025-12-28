package sfu

import (
	"sync"
	"testing"

	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSession はテスト用のSessionモック
type mockSession struct {
	id            string
	audioObserver *AudioObserver
	peers         []Peer
}

func newMockSession() *mockSession {
	return &mockSession{
		id:            "test-session",
		audioObserver: NewAudioObserver(50, 100, 50),
		peers:         []Peer{},
	}
}

func (m *mockSession) ID() string                        { return m.id }
func (m *mockSession) Publish(router Router, r Receiver) {}
func (m *mockSession) Subscribe(peer Peer)               {}
func (m *mockSession) AddPeer(peer Peer)                 { m.peers = append(m.peers, peer) }
func (m *mockSession) GetPeer(peerID string) Peer        { return nil }
func (m *mockSession) RemovePeer(peer Peer)              {}
func (m *mockSession) AddRelayPeer(peerID string, signalData []byte) ([]byte, error) {
	return nil, nil
}
func (m *mockSession) AudioObserver() *AudioObserver                              { return m.audioObserver }
func (m *mockSession) AddDatachannel(owner string, dc *webrtc.DataChannel)        {}
func (m *mockSession) GetDCMiddlewares() []*Datachannel                           { return nil }
func (m *mockSession) GetFanOutDataChannelLabels() []string                       { return nil }
func (m *mockSession) GetDataChannels(peerID, label string) []*webrtc.DataChannel { return nil }
func (m *mockSession) FanOutMessage(origin, label string, msg webrtc.DataChannelMessage) {
}
func (m *mockSession) Peers() []Peer            { return m.peers }
func (m *mockSession) RelayPeers() []*RelayPeer { return nil }
func (m *mockSession) OnClose(f func())         {}

func TestRouterConfig(t *testing.T) {
	t.Run("デフォルト値", func(t *testing.T) {
		cfg := RouterConfig{}

		assert.Equal(t, uint64(0), cfg.MaxBandwidth)
		assert.Equal(t, 0, cfg.MaxPacketTrack)
		assert.Equal(t, 0, cfg.AudioLevelInterval)
		assert.Equal(t, uint8(0), cfg.AudioLevelThreshold)
		assert.Equal(t, 0, cfg.AudioLevelFilter)
		assert.False(t, cfg.Simulcast.BestQualityFirst)
		assert.False(t, cfg.AllowSelfSubscribe)
	})

	t.Run("値を設定", func(t *testing.T) {
		cfg := RouterConfig{
			MaxBandwidth:        5000,
			MaxPacketTrack:      1000,
			AudioLevelInterval:  100,
			AudioLevelThreshold: 40,
			AudioLevelFilter:    50,
			Simulcast: SimulcastConfig{
				BestQualityFirst:    true,
				EnableTemporalLayer: true,
			},
			AllowSelfSubscribe: true,
		}

		assert.Equal(t, uint64(5000), cfg.MaxBandwidth)
		assert.Equal(t, 1000, cfg.MaxPacketTrack)
		assert.Equal(t, 100, cfg.AudioLevelInterval)
		assert.Equal(t, uint8(40), cfg.AudioLevelThreshold)
		assert.Equal(t, 50, cfg.AudioLevelFilter)
		assert.True(t, cfg.Simulcast.BestQualityFirst)
		assert.True(t, cfg.Simulcast.EnableTemporalLayer)
		assert.True(t, cfg.AllowSelfSubscribe)
	})
}

func TestNewRouter(t *testing.T) {
	t.Run("正常に初期化される", func(t *testing.T) {
		session := newMockSession()
		bf := buffer.NewBufferFactory(500)
		cfg := &WebRTCTransportConfig{
			BufferFactory: bf,
			RouterConfig: RouterConfig{
				MaxBandwidth:   5000,
				MaxPacketTrack: 1000,
			},
		}

		r := NewRouter("user-123", session, cfg)

		require.NotNil(t, r)
		assert.Equal(t, "user-123", r.userID)
		assert.NotNil(t, r.rtcpCh)
		assert.NotNil(t, r.stopCh)
		assert.NotNil(t, r.receivers)
		assert.Empty(t, r.receivers)
		assert.Equal(t, bf, r.bufferFactory)
		assert.Equal(t, uint64(5000), r.config.MaxBandwidth)
		assert.Equal(t, session, r.session)
	})

	t.Run("異なるユーザーIDで作成", func(t *testing.T) {
		session := newMockSession()
		bf := buffer.NewBufferFactory(500)
		cfg := &WebRTCTransportConfig{BufferFactory: bf}

		r1 := NewRouter("user-1", session, cfg)
		r2 := NewRouter("user-2", session, cfg)

		assert.Equal(t, "user-1", r1.userID)
		assert.Equal(t, "user-2", r2.userID)
		assert.NotSame(t, r1, r2)
	})
}

func TestRouter_UserID(t *testing.T) {
	session := newMockSession()
	bf := buffer.NewBufferFactory(500)
	cfg := &WebRTCTransportConfig{BufferFactory: bf}

	r := NewRouter("test-user-id", session, cfg)

	assert.Equal(t, "test-user-id", r.UserID())
}

func TestRouter_GetReceiver(t *testing.T) {
	t.Run("空のreceivers", func(t *testing.T) {
		session := newMockSession()
		bf := buffer.NewBufferFactory(500)
		cfg := &WebRTCTransportConfig{BufferFactory: bf}

		r := NewRouter("user-1", session, cfg)

		receivers := r.GetReceiver()

		assert.NotNil(t, receivers)
		assert.Empty(t, receivers)
	})
}

func TestRouter_OnAddReceiverTrack(t *testing.T) {
	session := newMockSession()
	bf := buffer.NewBufferFactory(500)
	cfg := &WebRTCTransportConfig{BufferFactory: bf}
	r := NewRouter("user-1", session, cfg)

	t.Run("コールバックを設定", func(t *testing.T) {
		r.OnAddReceiverTrack(func(receiver Receiver) {})

		// コールバックが設定されていることを確認
		handler, ok := r.onAddTrack.Load().(func(Receiver))
		assert.True(t, ok)
		assert.NotNil(t, handler)
	})

	t.Run("nilでも設定可能", func(t *testing.T) {
		r.OnAddReceiverTrack(nil)

		handler := r.onAddTrack.Load()
		assert.Nil(t, handler)
	})
}

func TestRouter_OnDelReceiverTrack(t *testing.T) {
	session := newMockSession()
	bf := buffer.NewBufferFactory(500)
	cfg := &WebRTCTransportConfig{BufferFactory: bf}
	r := NewRouter("user-1", session, cfg)

	t.Run("コールバックを設定", func(t *testing.T) {
		r.OnDelReceiverTrack(func(receiver Receiver) {})

		handler, ok := r.onDelTrack.Load().(func(Receiver))
		assert.True(t, ok)
		assert.NotNil(t, handler)
	})
}

func TestRouter_SetRTCPWriter(t *testing.T) {
	session := newMockSession()
	bf := buffer.NewBufferFactory(500)
	cfg := &WebRTCTransportConfig{BufferFactory: bf}
	r := NewRouter("user-1", session, cfg)

	t.Run("RTCPライターを設定", func(t *testing.T) {
		writeCount := 0
		var mu sync.Mutex

		r.SetRTCPWriter(func(packets []rtcp.Packet) error {
			mu.Lock()
			writeCount++
			mu.Unlock()
			return nil
		})

		assert.NotNil(t, r.writeRTCP)

		// RTCPチャネルにパケットを送信
		r.rtcpCh <- []rtcp.Packet{}

		// goroutineが処理するのを待つ
		// Stopを呼んで終了
		r.Stop()
	})
}

func TestRouter_Stop(t *testing.T) {
	session := newMockSession()
	bf := buffer.NewBufferFactory(500)
	cfg := &WebRTCTransportConfig{BufferFactory: bf}
	r := NewRouter("user-1", session, cfg)

	t.Run("Stopでgoroutineが終了する", func(t *testing.T) {
		stopped := make(chan struct{})

		r.SetRTCPWriter(func(packets []rtcp.Packet) error {
			return nil
		})

		go func() {
			<-r.stopCh
			close(stopped)
		}()

		r.Stop()

		// stopChにシグナルが送られたことを確認
		<-stopped
	})
}

func TestRouter_DeleteReceiver(t *testing.T) {
	session := newMockSession()
	bf := buffer.NewBufferFactory(500)
	cfg := &WebRTCTransportConfig{BufferFactory: bf}
	r := NewRouter("user-1", session, cfg)

	t.Run("receiversから削除される", func(t *testing.T) {
		// 手動でreceiverを追加
		r.receivers["track-1"] = nil

		assert.Len(t, r.receivers, 1)

		r.deleteReceiver("track-1", 12345)

		assert.Empty(t, r.receivers)
	})

	t.Run("OnDelTrackコールバックが呼ばれる", func(t *testing.T) {
		called := false
		r.OnDelReceiverTrack(func(receiver Receiver) {
			called = true
		})

		r.receivers["track-2"] = nil

		r.deleteReceiver("track-2", 12345)

		assert.True(t, called)
	})

	t.Run("存在しないトラックの削除", func(t *testing.T) {
		// パニックしないことを確認
		r.deleteReceiver("nonexistent", 12345)

		assert.Empty(t, r.receivers)
	})
}

func TestRouterInterface(t *testing.T) {
	t.Run("router型がRouterインターフェースを実装している", func(t *testing.T) {
		session := newMockSession()
		bf := buffer.NewBufferFactory(500)
		cfg := &WebRTCTransportConfig{BufferFactory: bf}

		var _ Router = NewRouter("user-1", session, cfg)
	})
}

func TestRouter_ConcurrentAccess(t *testing.T) {
	session := newMockSession()
	bf := buffer.NewBufferFactory(500)
	cfg := &WebRTCTransportConfig{BufferFactory: bf}
	r := NewRouter("user-1", session, cfg)

	var wg sync.WaitGroup

	// 並行してreceiversにアクセス
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.GetReceiver()
		}()
	}

	// 並行してコールバックを設定
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.OnAddReceiverTrack(func(receiver Receiver) {})
		}()
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.OnDelReceiverTrack(func(receiver Receiver) {})
		}()
	}

	wg.Wait()
	// パニックなく完了すればテスト成功
}

func TestRouter_RTCPChannel(t *testing.T) {
	session := newMockSession()
	bf := buffer.NewBufferFactory(500)
	cfg := &WebRTCTransportConfig{BufferFactory: bf}
	r := NewRouter("user-1", session, cfg)

	t.Run("RTCPチャネルのバッファサイズ", func(t *testing.T) {
		// rtcpChはバッファサイズ10で作成される
		assert.Equal(t, 10, cap(r.rtcpCh))
	})

	t.Run("RTCPチャネルへの送信", func(t *testing.T) {
		// チャネルに送信できることを確認
		select {
		case r.rtcpCh <- []rtcp.Packet{}:
			// 送信成功
		default:
			t.Error("Failed to send to rtcpCh")
		}

		// チャネルから受信
		<-r.rtcpCh
	})
}
