package sfu

import (
	"testing"
)

// ベンチマーク: sequencer.push (連続)
func BenchmarkSequencerPush(b *testing.B) {
	seq := newSequencer(500)

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		seq.push(uint16(i), uint16(i), uint32(i)*3000, 0, true)
	}
}

// ベンチマーク: sequencer.push (ギャップあり)
func BenchmarkSequencerPushWithGap(b *testing.B) {
	seq := newSequencer(500)

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		// 2パケットおきにギャップ
		sn := uint16(i * 3)
		seq.push(sn, sn, uint32(sn)*3000, 0, true)
	}
}

// ベンチマーク: sequencer.push (遅延パケット)
func BenchmarkSequencerPushLate(b *testing.B) {
	seq := newSequencer(500)

	// 先にパケットを追加
	for i := range 100 {
		seq.push(uint16(i), uint16(i), uint32(i)*3000, 0, true)
	}

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		// 遅延パケット（過去のシーケンス番号）
		sn := uint16(50 + (i % 40))
		seq.push(sn, sn, uint32(sn)*3000, 0, false)
	}
}

// ベンチマーク: sequencer.push (並列)
func BenchmarkSequencerPushParallel(b *testing.B) {
	seq := newSequencer(500)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			seq.push(uint16(i), uint16(i), uint32(i)*3000, 0, true)
			i++
		}
	})
}

// ベンチマーク: sequencer.getSeqNoPairs (少数)
func BenchmarkSequencerGetSeqNoPairsSmall(b *testing.B) {
	seq := newSequencer(500)

	// パケットを追加
	for i := range 200 {
		seq.push(uint16(i), uint16(i), uint32(i)*3000, 0, true)
	}

	seqNos := []uint16{50, 60, 70, 80, 90}

	b.ResetTimer()
	for b.Loop() {
		_ = seq.getSeqNoPairs(seqNos)
	}
}

// ベンチマーク: sequencer.getSeqNoPairs (最大バッチ)
func BenchmarkSequencerGetSeqNoPairsMaxBatch(b *testing.B) {
	seq := newSequencer(500)

	// パケットを追加
	for i := range 200 {
		seq.push(uint16(i), uint16(i), uint32(i)*3000, 0, true)
	}

	// maxNackBatch (17) 個のシーケンス番号
	seqNos := make([]uint16, 17)
	for i := range 17 {
		seqNos[i] = uint16(50 + i*5)
	}

	b.ResetTimer()
	for b.Loop() {
		_ = seq.getSeqNoPairs(seqNos)
	}
}

// ベンチマーク: sequencer.getSeqNoPairs (存在しないSN)
func BenchmarkSequencerGetSeqNoPairsMissing(b *testing.B) {
	seq := newSequencer(500)

	// パケットを追加
	for i := range 200 {
		seq.push(uint16(i), uint16(i), uint32(i)*3000, 0, true)
	}

	// 存在しないシーケンス番号
	seqNos := []uint16{1000, 1001, 1002, 1003, 1004}

	b.ResetTimer()
	for b.Loop() {
		_ = seq.getSeqNoPairs(seqNos)
	}
}

// ベンチマーク: packetMeta VP8ペイロード操作
func BenchmarkPacketMetaVP8(b *testing.B) {
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		pm := &packetMeta{}
		pm.setVP8PayloadMeta(uint8(i%256), uint16(i))
		_, _ = pm.getVP8PayloadMeta()
	}
}

// ベンチマーク: newSequencer
func BenchmarkNewSequencer(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		_ = newSequencer(500)
	}
}

// ベンチマーク: newSequencer (大きいバッファ)
func BenchmarkNewSequencerLarge(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		_ = newSequencer(2000)
	}
}

// ベンチマーク: 現実的なシナリオ (30fps video)
func BenchmarkSequencerRealistic30fps(b *testing.B) {
	seq := newSequencer(500)

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		// 30fpsで各フレーム約3パケット
		sn := uint16(i)
		ts := uint32(i/3) * 3000 // 90kHz clock, 30fps
		layer := uint8(0)
		pm := seq.push(sn, sn, ts, layer, true)
		if pm != nil {
			pm.setVP8PayloadMeta(uint8(i%256), uint16(i%32768))
		}
	}
}

// ベンチマーク: NACKシナリオ (5%パケットロス)
func BenchmarkSequencerNACKScenario(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		seq := newSequencer(500)

		// 100パケット追加
		for i := range 100 {
			seq.push(uint16(i), uint16(i), uint32(i)*3000, 0, true)
		}

		// 5%のパケットロス（NACK要求をシミュレート）
		lostPackets := []uint16{5, 23, 47, 68, 91}
		_ = seq.getSeqNoPairs(lostPackets)
	}
}

// ベンチマーク: シーケンス番号ラップアラウンド
func BenchmarkSequencerWrapAround(b *testing.B) {
	seq := newSequencer(500)

	// 最大値付近から開始
	startSN := uint16(65530)
	for i := range 100 {
		sn := startSN + uint16(i)
		seq.push(sn, sn, uint32(i)*3000, 0, true)
	}

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		sn := startSN + uint16(100+i)
		seq.push(sn, sn, uint32(100+i)*3000, 0, true)
	}
}

// ベンチマーク: 複数レイヤー (シミュルキャスト)
func BenchmarkSequencerMultiLayer(b *testing.B) {
	seq := newSequencer(500)

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		sn := uint16(i)
		layer := uint8(i % 3) // 3レイヤー
		seq.push(sn, sn, uint32(i)*3000, layer, true)
	}
}
