package sfu_test

import (
	"sync"
	"testing"

	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/HMasataka/choice/pkg/sfu"
	mock_sfu "github.com/HMasataka/choice/pkg/sfu/mock"
	"github.com/pion/rtcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRouterConfig(t *testing.T) {
	t.Run("デフォルト値", func(t *testing.T) {
		cfg := sfu.RouterConfig{}

		assert.Equal(t, uint64(0), cfg.MaxBandwidth)
		assert.Equal(t, 0, cfg.MaxPacketTrack)
		assert.Equal(t, 0, cfg.AudioLevelInterval)
		assert.Equal(t, uint8(0), cfg.AudioLevelThreshold)
		assert.Equal(t, 0, cfg.AudioLevelFilter)
		assert.False(t, cfg.Simulcast.BestQualityFirst)
		assert.False(t, cfg.AllowSelfSubscribe)
	})

	t.Run("値を設定", func(t *testing.T) {
		cfg := sfu.RouterConfig{
			MaxBandwidth:        5000,
			MaxPacketTrack:      1000,
			AudioLevelInterval:  100,
			AudioLevelThreshold: 40,
			AudioLevelFilter:    50,
			Simulcast: sfu.SimulcastConfig{
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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("正常に初期化される", func(t *testing.T) {
		session := mock_sfu.NewMockSession(ctrl)
		session.EXPECT().AudioObserver().Return(sfu.NewAudioObserver(50, 100, 50)).AnyTimes()
		bf := buffer.NewBufferFactory(500)
		cfg := &sfu.WebRTCTransportConfig{
			BufferFactory: bf,
			RouterConfig: sfu.RouterConfig{
				MaxBandwidth:   5000,
				MaxPacketTrack: 1000,
			},
		}

		r := sfu.NewRouter("user-123", session, cfg)

		require.NotNil(t, r)
		assert.Equal(t, "user-123", r.UserID())
	})

	t.Run("異なるユーザーIDで作成", func(t *testing.T) {
		session := mock_sfu.NewMockSession(ctrl)
		session.EXPECT().AudioObserver().Return(sfu.NewAudioObserver(50, 100, 50)).AnyTimes()
		bf := buffer.NewBufferFactory(500)
		cfg := &sfu.WebRTCTransportConfig{BufferFactory: bf}

		r1 := sfu.NewRouter("user-1", session, cfg)
		r2 := sfu.NewRouter("user-2", session, cfg)

		assert.Equal(t, "user-1", r1.UserID())
		assert.Equal(t, "user-2", r2.UserID())
		assert.NotSame(t, r1, r2)
	})
}

func TestRouter_UserID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	session := mock_sfu.NewMockSession(ctrl)
	session.EXPECT().AudioObserver().Return(sfu.NewAudioObserver(50, 100, 50)).AnyTimes()
	bf := buffer.NewBufferFactory(500)
	cfg := &sfu.WebRTCTransportConfig{BufferFactory: bf}

	r := sfu.NewRouter("test-user-id", session, cfg)

	assert.Equal(t, "test-user-id", r.UserID())
}

func TestRouter_GetReceiver(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("空のreceivers", func(t *testing.T) {
		session := mock_sfu.NewMockSession(ctrl)
		session.EXPECT().AudioObserver().Return(sfu.NewAudioObserver(50, 100, 50)).AnyTimes()
		bf := buffer.NewBufferFactory(500)
		cfg := &sfu.WebRTCTransportConfig{BufferFactory: bf}

		r := sfu.NewRouter("user-1", session, cfg)

		receivers := r.GetReceiver()

		assert.NotNil(t, receivers)
		assert.Empty(t, receivers)
	})
}

func TestRouter_OnAddReceiverTrack(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	session := mock_sfu.NewMockSession(ctrl)
	session.EXPECT().AudioObserver().Return(sfu.NewAudioObserver(50, 100, 50)).AnyTimes()
	bf := buffer.NewBufferFactory(500)
	cfg := &sfu.WebRTCTransportConfig{BufferFactory: bf}
	r := sfu.NewRouter("user-1", session, cfg)

	t.Run("コールバックを設定", func(t *testing.T) {
		// パニックしないことを確認
		r.OnAddReceiverTrack(func(receiver sfu.Receiver) {})
	})

	t.Run("nilでも設定可能", func(t *testing.T) {
		// パニックしないことを確認
		r.OnAddReceiverTrack(nil)
	})
}

func TestRouter_OnDelReceiverTrack(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	session := mock_sfu.NewMockSession(ctrl)
	session.EXPECT().AudioObserver().Return(sfu.NewAudioObserver(50, 100, 50)).AnyTimes()
	bf := buffer.NewBufferFactory(500)
	cfg := &sfu.WebRTCTransportConfig{BufferFactory: bf}
	r := sfu.NewRouter("user-1", session, cfg)

	t.Run("コールバックを設定", func(t *testing.T) {
		// パニックしないことを確認
		r.OnDelReceiverTrack(func(receiver sfu.Receiver) {})
	})
}

func TestRouter_SetRTCPWriter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	session := mock_sfu.NewMockSession(ctrl)
	session.EXPECT().AudioObserver().Return(sfu.NewAudioObserver(50, 100, 50)).AnyTimes()
	bf := buffer.NewBufferFactory(500)
	cfg := &sfu.WebRTCTransportConfig{BufferFactory: bf}
	r := sfu.NewRouter("user-1", session, cfg)

	t.Run("RTCPライターを設定", func(t *testing.T) {
		// パニックしないことを確認
		r.SetRTCPWriter(func(packets []rtcp.Packet) error {
			return nil
		})

		// Stopを呼んで終了
		r.Stop()
	})
}

func TestRouter_Stop(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	session := mock_sfu.NewMockSession(ctrl)
	session.EXPECT().AudioObserver().Return(sfu.NewAudioObserver(50, 100, 50)).AnyTimes()
	bf := buffer.NewBufferFactory(500)
	cfg := &sfu.WebRTCTransportConfig{BufferFactory: bf}
	r := sfu.NewRouter("user-1", session, cfg)

	t.Run("Stopでgoroutineが終了する", func(t *testing.T) {
		// SetRTCPWriterを呼んでgoroutineを起動してからStopを呼ぶ
		r.SetRTCPWriter(func(packets []rtcp.Packet) error {
			return nil
		})
		// パニックしないことを確認
		r.Stop()
	})
}

func TestRouterInterface(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("router型がRouterインターフェースを実装している", func(t *testing.T) {
		session := mock_sfu.NewMockSession(ctrl)
		session.EXPECT().AudioObserver().Return(sfu.NewAudioObserver(50, 100, 50)).AnyTimes()
		bf := buffer.NewBufferFactory(500)
		cfg := &sfu.WebRTCTransportConfig{BufferFactory: bf}

		var _ sfu.Router = sfu.NewRouter("user-1", session, cfg)
	})
}

func TestRouter_ConcurrentAccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	session := mock_sfu.NewMockSession(ctrl)
	session.EXPECT().AudioObserver().Return(sfu.NewAudioObserver(50, 100, 50)).AnyTimes()
	bf := buffer.NewBufferFactory(500)
	cfg := &sfu.WebRTCTransportConfig{BufferFactory: bf}
	r := sfu.NewRouter("user-1", session, cfg)

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
			r.OnAddReceiverTrack(func(receiver sfu.Receiver) {})
		}()
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.OnDelReceiverTrack(func(receiver sfu.Receiver) {})
		}()
	}

	wg.Wait()
	// パニックなく完了すればテスト成功
}
