package buffer

import (
	"testing"

	"github.com/pion/rtp"
)

func newTestBucket() *Bucket {
	buf := make([]byte, minBufferSize)
	return NewBucket(&buf)
}

func createTestRTPPacketForBucket(seqNo uint16, timestamp uint32) []byte {
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

// ベンチマーク: Bucket.AddPacket (連続書き込み)
func BenchmarkBucketAddPacket(b *testing.B) {
	bucket := newTestBucket()

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		pkt := createTestRTPPacketForBucket(uint16(i), uint32(i)*100)
		_, _ = bucket.AddPacket(pkt, uint16(i), true)
	}
}

// ベンチマーク: Bucket.AddPacket (並列)
func BenchmarkBucketAddPacketParallel(b *testing.B) {
	bucket := newTestBucket()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		seqNo := uint16(0)
		for pb.Next() {
			pkt := createTestRTPPacketForBucket(seqNo, uint32(seqNo)*100)
			_, _ = bucket.AddPacket(pkt, seqNo, true)
			seqNo++
		}
	})
}

// ベンチマーク: Bucket.GetPacket
func BenchmarkBucketGetPacket(b *testing.B) {
	bucket := newTestBucket()

	// 事前にパケットを書き込む
	for i := range 1000 {
		pkt := createTestRTPPacketForBucket(uint16(i), uint32(i)*100)
		_, _ = bucket.AddPacket(pkt, uint16(i), true)
	}

	buf := make([]byte, maxPktSize)

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		sn := uint16(i % 1000)
		_, _ = bucket.GetPacket(buf, sn)
	}
}

// ベンチマーク: Bucket.AddPacket (非連続シーケンス番号)
func BenchmarkBucketAddPacketWithGap(b *testing.B) {
	bucket := newTestBucket()

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		// 2つおきにシーケンス番号を進める（パケットロスをシミュレート）
		seqNo := uint16(i * 2)
		pkt := createTestRTPPacketForBucket(seqNo, uint32(seqNo)*100)
		_, _ = bucket.AddPacket(pkt, seqNo, true)
	}
}

// ベンチマーク: Bucket.AddPacket (リオーダー)
func BenchmarkBucketAddPacketReorder(b *testing.B) {
	bucket := newTestBucket()

	// 事前に連続パケットを書き込む
	for i := range 100 {
		pkt := createTestRTPPacketForBucket(uint16(i), uint32(i)*100)
		_, _ = bucket.AddPacket(pkt, uint16(i), true)
	}

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		// 過去のシーケンス番号で再送パケットをシミュレート
		seqNo := uint16(50 + (i % 30))
		pkt := createTestRTPPacketForBucket(seqNo, uint32(seqNo)*100)
		_, _ = bucket.AddPacket(pkt, seqNo, false)
	}
}

// ベンチマーク: Bucket 作成
func BenchmarkBucketNew(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		buf := make([]byte, minBufferSize)
		_ = NewBucket(&buf)
	}
}

// ベンチマーク: AddPacket と GetPacket の組み合わせ
func BenchmarkBucketAddAndGet(b *testing.B) {
	bucket := newTestBucket()
	getBuf := make([]byte, maxPktSize)

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		seqNo := uint16(i)
		pkt := createTestRTPPacketForBucket(seqNo, uint32(seqNo)*100)
		_, _ = bucket.AddPacket(pkt, seqNo, true)

		// 直前に追加したパケットを取得
		_, _ = bucket.GetPacket(getBuf, seqNo)
	}
}
