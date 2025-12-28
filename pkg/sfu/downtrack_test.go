package sfu

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownTrackInterface(t *testing.T) {
	t.Run("downTrack型がDownTrackインターフェースを実装している", func(t *testing.T) {
		// コンパイル時チェック
		var _ DownTrack = (*downTrack)(nil)
	})

	t.Run("downTrack型がwebrtc.TrackLocalインターフェースを実装している", func(t *testing.T) {
		// コンパイル時チェック
		var _ webrtc.TrackLocal = (*downTrack)(nil)
	})
}

func TestDownTrackType(t *testing.T) {
	t.Run("定数の値", func(t *testing.T) {
		assert.Equal(t, DownTrackType(1), SimpleDownTrack)
		assert.Equal(t, DownTrackType(2), SimulcastDownTrack)
	})
}

func TestNewDownTrack(t *testing.T) {
	t.Run("正常に初期化される", func(t *testing.T) {
		receiver := &mockReceiverForDownTrack{
			trackID:  "test-track",
			streamID: "test-stream",
		}
		codec := webrtc.RTPCodecCapability{
			MimeType:  "video/VP8",
			ClockRate: 90000,
		}
		bf := buffer.NewBufferFactory(500)

		dt, err := NewDownTrack(codec, receiver, bf, "peer-123", 100)

		require.NoError(t, err)
		require.NotNil(t, dt)
		assert.Equal(t, "test-track", dt.ID())
		assert.Equal(t, "test-stream", dt.StreamID())
		assert.Equal(t, "peer-123", dt.peerID)
		assert.Equal(t, 100, dt.maxTrack)
		assert.Equal(t, codec, dt.codec)
	})
}

func TestDownTrack_ID(t *testing.T) {
	dt := &downTrack{id: "track-123"}

	assert.Equal(t, "track-123", dt.ID())
}

func TestDownTrack_StreamID(t *testing.T) {
	dt := &downTrack{streamID: "stream-456"}

	assert.Equal(t, "stream-456", dt.StreamID())
}

func TestDownTrack_RID(t *testing.T) {
	dt := &downTrack{}

	// RIDは常に空文字を返す
	assert.Equal(t, "", dt.RID())
}

func TestDownTrack_Codec(t *testing.T) {
	codec := webrtc.RTPCodecCapability{
		MimeType:  "video/VP8",
		ClockRate: 90000,
	}
	dt := &downTrack{codec: codec}

	result := dt.Codec()

	assert.Equal(t, "video/VP8", result.MimeType)
	assert.Equal(t, uint32(90000), result.ClockRate)
}

func TestDownTrack_Kind(t *testing.T) {
	t.Run("オーディオ", func(t *testing.T) {
		dt := &downTrack{
			codec: webrtc.RTPCodecCapability{MimeType: "audio/opus"},
		}

		assert.Equal(t, webrtc.RTPCodecTypeAudio, dt.Kind())
	})

	t.Run("ビデオ", func(t *testing.T) {
		dt := &downTrack{
			codec: webrtc.RTPCodecCapability{MimeType: "video/VP8"},
		}

		assert.Equal(t, webrtc.RTPCodecTypeVideo, dt.Kind())
	})

	t.Run("不明", func(t *testing.T) {
		dt := &downTrack{
			codec: webrtc.RTPCodecCapability{MimeType: "unknown/type"},
		}

		assert.Equal(t, webrtc.RTPCodecType(0), dt.Kind())
	})
}

func TestDownTrack_Enabled(t *testing.T) {
	t.Run("初期値はfalse", func(t *testing.T) {
		dt := &downTrack{}

		assert.False(t, dt.Enabled())
	})

	t.Run("trueに設定", func(t *testing.T) {
		dt := &downTrack{}
		dt.enabled.Store(true)

		assert.True(t, dt.Enabled())
	})
}

func TestDownTrack_Bound(t *testing.T) {
	t.Run("初期値はfalse", func(t *testing.T) {
		dt := &downTrack{}

		assert.False(t, dt.Bound())
	})

	t.Run("バインド後はtrue", func(t *testing.T) {
		dt := &downTrack{}
		dt.bound.Store(true)

		assert.True(t, dt.Bound())
	})
}

func TestDownTrack_Mute(t *testing.T) {
	t.Run("有効状態からミュート", func(t *testing.T) {
		dt := &downTrack{}
		dt.enabled.Store(true)

		dt.Mute(true)

		assert.False(t, dt.Enabled())
		assert.True(t, dt.reSync.Load())
	})

	t.Run("無効状態からアンミュート", func(t *testing.T) {
		dt := &downTrack{}
		dt.enabled.Store(false)

		dt.Mute(false)

		assert.True(t, dt.Enabled())
	})

	t.Run("既にミュート済みの場合は変更なし", func(t *testing.T) {
		dt := &downTrack{}
		dt.enabled.Store(false)

		dt.Mute(true)

		// 値が同じなら変更されない
		assert.False(t, dt.Enabled())
	})
}

func TestDownTrack_SetTransceiver(t *testing.T) {
	dt := &downTrack{}

	// nilでも設定可能
	dt.SetTransceiver(nil)

	assert.Nil(t, dt.transceiver)
}

func TestDownTrack_SetInitialLayers(t *testing.T) {
	dt := &downTrack{}

	dt.SetInitialLayers(2, 1)

	assert.Equal(t, int32(2), atomic.LoadInt32(&dt.currentSpatialLayer))
	assert.Equal(t, int32(2), atomic.LoadInt32(&dt.targetSpatialLayer))
}

func TestDownTrack_CurrentSpatialLayer(t *testing.T) {
	dt := &downTrack{}
	atomic.StoreInt32(&dt.currentSpatialLayer, 1)

	assert.Equal(t, 1, dt.CurrentSpatialLayer())
}

func TestDownTrack_SwitchSpatialLayerDone(t *testing.T) {
	dt := &downTrack{
		snOffset: 100,
		tsOffset: 1000,
	}

	dt.SwitchSpatialLayerDone(2)

	assert.Equal(t, int32(2), atomic.LoadInt32(&dt.currentSpatialLayer))
	assert.Equal(t, uint16(0), dt.snOffset)
	assert.Equal(t, uint32(0), dt.tsOffset)
}

func TestDownTrack_OnCloseHandler(t *testing.T) {
	dt := &downTrack{}

	called := false
	dt.OnCloseHandler(func() {
		called = true
	})

	assert.NotNil(t, dt.onCloseHandler)

	// ハンドラを呼び出し
	dt.onCloseHandler()
	assert.True(t, called)
}

func TestDownTrack_OnBind(t *testing.T) {
	dt := &downTrack{}

	called := false
	dt.OnBind(func() {
		called = true
	})

	assert.NotNil(t, dt.onBind)

	// ハンドラを呼び出し
	dt.onBind()
	assert.True(t, called)
}

func TestDownTrack_SetMaxSpatialLayer(t *testing.T) {
	dt := &downTrack{}

	dt.SetMaxSpatialLayer(2)

	assert.Equal(t, int32(2), atomic.LoadInt32(&dt.maxSpatialLayer))
}

func TestDownTrack_SetMaxTemporalLayer(t *testing.T) {
	dt := &downTrack{}

	dt.SetMaxTemporalLayer(2)

	assert.Equal(t, int32(2), atomic.LoadInt32(&dt.maxTemporalLayer))
}

func TestDownTrack_SetLastSSRC(t *testing.T) {
	dt := &downTrack{}

	dt.SetLastSSRC(12345)

	assert.Equal(t, uint32(12345), atomic.LoadUint32(&dt.lastSSRC))
}

func TestDownTrack_SetTrackType(t *testing.T) {
	dt := &downTrack{}

	dt.SetTrackType(SimulcastDownTrack)

	assert.Equal(t, SimulcastDownTrack, dt.trackType)
}

func TestDownTrack_GetMime(t *testing.T) {
	dt := &downTrack{mime: "video/vp8"}

	assert.Equal(t, "video/vp8", dt.GetMime())
}

func TestDownTrack_GetPayloadType(t *testing.T) {
	dt := &downTrack{payloadType: 96}

	assert.Equal(t, uint8(96), dt.GetPayloadType())
}

func TestDownTrack_GetSSRC(t *testing.T) {
	dt := &downTrack{ssrc: 12345}

	assert.Equal(t, uint32(12345), dt.GetSSRC())
}

func TestDownTrack_GetWriteStream(t *testing.T) {
	dt := &downTrack{writeStream: nil}

	assert.Nil(t, dt.GetWriteStream())
}

func TestDownTrack_SetPayload(t *testing.T) {
	dt := &downTrack{}
	payload := make([]byte, 100)

	dt.SetPayload(&payload)

	assert.NotNil(t, dt.payload)
}

func TestDownTrack_GetSimulcast(t *testing.T) {
	dt := &downTrack{
		simulcast: simulcastTrackHelpers{
			temporalSupported: true,
		},
	}

	result := dt.GetSimulcast()

	assert.True(t, result.temporalSupported)
}

func TestDownTrack_UpdateStats(t *testing.T) {
	dt := &downTrack{}

	dt.UpdateStats(100)
	dt.UpdateStats(200)

	assert.Equal(t, uint32(300), atomic.LoadUint32(&dt.octetCount))
	assert.Equal(t, uint32(2), atomic.LoadUint32(&dt.packetCount))
}

func TestDownTrack_CreateSourceDescriptionChunks(t *testing.T) {
	t.Run("バインドされていない場合はnil", func(t *testing.T) {
		dt := &downTrack{}
		dt.bound.Store(false)

		result := dt.CreateSourceDescriptionChunks()

		assert.Nil(t, result)
	})
}

func TestDownTrack_CreateSenderReport(t *testing.T) {
	t.Run("バインドされていない場合はnil", func(t *testing.T) {
		dt := &downTrack{}
		dt.bound.Store(false)

		result := dt.CreateSenderReport()

		assert.Nil(t, result)
	})
}

func TestDownTrack_SwitchSpatialLayer(t *testing.T) {
	t.Run("SimpleDownTrackの場合はエラー", func(t *testing.T) {
		dt := &downTrack{trackType: SimpleDownTrack}

		err := dt.SwitchSpatialLayer(1, false)

		assert.ErrorIs(t, err, ErrSpatialNotSupported)
	})

	t.Run("SimulcastDownTrackで前の切り替えが未完了の場合はエラー", func(t *testing.T) {
		dt := &downTrack{trackType: SimulcastDownTrack}
		atomic.StoreInt32(&dt.currentSpatialLayer, 0)
		atomic.StoreInt32(&dt.targetSpatialLayer, 1) // 前回の切り替えが未完了

		err := dt.SwitchSpatialLayer(2, false)

		assert.ErrorIs(t, err, ErrSpatialLayerBusy)
	})

	t.Run("同じレイヤーへの切り替えはエラー", func(t *testing.T) {
		dt := &downTrack{trackType: SimulcastDownTrack}
		atomic.StoreInt32(&dt.currentSpatialLayer, 1)
		atomic.StoreInt32(&dt.targetSpatialLayer, 1)

		err := dt.SwitchSpatialLayer(1, false)

		assert.ErrorIs(t, err, ErrSpatialLayerBusy)
	})
}

func TestDownTrack_SwitchTemporalLayer(t *testing.T) {
	t.Run("SimpleDownTrackの場合は何もしない", func(t *testing.T) {
		dt := &downTrack{trackType: SimpleDownTrack}

		// パニックしないことを確認
		dt.SwitchTemporalLayer(1, false)
	})

	t.Run("SimulcastDownTrackの場合", func(t *testing.T) {
		dt := &downTrack{trackType: SimulcastDownTrack}
		// 現在のレイヤーとターゲットレイヤーが同じ状態に設定
		atomic.StoreInt32(&dt.temporalLayer, 0) // current=0, target=0

		dt.SwitchTemporalLayer(2, true)

		// maxTemporalLayerが設定される
		assert.Equal(t, int32(2), atomic.LoadInt32(&dt.maxTemporalLayer))
	})
}

func TestDownTrack_UptrackLayersChange(t *testing.T) {
	t.Run("SimpleDownTrackの場合はエラー", func(t *testing.T) {
		dt := &downTrack{trackType: SimpleDownTrack, id: "test"}

		_, err := dt.UptrackLayersChange([]uint16{0, 1, 2})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not support simulcast")
	})
}

func TestDownTrack_GetTargetLayer(t *testing.T) {
	t.Run("有効なレイヤーがある場合", func(t *testing.T) {
		dt := &downTrack{}
		atomic.StoreInt32(&dt.maxSpatialLayer, 2)

		layer, err := dt.getTargetLayer([]uint16{0, 1, 2})

		require.NoError(t, err)
		assert.Equal(t, uint16(2), layer)
	})

	t.Run("maxLayer以下のレイヤーのみ", func(t *testing.T) {
		dt := &downTrack{}
		atomic.StoreInt32(&dt.maxSpatialLayer, 1)

		layer, err := dt.getTargetLayer([]uint16{0, 1, 2})

		require.NoError(t, err)
		assert.Equal(t, uint16(1), layer)
	})

	t.Run("maxLayerを超えるレイヤーのみの場合", func(t *testing.T) {
		dt := &downTrack{}
		atomic.StoreInt32(&dt.maxSpatialLayer, 0)

		layer, err := dt.getTargetLayer([]uint16{1, 2})

		require.NoError(t, err)
		assert.Equal(t, uint16(1), layer) // 最小値
	})

	t.Run("空のレイヤーリストの場合はエラー", func(t *testing.T) {
		dt := &downTrack{}

		_, err := dt.getTargetLayer([]uint16{})

		assert.Error(t, err)
	})
}

func TestDownTrack_WriteRTP(t *testing.T) {
	t.Run("無効な場合は何もしない", func(t *testing.T) {
		dt := &downTrack{}
		dt.enabled.Store(false)
		dt.bound.Store(true)

		err := dt.WriteRTP(nil, 0)

		assert.NoError(t, err)
	})

	t.Run("バインドされていない場合は何もしない", func(t *testing.T) {
		dt := &downTrack{}
		dt.enabled.Store(true)
		dt.bound.Store(false)

		err := dt.WriteRTP(nil, 0)

		assert.NoError(t, err)
	})
}

func TestDownTrack_Stop(t *testing.T) {
	t.Run("transceiverがnilの場合はエラー", func(t *testing.T) {
		dt := &downTrack{transceiver: nil}

		err := dt.Stop()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "transceiver not exists")
	})
}

func TestDownTrack_Unbind(t *testing.T) {
	t.Run("正常にアンバインド", func(t *testing.T) {
		dt := &downTrack{}
		dt.bound.Store(true)

		err := dt.Unbind(nil)

		assert.NoError(t, err)
		assert.False(t, dt.Bound())
	})
}

func TestDownTrack_Close(t *testing.T) {
	t.Run("onCloseHandlerが呼ばれる", func(t *testing.T) {
		dt := &downTrack{}
		called := false
		dt.OnCloseHandler(func() {
			called = true
		})

		dt.Close()

		assert.True(t, called)
	})

	t.Run("複数回呼んでも一度だけ実行", func(t *testing.T) {
		dt := &downTrack{}
		count := 0
		dt.OnCloseHandler(func() {
			count++
		})

		dt.Close()
		dt.Close()
		dt.Close()

		assert.Equal(t, 1, count)
	})
}

func TestDownTrack_ConcurrentAccess(t *testing.T) {
	dt := &downTrack{}

	var wg sync.WaitGroup

	// 並行してEnabledを読み書き
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dt.enabled.Store(true)
			_ = dt.Enabled()
		}()
	}

	// 並行してUpdateStatsを呼び出し
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dt.UpdateStats(100)
		}()
	}

	// 並行してCurrentSpatialLayerを読み取り
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = dt.CurrentSpatialLayer()
		}()
	}

	wg.Wait()
	// パニックなく完了すればテスト成功
}

func TestDownTrack_CalculateAdjustedRTPTime(t *testing.T) {
	t.Run("正の時間差", func(t *testing.T) {
		dt := &downTrack{
			codec: webrtc.RTPCodecCapability{ClockRate: 90000},
		}

		// この関数はsrNTPからsrTimeを計算し、現在時刻との差分を計算する
		// テストは関数の存在確認のみ
		_ = dt.calculateAdjustedRTPTime(0, 0, time.Now())
	})
}

// mockReceiverForDownTrack はテスト用のReceiverモック
type mockReceiverForDownTrack struct {
	trackID  string
	streamID string
}

func (m *mockReceiverForDownTrack) TrackID() string                       { return m.trackID }
func (m *mockReceiverForDownTrack) StreamID() string                      { return m.streamID }
func (m *mockReceiverForDownTrack) Codec() webrtc.RTPCodecParameters      { return webrtc.RTPCodecParameters{} }
func (m *mockReceiverForDownTrack) Kind() webrtc.RTPCodecType             { return webrtc.RTPCodecTypeVideo }
func (m *mockReceiverForDownTrack) SSRC(layer int) uint32                 { return 0 }
func (m *mockReceiverForDownTrack) SetTrackMeta(trackID, streamID string) {}
func (m *mockReceiverForDownTrack) AddUpTrack(track *webrtc.TrackRemote, buffer *buffer.Buffer, bestQualityFirst bool) {
}
func (m *mockReceiverForDownTrack) AddDownTrack(track DownTrack, bestQualityFirst bool) {}
func (m *mockReceiverForDownTrack) SwitchDownTrack(track DownTrack, layer int) error    { return nil }
func (m *mockReceiverForDownTrack) GetBitrate() [3]uint64                               { return [3]uint64{} }
func (m *mockReceiverForDownTrack) GetMaxTemporalLayer() [3]int32                       { return [3]int32{} }
func (m *mockReceiverForDownTrack) RetransmitPackets(track DownTrack, packets []packetMeta) error {
	return nil
}
func (m *mockReceiverForDownTrack) DeleteDownTrack(layer int, id string)           {}
func (m *mockReceiverForDownTrack) OnCloseHandler(fn func())                        {}
func (m *mockReceiverForDownTrack) SendRTCP(p []rtcp.Packet)                        {}
func (m *mockReceiverForDownTrack) SetRTCPCh(ch chan []rtcp.Packet)                 {}
func (m *mockReceiverForDownTrack) GetSenderReportTime(layer int) (uint32, uint64) { return 0, 0 }
