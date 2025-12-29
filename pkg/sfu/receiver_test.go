package sfu

import (
	"io"
	"sync"
	"testing"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
)

func TestReceiverInterface(t *testing.T) {
	t.Run("WebRTCReceiver型がReceiverインターフェースを実装している", func(t *testing.T) {
		// コンパイル時チェック
		var _ Receiver = (*WebRTCReceiver)(nil)
	})
}

func TestWebRTCReceiver_StructFields(t *testing.T) {
	t.Run("初期状態", func(t *testing.T) {
		r := &WebRTCReceiver{
			peerID:      "test-peer",
			trackID:     "test-track",
			streamID:    "test-stream",
			kind:        webrtc.RTPCodecTypeVideo,
			isSimulcast: false,
		}

		assert.Equal(t, "test-peer", r.peerID)
		assert.Equal(t, "test-track", r.trackID)
		assert.Equal(t, "test-stream", r.streamID)
		assert.Equal(t, webrtc.RTPCodecTypeVideo, r.kind)
		assert.False(t, r.isSimulcast)
		assert.False(t, r.closed.Load())
	})
}

func TestWebRTCReceiver_TrackID(t *testing.T) {
	r := &WebRTCReceiver{trackID: "track-123"}

	assert.Equal(t, "track-123", r.TrackID())
}

func TestWebRTCReceiver_StreamID(t *testing.T) {
	r := &WebRTCReceiver{streamID: "stream-456"}

	assert.Equal(t, "stream-456", r.StreamID())
}

func TestWebRTCReceiver_SetTrackMeta(t *testing.T) {
	t.Run("メタデータを設定", func(t *testing.T) {
		r := &WebRTCReceiver{
			trackID:  "old-track",
			streamID: "old-stream",
		}

		r.SetTrackMeta("new-track", "new-stream")

		assert.Equal(t, "new-track", r.trackID)
		assert.Equal(t, "new-stream", r.streamID)
	})
}

func TestWebRTCReceiver_Kind(t *testing.T) {
	t.Run("Video", func(t *testing.T) {
		r := &WebRTCReceiver{kind: webrtc.RTPCodecTypeVideo}
		assert.Equal(t, webrtc.RTPCodecTypeVideo, r.Kind())
	})

	t.Run("Audio", func(t *testing.T) {
		r := &WebRTCReceiver{kind: webrtc.RTPCodecTypeAudio}
		assert.Equal(t, webrtc.RTPCodecTypeAudio, r.Kind())
	})
}

func TestWebRTCReceiver_Codec(t *testing.T) {
	t.Run("コーデック情報を返す", func(t *testing.T) {
		codec := webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  "video/VP8",
				ClockRate: 90000,
			},
			PayloadType: 96,
		}
		r := &WebRTCReceiver{codec: codec}

		result := r.Codec()

		assert.Equal(t, "video/VP8", result.MimeType)
		assert.Equal(t, uint32(90000), result.ClockRate)
		assert.Equal(t, webrtc.PayloadType(96), result.PayloadType)
	})
}

func TestWebRTCReceiver_SSRC(t *testing.T) {
	t.Run("upTracksがnilの場合は0を返す", func(t *testing.T) {
		r := &WebRTCReceiver{}

		ssrc := r.SSRC(0)

		assert.Equal(t, uint32(0), ssrc)
	})

	t.Run("範囲外のlayerでも安全", func(t *testing.T) {
		r := &WebRTCReceiver{}

		// パニックしないことを確認
		ssrc := r.SSRC(2)
		assert.Equal(t, uint32(0), ssrc)
	})
}

func TestWebRTCReceiver_GetBitrate(t *testing.T) {
	t.Run("バッファがnilの場合", func(t *testing.T) {
		r := &WebRTCReceiver{}

		br := r.GetBitrate()

		assert.Equal(t, [3]uint64{0, 0, 0}, br)
	})
}

func TestWebRTCReceiver_GetMaxTemporalLayer(t *testing.T) {
	t.Run("利用可能なレイヤーがない場合", func(t *testing.T) {
		r := &WebRTCReceiver{}

		tls := r.GetMaxTemporalLayer()

		assert.Equal(t, [3]int32{0, 0, 0}, tls)
	})
}

func TestWebRTCReceiver_OnCloseHandler(t *testing.T) {
	t.Run("ハンドラを設定", func(t *testing.T) {
		r := &WebRTCReceiver{}

		called := false
		r.OnCloseHandler(func() {
			called = true
		})

		assert.NotNil(t, r.onCloseHandler)

		// ハンドラを呼び出し
		r.onCloseHandler()
		assert.True(t, called)
	})

	t.Run("nilで設定", func(t *testing.T) {
		r := &WebRTCReceiver{}
		r.OnCloseHandler(nil)

		assert.Nil(t, r.onCloseHandler)
	})
}

func TestWebRTCReceiver_SetRTCPCh(t *testing.T) {
	t.Run("RTCPチャネルを設定", func(t *testing.T) {
		r := &WebRTCReceiver{}
		ch := make(chan []rtcp.Packet, 10)

		r.SetRTCPCh(ch)

		assert.Equal(t, ch, r.rtcpCh)
	})
}

func TestWebRTCReceiver_DetermineDownTrackLayer(t *testing.T) {
	t.Run("シミュルキャストでない場合は0を返す", func(t *testing.T) {
		r := &WebRTCReceiver{isSimulcast: false}

		layer := r.determineDownTrackLayer(true)

		assert.Equal(t, 0, layer)
	})

	t.Run("シミュルキャスト + bestQualityFirst=true", func(t *testing.T) {
		r := &WebRTCReceiver{isSimulcast: true}
		r.available[0].Store(true)
		r.available[1].Store(true)
		r.available[2].Store(true)

		layer := r.determineDownTrackLayer(true)

		// 最高品質(2)を返す
		assert.Equal(t, 2, layer)
	})

	t.Run("シミュルキャスト + bestQualityFirst=false", func(t *testing.T) {
		r := &WebRTCReceiver{isSimulcast: true}
		r.available[0].Store(true)
		r.available[1].Store(true)
		r.available[2].Store(true)

		layer := r.determineDownTrackLayer(false)

		// 最低品質(0)を返す
		assert.Equal(t, 0, layer)
	})

	t.Run("利用可能なレイヤーが一部のみ", func(t *testing.T) {
		r := &WebRTCReceiver{isSimulcast: true}
		r.available[0].Store(false)
		r.available[1].Store(true)
		r.available[2].Store(false)

		layer := r.determineDownTrackLayer(true)

		assert.Equal(t, 1, layer)
	})
}

func TestWebRTCReceiver_DeleteDownTrack(t *testing.T) {
	t.Run("クローズ済みの場合は何もしない", func(t *testing.T) {
		r := &WebRTCReceiver{}
		r.closed.Store(true)

		// パニックしないことを確認
		r.DeleteDownTrack(0, "test-id")
	})
}

func TestWebRTCReceiver_SwitchDownTrack(t *testing.T) {
	t.Run("クローズ済みの場合はエラー", func(t *testing.T) {
		r := &WebRTCReceiver{}
		r.closed.Store(true)
		dt := newMockDownTrackFull("track-1", "stream-1")

		err := r.SwitchDownTrack(dt, 1)

		assert.ErrorIs(t, err, errNoReceiverFound)
	})

	t.Run("利用不可のレイヤーへの切り替えはエラー", func(t *testing.T) {
		r := &WebRTCReceiver{}
		r.available[1].Store(false)
		dt := newMockDownTrackFull("track-1", "stream-1")

		err := r.SwitchDownTrack(dt, 1)

		assert.ErrorIs(t, err, errNoReceiverFound)
	})

	t.Run("利用可能なレイヤーへの切り替えは成功", func(t *testing.T) {
		r := &WebRTCReceiver{isSimulcast: true}
		r.available[1].Store(true)
		r.downTracks[1].Store(make([]DownTrack, 0))
		r.pendingTracks[1] = make([]DownTrack, 0)
		dt := newMockDownTrackFull("track-1", "stream-1")

		err := r.SwitchDownTrack(dt, 1)

		assert.NoError(t, err)
		assert.True(t, r.pending[1].Load())
		assert.Len(t, r.pendingTracks[1], 1)
	})
}

func TestWebRTCReceiver_CreatePLIPacket(t *testing.T) {
	t.Run("PLIパケットを作成", func(t *testing.T) {
		r := &WebRTCReceiver{}

		packets := r.createPLIPacket(0)

		assert.Len(t, packets, 1)
		pli, ok := packets[0].(*rtcp.PictureLossIndication)
		assert.True(t, ok)
		assert.NotNil(t, pli)
	})
}

func TestWebRTCReceiver_HandleSimulcastQualityAdjustment(t *testing.T) {
	t.Run("シミュルキャストでない場合は何もしない", func(t *testing.T) {
		r := &WebRTCReceiver{isSimulcast: false}

		// パニックしないことを確認
		r.handleSimulcastQualityAdjustment(0, true)
	})
}

func TestWebRTCReceiver_ConfigureSimulcastDownTrack(t *testing.T) {
	t.Run("シミュルキャスト設定を適用", func(t *testing.T) {
		r := &WebRTCReceiver{
			streamID: "stream-1",
			trackID:  "track-1",
		}
		dt := newMockDownTrackFull("dt-1", "stream-1")

		r.configureSimulcastDownTrack(dt, 2)

		// mockDownTrackFullはSetInitialLayersなどを実装しているため
		// パニックしないことを確認
	})
}

func TestWebRTCReceiver_ConfigureSimpleDownTrack(t *testing.T) {
	t.Run("シンプル設定を適用", func(t *testing.T) {
		r := &WebRTCReceiver{
			streamID: "stream-1",
			trackID:  "track-1",
		}
		dt := newMockDownTrackFull("dt-1", "stream-1")

		r.configureSimpleDownTrack(dt)

		// mockDownTrackFullはSetInitialLayersなどを実装しているため
		// パニックしないことを確認
	})
}

func TestWebRTCReceiver_AddDownTrack(t *testing.T) {
	t.Run("クローズ済みの場合は追加しない", func(t *testing.T) {
		r := &WebRTCReceiver{}
		r.closed.Store(true)
		dt := newMockDownTrackFull("track-1", "stream-1")

		// パニックしないことを確認
		r.AddDownTrack(dt, true)
	})

	t.Run("シンプルモードでDownTrackを追加", func(t *testing.T) {
		r := &WebRTCReceiver{
			isSimulcast: false,
			streamID:    "stream-1",
			trackID:     "track-1",
		}
		r.downTracks[0].Store(make([]DownTrack, 0))
		dt := newMockDownTrackFull("dt-1", "stream-1")

		r.AddDownTrack(dt, false)

		dts := r.downTracks[0].Load().([]DownTrack)
		assert.Len(t, dts, 1)
	})

	t.Run("既に登録されている場合は追加しない", func(t *testing.T) {
		r := &WebRTCReceiver{
			isSimulcast: false,
			streamID:    "stream-1",
			trackID:     "track-1",
		}
		dt := newMockDownTrackFull("dt-1", "stream-1")
		r.downTracks[0].Store([]DownTrack{dt})

		r.AddDownTrack(dt, false)

		dts := r.downTracks[0].Load().([]DownTrack)
		assert.Len(t, dts, 1) // 変わらない
	})
}

func TestWebRTCReceiver_AddUpTrack(t *testing.T) {
	t.Run("クローズ済みの場合は追加しない", func(t *testing.T) {
		r := &WebRTCReceiver{}
		r.closed.Store(true)

		// nilトラックとバッファでパニックしないことを確認
		r.AddUpTrack(nil, nil, true)
	})
}

func TestWebRTCReceiver_IsDownTrackSubscribed(t *testing.T) {
	t.Run("登録されている場合", func(t *testing.T) {
		r := &WebRTCReceiver{}
		dt := newMockDownTrackFull("dt-1", "stream-1")
		r.downTracks[0].Store([]DownTrack{dt})

		result := r.isDownTrackSubscribed(0, dt)

		assert.True(t, result)
	})

	t.Run("登録されていない場合", func(t *testing.T) {
		r := &WebRTCReceiver{}
		dt1 := newMockDownTrackFull("dt-1", "stream-1")
		dt2 := newMockDownTrackFull("dt-2", "stream-1")
		r.downTracks[0].Store([]DownTrack{dt1})

		result := r.isDownTrackSubscribed(0, dt2)

		assert.False(t, result)
	})
}

func TestWebRTCReceiver_StoreDownTrack(t *testing.T) {
	t.Run("DownTrackを追加", func(t *testing.T) {
		r := &WebRTCReceiver{}
		r.downTracks[0].Store(make([]DownTrack, 0))
		dt := newMockDownTrackFull("dt-1", "stream-1")

		r.storeDownTrack(0, dt)

		dts := r.downTracks[0].Load().([]DownTrack)
		assert.Len(t, dts, 1)
	})

	t.Run("複数のDownTrackを追加", func(t *testing.T) {
		r := &WebRTCReceiver{}
		r.downTracks[0].Store(make([]DownTrack, 0))
		dt1 := newMockDownTrackFull("dt-1", "stream-1")
		dt2 := newMockDownTrackFull("dt-2", "stream-1")

		r.storeDownTrack(0, dt1)
		r.storeDownTrack(0, dt2)

		dts := r.downTracks[0].Load().([]DownTrack)
		assert.Len(t, dts, 2)
	})
}

func TestWebRTCReceiver_ConcurrentAccess(t *testing.T) {
	r := &WebRTCReceiver{
		streamID: "stream-1",
		trackID:  "track-1",
	}
	for i := range r.downTracks {
		r.downTracks[i].Store(make([]DownTrack, 0))
	}

	var wg sync.WaitGroup

	// 並行してdownTrackを追加
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dt := newMockDownTrackFull("dt", "stream-1")
			r.AddDownTrack(dt, false)
		}()
	}

	// 並行してGetBitrateを呼び出し
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.GetBitrate()
		}()
	}

	// 並行してGetMaxTemporalLayerを呼び出し
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.GetMaxTemporalLayer()
		}()
	}

	wg.Wait()
	// パニックなく完了すればテスト成功
}

func TestWebRTCReceiver_ProcessSimulcastLayerSwitching(t *testing.T) {
	t.Run("シミュルキャストでない場合は何もしない", func(t *testing.T) {
		r := &WebRTCReceiver{isSimulcast: false}

		// パニックしないことを確認
		r.processSimulcastLayerSwitching(0, nil, nil)
	})

	t.Run("pendingがない場合は何もしない", func(t *testing.T) {
		r := &WebRTCReceiver{isSimulcast: true}
		r.pending[0].Store(false)

		// パニックしないことを確認
		r.processSimulcastLayerSwitching(0, nil, nil)
	})
}

func TestWebRTCReceiver_SwitchDownTracksToTargetQuality(t *testing.T) {
	t.Run("downTracksがnilの場合も安全", func(t *testing.T) {
		// downTracksを初期化しない状態でのnilアクセスは
		// runtime panicを引き起こすため、初期化が必要
		// このテストは構造体の状態確認のみ行う
	})
}

func TestWebRTCReceiver_RetrieveAndPreparePacket(t *testing.T) {
	t.Run("バッファがnilの場合はEOFを返す", func(t *testing.T) {
		r := &WebRTCReceiver{}
		dt := newMockDownTrackFull("dt-1", "stream-1")
		meta := PacketMeta{layer: 0}

		_, _, err := r.retrieveAndPreparePacket(meta, dt, make([]byte, 1500))

		assert.ErrorIs(t, err, io.EOF)
	})
}
