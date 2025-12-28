package twcc

import (
	"testing"

	"github.com/pion/rtcp"
)

func newTestResponder() *Responder {
	r := NewTransportWideCCResponder(12345)
	r.OnFeedback(func(p rtcp.RawPacket) {})
	return r
}

// ベンチマーク: Responder.Push (単一パケット)
func BenchmarkResponderPush(b *testing.B) {
	r := newTestResponder()

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		r.Push(uint16(i), int64(i)*1000000, false)
	}
}

// ベンチマーク: Responder.Push (マーカービット付き)
func BenchmarkResponderPushWithMarker(b *testing.B) {
	r := newTestResponder()

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		// 30パケットごとにマーカービットを設定（フレーム境界をシミュレート）
		marker := i%30 == 29
		r.Push(uint16(i), int64(i)*1000000, marker)
	}
}

// ベンチマーク: Responder.Push (並列、各goroutineで独立したResponder)
func BenchmarkResponderPushParallel(b *testing.B) {
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		r := newTestResponder()
		i := 0
		for pb.Next() {
			r.Push(uint16(i), int64(i)*1000000, false)
			i++
		}
	})
}

// ベンチマーク: Responder.Push (フィードバック生成あり)
// 21パケット以上で100ms経過でフィードバック送信
func BenchmarkResponderPushWithFeedback(b *testing.B) {
	r := newTestResponder()

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		// 100ms間隔をシミュレート（tccReportDelta = 1e8）
		timeNS := int64(i) * 5000000 // 5ms間隔
		r.Push(uint16(i), timeNS, false)
	}
}

// ベンチマーク: Responder.Push (パケットロスあり)
func BenchmarkResponderPushWithLoss(b *testing.B) {
	r := newTestResponder()

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		// 10%のパケットロスをシミュレート
		if i%10 == 5 {
			continue
		}
		r.Push(uint16(i), int64(i)*1000000, false)
	}
}

// ベンチマーク: Responder.Push (シーケンス番号ラップアラウンド)
func BenchmarkResponderPushWithWrap(b *testing.B) {
	r := newTestResponder()

	// シーケンス番号を最大値付近から開始
	startSN := uint16(65530)

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		sn := startSN + uint16(i)
		r.Push(sn, int64(i)*1000000, false)
	}
}

// ベンチマーク: buildTransportCCPacket
func BenchmarkBuildTransportCCPacket(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		r := newTestResponder()
		// フィードバック生成に十分なパケットを追加
		for i := range 50 {
			r.Push(uint16(i), int64(i)*1000000, false)
		}
		// 直接呼び出し
		r.mu.Lock()
		_ = r.buildTransportCCPacket()
		r.mu.Unlock()
	}
}

// ベンチマーク: buildPacketList (ソートとギャップ検出)
func BenchmarkBuildPacketList(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		r := newTestResponder()
		// パケットを順不同で追加
		for _, sn := range []uint16{5, 2, 8, 1, 9, 3, 7, 4, 6, 0} {
			r.extInfo = append(r.extInfo, rtpExtInfo{
				ExtTSN:    uint32(sn),
				Timestamp: int64(sn) * 1000,
			})
		}
		r.mu.Lock()
		_ = r.buildPacketList()
		r.mu.Unlock()
	}
}

// ベンチマーク: setNBitsOfUint16
func BenchmarkSetNBitsOfUint16(b *testing.B) {
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		_ = setNBitsOfUint16(0, 2, 4, uint16(i%4))
	}
}

// ベンチマーク: clampInt16
func BenchmarkClampInt16(b *testing.B) {
	values := []int64{0, 100, -100, 32767, -32768, 100000, -100000}

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		_ = clampInt16(values[i%len(values)])
	}
}

// ベンチマーク: NewTransportWideCCResponder
func BenchmarkNewTransportWideCCResponder(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		r := NewTransportWideCCResponder(12345)
		r.OnFeedback(func(p rtcp.RawPacket) {})
	}
}

// ベンチマーク: 高スループットシナリオ (60fps video)
func BenchmarkResponderHighThroughput(b *testing.B) {
	r := newTestResponder()

	// 60fpsで30パケット/フレームをシミュレート
	packetsPerFrame := 30
	frameIntervalNS := int64(16666667) // ~60fps

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		frame := i / packetsPerFrame
		packetInFrame := i % packetsPerFrame
		timeNS := int64(frame)*frameIntervalNS + int64(packetInFrame)*100000
		marker := packetInFrame == packetsPerFrame-1
		r.Push(uint16(i), timeNS, marker)
	}
}
