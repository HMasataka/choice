package sfu

import (
	"testing"

	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// mockReceiver はベンチマーク用のモックReceiver
type mockReceiver struct {
	trackID  string
	streamID string
}

func (m *mockReceiver) TrackID() string                      { return m.trackID }
func (m *mockReceiver) StreamID() string                     { return m.streamID }
func (m *mockReceiver) SSRC(_ int) uint32                    { return 12345 }
func (m *mockReceiver) Codec() webrtc.RTPCodecParameters     { return webrtc.RTPCodecParameters{} }
func (m *mockReceiver) Kind() webrtc.RTPCodecType            { return webrtc.RTPCodecTypeVideo }
func (m *mockReceiver) AddUpTrack(_ *webrtc.TrackRemote, _ *buffer.Buffer, _ bool) {
}
func (m *mockReceiver) AddDownTrack(_ DownTrack, _ bool)                    {}
func (m *mockReceiver) SwitchDownTrack(_ DownTrack, _ int) error            { return nil }
func (m *mockReceiver) GetBitrate() [3]uint64                               { return [3]uint64{100000, 500000, 1000000} }
func (m *mockReceiver) GetMaxTemporalLayer() [3]int32                       { return [3]int32{2, 2, 2} }
func (m *mockReceiver) RetransmitPackets(_ DownTrack, _ []packetMeta) error { return nil }
func (m *mockReceiver) DeleteDownTrack(_ int, _ string)                     {}
func (m *mockReceiver) OnCloseHandler(_ func())                             {}
func (m *mockReceiver) SendRTCP(_ []rtcp.Packet)                            {}
func (m *mockReceiver) SetRTCPCh(_ chan []rtcp.Packet)                      {}
func (m *mockReceiver) GetSenderReportTime(_ int) (uint32, uint64)          { return 0, 0 }
func (m *mockReceiver) ReadRTP(_ []byte, _ int) (int, error)                { return 0, nil }
func (m *mockReceiver) SetTrackMeta(_, _ string)                            {}
func (m *mockReceiver) GetTrackMeta() (string, string)                      { return "", "" }

// mockWriteStream はベンチマーク用のモックWriteStream
type mockWriteStream struct{}

func (m *mockWriteStream) WriteRTP(_ *rtp.Header, _ []byte) (int, error) { return 0, nil }
func (m *mockWriteStream) Write(_ []byte) (int, error)                   { return 0, nil }

func newTestDownTrack() *downTrack {
	receiver := &mockReceiver{
		trackID:  "test-track",
		streamID: "test-stream",
	}

	bf := buffer.NewBufferFactory(100)

	dt, _ := NewDownTrack(
		webrtc.RTPCodecCapability{
			MimeType:  "video/VP8",
			ClockRate: 90000,
		},
		receiver,
		bf,
		"test-peer",
		500,
	)

	dt.writeStream = &mockWriteStream{}
	dt.ssrc = 12345
	dt.payloadType = 96
	dt.mime = "video/vp8"
	dt.sequencer = newSequencer(500)
	dt.enabled.Store(true)
	dt.bound.Store(true)
	dt.trackType = SimpleDownTrack

	return dt
}

func createTestExtPacket(seqNo uint16, timestamp uint32, keyframe bool) *buffer.ExtPacket {
	return &buffer.ExtPacket{
		Head:     true,
		KeyFrame: keyframe,
		Packet: rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    96,
				SequenceNumber: seqNo,
				Timestamp:      timestamp,
				SSRC:           12345,
			},
			Payload: make([]byte, 100),
		},
	}
}

// ベンチマーク: NewDownTrack
func BenchmarkNewDownTrack(b *testing.B) {
	receiver := &mockReceiver{
		trackID:  "test-track",
		streamID: "test-stream",
	}

	bf := buffer.NewBufferFactory(100)

	codec := webrtc.RTPCodecCapability{
		MimeType:  "video/VP8",
		ClockRate: 90000,
	}

	b.ResetTimer()
	for b.Loop() {
		_, _ = NewDownTrack(codec, receiver, bf, "test-peer", 500)
	}
}

// ベンチマーク: writeSimpleRTP
func BenchmarkDownTrackWriteSimpleRTP(b *testing.B) {
	dt := newTestDownTrack()

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		pkt := createTestExtPacket(uint16(i), uint32(i)*3000, true)
		_ = dt.writeSimpleRTP(pkt)
	}
}

// ベンチマーク: writeSimpleRTP (再同期あり)
func BenchmarkDownTrackWriteSimpleRTPWithReSync(b *testing.B) {
	dt := newTestDownTrack()

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		if i%100 == 0 {
			dt.reSync.Store(true)
		}
		pkt := createTestExtPacket(uint16(i), uint32(i)*3000, true)
		_ = dt.writeSimpleRTP(pkt)
	}
}

// ベンチマーク: writeSimulcastRTP
func BenchmarkDownTrackWriteSimulcastRTP(b *testing.B) {
	dt := newTestDownTrack()
	dt.trackType = SimulcastDownTrack
	dt.lastSSRC = 12345

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		pkt := createTestExtPacket(uint16(i), uint32(i)*3000, true)
		pkt.Arrival = int64(i) * 1000000
		_ = dt.writeSimulcastRTP(pkt, 0)
	}
}

// ベンチマーク: UpdateStats
func BenchmarkDownTrackUpdateStats(b *testing.B) {
	dt := newTestDownTrack()

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		dt.UpdateStats(uint32(100 + i%50))
	}
}

// ベンチマーク: UpdateStats (並列)
func BenchmarkDownTrackUpdateStatsParallel(b *testing.B) {
	dt := newTestDownTrack()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			dt.UpdateStats(uint32(100 + i%50))
			i++
		}
	})
}

// ベンチマーク: CreateSourceDescriptionChunks
func BenchmarkDownTrackCreateSourceDescriptionChunks(b *testing.B) {
	dt := newTestDownTrack()
	dt.transceiver = &webrtc.RTPTransceiver{}

	b.ResetTimer()
	for b.Loop() {
		_ = dt.CreateSourceDescriptionChunks()
	}
}

// ベンチマーク: CreateSenderReport
func BenchmarkDownTrackCreateSenderReport(b *testing.B) {
	dt := newTestDownTrack()

	b.ResetTimer()
	for b.Loop() {
		_ = dt.CreateSenderReport()
	}
}

// ベンチマーク: Kind
func BenchmarkDownTrackKind(b *testing.B) {
	dt := newTestDownTrack()

	b.ResetTimer()
	for b.Loop() {
		_ = dt.Kind()
	}
}

// ベンチマーク: Enabled/Mute
func BenchmarkDownTrackEnabledMute(b *testing.B) {
	dt := newTestDownTrack()

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		_ = dt.Enabled()
		if i%100 == 0 {
			dt.Mute(true)
			dt.Mute(false)
		}
	}
}

// ベンチマーク: buildAdjustedHeader
func BenchmarkDownTrackBuildAdjustedHeader(b *testing.B) {
	dt := newTestDownTrack()
	hdr := rtp.Header{
		Version:        2,
		PayloadType:    96,
		SequenceNumber: 1000,
		Timestamp:      3000000,
		SSRC:           12345,
	}

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		_ = dt.buildAdjustedHeader(hdr, uint16(i), uint32(i)*3000)
	}
}

// ベンチマーク: SwitchSpatialLayer
func BenchmarkDownTrackSwitchSpatialLayer(b *testing.B) {
	dt := newTestDownTrack()
	dt.trackType = SimulcastDownTrack

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		layer := int32(i % 3)
		_ = dt.SwitchSpatialLayer(layer, false)
	}
}

// ベンチマーク: SwitchTemporalLayer
func BenchmarkDownTrackSwitchTemporalLayer(b *testing.B) {
	dt := newTestDownTrack()
	dt.trackType = SimulcastDownTrack

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		layer := int32(i % 3)
		dt.SwitchTemporalLayer(layer, false)
	}
}

// ベンチマーク: CurrentSpatialLayer
func BenchmarkDownTrackCurrentSpatialLayer(b *testing.B) {
	dt := newTestDownTrack()

	b.ResetTimer()
	for b.Loop() {
		_ = dt.CurrentSpatialLayer()
	}
}

// ベンチマーク: 現実的なシナリオ (30fps video streaming)
func BenchmarkDownTrackRealistic30fps(b *testing.B) {
	dt := newTestDownTrack()

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		// 30fpsで各フレーム約3パケット
		seqNo := uint16(i)
		ts := uint32(i/3) * 3000
		keyframe := i%90 == 0 // 3秒ごとにキーフレーム

		pkt := createTestExtPacket(seqNo, ts, keyframe)
		_ = dt.writeSimpleRTP(pkt)
	}
}

// ベンチマーク: 高スループット (60fps video streaming)
func BenchmarkDownTrackHighThroughput60fps(b *testing.B) {
	dt := newTestDownTrack()

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		// 60fpsで各フレーム約5パケット
		seqNo := uint16(i)
		ts := uint32(i/5) * 1500
		keyframe := i%300 == 0 // 1秒ごとにキーフレーム

		pkt := createTestExtPacket(seqNo, ts, keyframe)
		_ = dt.writeSimpleRTP(pkt)
	}
}
