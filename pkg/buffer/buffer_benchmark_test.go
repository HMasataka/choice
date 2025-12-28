package buffer

import (
	"sync"
	"testing"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// テスト用のヘルパー関数
func newTestBuffer() *Buffer {
	videoPool := &sync.Pool{
		New: func() any {
			b := make([]byte, minBufferSize)
			return &b
		},
	}
	audioPool := &sync.Pool{
		New: func() any {
			b := make([]byte, minBufferSize)
			return &b
		},
	}
	return NewBuffer(12345, videoPool, audioPool)
}

func bindTestBuffer(b *Buffer, codecType string) {
	var mimeType string
	switch codecType {
	case "video":
		mimeType = "video/VP8"
	case "audio":
		mimeType = "audio/opus"
	default:
		mimeType = "video/VP8"
	}

	// コールバックを設定
	b.OnFeedback(func(fb []rtcp.Packet) {})
	b.OnTransportWideCC(func(sn uint16, timeNS int64, marker bool) {})
	b.OnAudioLevel(func(level uint8) {})

	params := webrtc.RTPParameters{
		Codecs: []webrtc.RTPCodecParameters{
			{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:  mimeType,
					ClockRate: 90000,
				},
			},
		},
	}
	b.Bind(params, Options{MaxBitRate: 1_000_000})
}

func createTestRTPPacket(seqNo uint16, timestamp uint32) []byte {
	pkt := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Padding:        false,
			Extension:      false,
			Marker:         false,
			PayloadType:    96,
			SequenceNumber: seqNo,
			Timestamp:      timestamp,
			SSRC:           12345,
		},
		Payload: make([]byte, 100),
	}
	data, _ := pkt.Marshal()
	return data
}

// ベンチマーク: Buffer.Write
func BenchmarkBufferWrite(b *testing.B) {
	buf := newTestBuffer()
	bindTestBuffer(buf, "video")

	pkt := createTestRTPPacket(1, 1000)

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		// シーケンス番号を更新してパケットを作成
		pkt[2] = byte(i >> 8)
		pkt[3] = byte(i)
		_, _ = buf.Write(pkt)
	}
}

// ベンチマーク: Buffer.Write (並列)
func BenchmarkBufferWriteParallel(b *testing.B) {
	buf := newTestBuffer()
	bindTestBuffer(buf, "video")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		seqNo := uint16(0)
		for pb.Next() {
			pkt := createTestRTPPacket(seqNo, uint32(seqNo)*100)
			_, _ = buf.Write(pkt)
			seqNo++
		}
	})
}

// ベンチマーク: Buffer.ReadExtended
// 並行してWriteとReadを行い、Read側の性能を計測
func BenchmarkBufferReadExtended(b *testing.B) {
	buf := newTestBuffer()
	bindTestBuffer(buf, "video")

	// Writer goroutine: パケットを継続的に書き込む
	stop := make(chan struct{})
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				pkt := createTestRTPPacket(uint16(i), uint32(i)*100)
				_, _ = buf.Write(pkt)
			}
		}
	}()

	b.ResetTimer()
	for b.Loop() {
		_, err := buf.ReadExtended()
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()

	close(stop)
	_ = buf.Close()
	<-writerDone
}

// ベンチマーク: Write と ReadExtended の組み合わせ
func BenchmarkBufferWriteRead(b *testing.B) {
	buf := newTestBuffer()
	bindTestBuffer(buf, "video")

	readCount := 0
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, err := buf.ReadExtended()
			if err != nil {
				return
			}
			readCount++
			if readCount >= b.N {
				return
			}
		}
	}()

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		pkt := createTestRTPPacket(uint16(i), uint32(i)*100)
		_, _ = buf.Write(pkt)
	}
	b.StopTimer()

	buf.Close()
	<-done
}

// ベンチマーク: GetPacket
func BenchmarkBufferGetPacket(b *testing.B) {
	buf := newTestBuffer()
	bindTestBuffer(buf, "video")

	// 事前にパケットを書き込む
	for i := range 1000 {
		pkt := createTestRTPPacket(uint16(i), uint32(i)*100)
		_, _ = buf.Write(pkt)
	}

	buff := make([]byte, 1500)

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		sn := uint16(i % 1000)
		_, _ = buf.GetPacket(buff, sn)
	}
}

// ベンチマーク: GetStats
func BenchmarkBufferGetStats(b *testing.B) {
	buf := newTestBuffer()
	bindTestBuffer(buf, "video")

	// 事前にパケットを書き込む
	for i := range 100 {
		pkt := createTestRTPPacket(uint16(i), uint32(i)*100)
		_, _ = buf.Write(pkt)
	}

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		buf.GetStats()
	}
}

// ベンチマーク: Bind
func BenchmarkBufferBind(b *testing.B) {
	params := webrtc.RTPParameters{
		Codecs: []webrtc.RTPCodecParameters{
			{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:  "video/VP8",
					ClockRate: 90000,
				},
			},
		},
	}

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		buf := newTestBuffer()
		buf.Bind(params, Options{MaxBitRate: 1_000_000})
	}
}
