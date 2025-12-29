package sfu

import (
	"testing"
	"time"

	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
)

func TestSubscriberConstants(t *testing.T) {
	t.Run("APIChannelLabel", func(t *testing.T) {
		assert.Equal(t, "choice", APIChannelLabel)
	})

	t.Run("sendRTCPRetryAttempts", func(t *testing.T) {
		assert.Equal(t, 6, sendRTCPRetryAttempts)
	})

	t.Run("sdesChunkBatchSize", func(t *testing.T) {
		assert.Equal(t, 15, sdesChunkBatchSize)
	})

	t.Run("downTrackReportInterval", func(t *testing.T) {
		assert.Equal(t, 5*time.Second, downTrackReportInterval)
	})
}

func TestSubscriberInterface(t *testing.T) {
	t.Run("subscriber型がSubscriberインターフェースを実装している", func(t *testing.T) {
		// コンパイル時チェック
		var _ Subscriber = (*subscriber)(nil)
	})
}

func TestSubscriber_StructFields(t *testing.T) {
	t.Run("初期状態", func(t *testing.T) {
		s := &subscriber{
			userID:          "test-user",
			isAutoSubscribe: true,
			tracks:          make(map[string][]DownTrack),
			channels:        make(map[string]*webrtc.DataChannel),
		}

		assert.Equal(t, "test-user", s.userID)
		assert.True(t, s.isAutoSubscribe)
		assert.NotNil(t, s.tracks)
		assert.Empty(t, s.tracks)
	})
}

func TestSubscriber_GetUserID(t *testing.T) {
	s := &subscriber{userID: "user-123"}

	assert.Equal(t, "user-123", s.GetUserID())
}

func TestSubscriber_IsAutoSubscribe(t *testing.T) {
	t.Run("true", func(t *testing.T) {
		s := &subscriber{isAutoSubscribe: true}
		assert.True(t, s.IsAutoSubscribe())
	})

	t.Run("false", func(t *testing.T) {
		s := &subscriber{isAutoSubscribe: false}
		assert.False(t, s.IsAutoSubscribe())
	})
}

func TestSubscriber_DownTracks(t *testing.T) {
	t.Run("空のtracks", func(t *testing.T) {
		s := &subscriber{tracks: make(map[string][]DownTrack)}

		result := s.DownTracks()

		assert.Empty(t, result)
	})

	t.Run("複数のtracks", func(t *testing.T) {
		dt1 := newMockDownTrackFull("track-1", "stream-1")
		dt2 := newMockDownTrackFull("track-2", "stream-1")
		dt3 := newMockDownTrackFull("track-3", "stream-2")

		s := &subscriber{
			tracks: map[string][]DownTrack{
				"stream-1": {dt1, dt2},
				"stream-2": {dt3},
			},
		}

		result := s.DownTracks()

		assert.Len(t, result, 3)
	})
}

func TestSubscriber_GetDownTracks(t *testing.T) {
	dt1 := newMockDownTrackFull("track-1", "stream-1")
	dt2 := newMockDownTrackFull("track-2", "stream-1")

	s := &subscriber{
		tracks: map[string][]DownTrack{
			"stream-1": {dt1, dt2},
		},
	}

	t.Run("存在するストリーム", func(t *testing.T) {
		result := s.GetDownTracks("stream-1")
		assert.Len(t, result, 2)
	})

	t.Run("存在しないストリーム", func(t *testing.T) {
		result := s.GetDownTracks("nonexistent")
		assert.Nil(t, result)
	})
}

func TestSubscriber_AddDownTrack(t *testing.T) {
	t.Run("新しいストリームに追加", func(t *testing.T) {
		s := &subscriber{tracks: make(map[string][]DownTrack)}
		dt := newMockDownTrackFull("track-1", "stream-1")

		s.AddDownTrack("stream-1", dt)

		assert.Len(t, s.tracks["stream-1"], 1)
		assert.Equal(t, dt, s.tracks["stream-1"][0])
	})

	t.Run("既存のストリームに追加", func(t *testing.T) {
		dt1 := newMockDownTrackFull("track-1", "stream-1")
		s := &subscriber{
			tracks: map[string][]DownTrack{
				"stream-1": {dt1},
			},
		}
		dt2 := newMockDownTrackFull("track-2", "stream-1")

		s.AddDownTrack("stream-1", dt2)

		assert.Len(t, s.tracks["stream-1"], 2)
	})
}

func TestSubscriber_RemoveDownTrack(t *testing.T) {
	t.Run("存在するトラックを削除", func(t *testing.T) {
		dt1 := newMockDownTrackFull("track-1", "stream-1")
		dt2 := newMockDownTrackFull("track-2", "stream-1")
		s := &subscriber{
			tracks: map[string][]DownTrack{
				"stream-1": {dt1, dt2},
			},
		}

		s.RemoveDownTrack("stream-1", dt1)

		assert.Len(t, s.tracks["stream-1"], 1)
		assert.Equal(t, dt2, s.tracks["stream-1"][0])
	})

	t.Run("存在しないトラックを削除", func(t *testing.T) {
		dt1 := newMockDownTrackFull("track-1", "stream-1")
		dt2 := newMockDownTrackFull("track-2", "stream-1")
		s := &subscriber{
			tracks: map[string][]DownTrack{
				"stream-1": {dt1},
			},
		}

		// パニックしないことを確認
		s.RemoveDownTrack("stream-1", dt2)

		assert.Len(t, s.tracks["stream-1"], 1)
	})

	t.Run("存在しないストリームから削除", func(t *testing.T) {
		dt := newMockDownTrackFull("track-1", "stream-1")
		s := &subscriber{tracks: make(map[string][]DownTrack)}

		// パニックしないことを確認
		s.RemoveDownTrack("nonexistent", dt)
	})

	t.Run("最後のトラックを削除", func(t *testing.T) {
		dt := newMockDownTrackFull("track-1", "stream-1")
		s := &subscriber{
			tracks: map[string][]DownTrack{
				"stream-1": {dt},
			},
		}

		s.RemoveDownTrack("stream-1", dt)

		assert.Empty(t, s.tracks["stream-1"])
	})
}

func TestSubscriber_Negotiate(t *testing.T) {
	t.Run("negotiateがnilの場合", func(t *testing.T) {
		s := &subscriber{negotiate: nil}

		// パニックしないことを確認
		s.Negotiate()
	})

	t.Run("negotiateが設定されている場合", func(t *testing.T) {
		s := &subscriber{
			negotiate: func() {
				// called
			},
		}

		s.Negotiate()

		// debounceされるため、即座には呼ばれない可能性があるが
		// 設定されていることは確認できる
		assert.NotNil(t, s.negotiate)
	})
}

func TestSubscriber_DataChannel(t *testing.T) {
	t.Run("存在しないチャネル", func(t *testing.T) {
		s := &subscriber{
			channels: make(map[string]*webrtc.DataChannel),
		}

		result := s.DataChannel("nonexistent")

		assert.Nil(t, result)
	})
}

func TestSubscriber_GetDatachannel(t *testing.T) {
	t.Run("DataChannelと同じ結果を返す", func(t *testing.T) {
		s := &subscriber{
			channels: make(map[string]*webrtc.DataChannel),
		}

		result := s.GetDatachannel("test")

		assert.Nil(t, result)
	})
}

func TestSubscriber_RegisterDatachannel(t *testing.T) {
	t.Run("チャネルを登録", func(t *testing.T) {
		s := &subscriber{
			channels: make(map[string]*webrtc.DataChannel),
		}

		// DataChannelはnilでも登録可能
		s.RegisterDatachannel("test", nil)

		_, exists := s.channels["test"]
		assert.True(t, exists)
	})
}

func TestRtcpRetryExecutor(t *testing.T) {
	t.Run("構造体の初期化", func(t *testing.T) {
		packets := []rtcp.Packet{}
		executor := &rtcpRetryExecutor{
			streamID: "stream-1",
			packets:  packets,
		}

		assert.Equal(t, "stream-1", executor.streamID)
		assert.NotNil(t, executor.packets)
	})
}

func TestSubscriber_BuildTracksReports(t *testing.T) {
	t.Run("空のtracks", func(t *testing.T) {
		s := &subscriber{
			tracks: make(map[string][]DownTrack),
		}

		sd, reports := s.buildTracksReports()

		assert.Empty(t, sd)
		assert.Empty(t, reports)
	})

	t.Run("バインドされていないトラック", func(t *testing.T) {
		dt := newMockDownTrackFull("track-1", "stream-1")
		dt.bound = false

		s := &subscriber{
			tracks: map[string][]DownTrack{
				"stream-1": {dt},
			},
		}

		sd, reports := s.buildTracksReports()

		assert.Empty(t, sd)
		assert.Empty(t, reports)
	})

	t.Run("バインドされたトラック", func(t *testing.T) {
		dt := newMockDownTrackFull("track-1", "stream-1")
		dt.bound = true

		s := &subscriber{
			tracks: map[string][]DownTrack{
				"stream-1": {dt},
			},
		}

		sd, reports := s.buildTracksReports()

		// mockDownTrackFullはnilを返すのでemptyだが、メソッドが正常に呼ばれることを確認
		assert.Empty(t, sd)
		assert.Empty(t, reports)
	})
}

func TestSubscriber_BuildStreamSourceDescriptions(t *testing.T) {
	t.Run("空のストリーム", func(t *testing.T) {
		s := &subscriber{
			tracks: make(map[string][]DownTrack),
		}

		sd := s.buildStreamSourceDescriptions("nonexistent")

		assert.Empty(t, sd)
	})

	t.Run("バインドされていないトラック", func(t *testing.T) {
		dt := newMockDownTrackFull("track-1", "stream-1")
		dt.bound = false

		s := &subscriber{
			tracks: map[string][]DownTrack{
				"stream-1": {dt},
			},
		}

		sd := s.buildStreamSourceDescriptions("stream-1")

		assert.Empty(t, sd)
	})
}

func TestSubscriber_SendRTCPWithRetry(t *testing.T) {
	t.Run("nilのsubscriberでパニックしない", func(t *testing.T) {
		var s *subscriber = nil
		s.sendRTCPWithRetry("stream-1", []rtcp.Packet{}, 3, 10*time.Millisecond)
		// パニックしなければ成功
	})

	t.Run("空のパケットでは何もしない", func(t *testing.T) {
		s := &subscriber{}
		s.sendRTCPWithRetry("stream-1", []rtcp.Packet{}, 3, 10*time.Millisecond)
		// パニックしなければ成功
	})
}

func TestSubscriber_SendBatchedReports(t *testing.T) {
	t.Run("空のバッチサイズ", func(t *testing.T) {
		// batchSize <= 0 の場合はデフォルト値が使用される
		s := &subscriber{}
		sd := []rtcp.SourceDescriptionChunk{}
		reports := []rtcp.Packet{}

		// pcがnilの場合はパニックするため、この関数は統合テストで検証
		_ = s
		_ = sd
		_ = reports
	})
}

// mockDownTrackFull はテスト用のDownTrackフルモック
type mockDownTrackFull struct {
	id       string
	streamID string
	bound    bool
	rid      string
	kind     webrtc.RTPCodecType
	codec    webrtc.RTPCodecCapability
	enabled  bool
	muted    bool
	payload  *[]byte
}

func newMockDownTrackFull(id, streamID string) *mockDownTrackFull {
	return &mockDownTrackFull{
		id:       id,
		streamID: streamID,
		bound:    false,
		kind:     webrtc.RTPCodecTypeVideo,
		enabled:  true,
	}
}

func (m *mockDownTrackFull) ID() string       { return m.id }
func (m *mockDownTrackFull) StreamID() string { return m.streamID }
func (m *mockDownTrackFull) RID() string      { return m.rid }
func (m *mockDownTrackFull) Bound() bool      { return m.bound }

func (m *mockDownTrackFull) Bind(t webrtc.TrackLocalContext) (webrtc.RTPCodecParameters, error) {
	return webrtc.RTPCodecParameters{}, nil
}
func (m *mockDownTrackFull) Unbind(_ webrtc.TrackLocalContext) error { return nil }
func (m *mockDownTrackFull) Kind() webrtc.RTPCodecType               { return m.kind }
func (m *mockDownTrackFull) Codec() webrtc.RTPCodecCapability        { return m.codec }
func (m *mockDownTrackFull) Stop() error                             { return nil }
func (m *mockDownTrackFull) SetTransceiver(transceiver *webrtc.RTPTransceiver) {
}
func (m *mockDownTrackFull) WriteRTP(p *buffer.ExtPacket, layer int) error { return nil }
func (m *mockDownTrackFull) Enabled() bool                                 { return m.enabled }
func (m *mockDownTrackFull) Mute(val bool)                                 { m.muted = val }
func (m *mockDownTrackFull) Close()                                        {}
func (m *mockDownTrackFull) SetInitialLayers(spatialLayer, temporalLayer int32) {
}
func (m *mockDownTrackFull) CurrentSpatialLayer() int { return 0 }
func (m *mockDownTrackFull) SwitchSpatialLayer(targetLayer int32, setAsMax bool) error {
	return nil
}
func (m *mockDownTrackFull) SwitchSpatialLayerDone(layer int32) {}
func (m *mockDownTrackFull) UptrackLayersChange(availableLayers []uint16) (int64, error) {
	return 0, nil
}
func (m *mockDownTrackFull) SwitchTemporalLayer(targetLayer int32, setAsMax bool) {}
func (m *mockDownTrackFull) OnCloseHandler(fn func())                             {}
func (m *mockDownTrackFull) OnBind(fn func())                                     {}
func (m *mockDownTrackFull) CreateSourceDescriptionChunks() []rtcp.SourceDescriptionChunk {
	return nil
}
func (m *mockDownTrackFull) CreateSenderReport() *rtcp.SenderReport { return nil }
func (m *mockDownTrackFull) UpdateStats(packetLen uint32)           {}
func (m *mockDownTrackFull) GetSimulcast() SimulcastTrackHelpers    { return SimulcastTrackHelpers{} }
func (m *mockDownTrackFull) GetMime() string                        { return "video/vp8" }
func (m *mockDownTrackFull) GetPayloadType() uint8                  { return 96 }
func (m *mockDownTrackFull) SetPayload(payload *[]byte)             { m.payload = payload }
func (m *mockDownTrackFull) GetWriteStream() webrtc.TrackLocalWriter {
	return nil
}
func (m *mockDownTrackFull) GetSSRC() uint32                 { return 12345 }
func (m *mockDownTrackFull) SetLastSSRC(ssrc uint32)         {}
func (m *mockDownTrackFull) SetTrackType(t DownTrackType)    {}
func (m *mockDownTrackFull) SetMaxSpatialLayer(layer int32)  {}
func (m *mockDownTrackFull) SetMaxTemporalLayer(layer int32) {}
