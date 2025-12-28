package buffer

import (
	"testing"
)

// ベンチマーク: nackQueue.push (連続)
func BenchmarkNackQueuePush(b *testing.B) {
	q := newNACKQueue()

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		q.push(uint32(i))
	}
}

// ベンチマーク: nackQueue.push (ランダム順)
func BenchmarkNackQueuePushRandom(b *testing.B) {
	// 疑似ランダムなシーケンス
	sequence := []uint32{42, 7, 99, 23, 56, 12, 88, 34, 67, 5}

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		q := newNACKQueue()
		for _, sn := range sequence {
			q.push(sn + uint32(i)*100)
		}
	}
}

// ベンチマーク: nackQueue.remove
func BenchmarkNackQueueRemove(b *testing.B) {
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		q := newNACKQueue()
		// キューを埋める
		for j := range 50 {
			q.push(uint32(j))
		}
		// 半分を削除
		for j := range 25 {
			q.remove(uint32(j * 2))
		}
	}
}

// ベンチマーク: nackQueue.find
func BenchmarkNackQueueFind(b *testing.B) {
	q := newNACKQueue()
	for i := range 50 {
		q.push(uint32(i * 2)) // 偶数のみ
	}

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		_ = q.find(uint32(i % 100))
	}
}

// ベンチマーク: nackQueue.pairs (少数のNACK)
func BenchmarkNackQueuePairsSmall(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		q := newNACKQueue()
		for i := range 10 {
			q.push(uint32(i))
		}
		_, _ = q.pairs(100)
	}
}

// ベンチマーク: nackQueue.pairs (多数のNACK)
func BenchmarkNackQueuePairsLarge(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		q := newNACKQueue()
		for i := range 80 {
			q.push(uint32(i))
		}
		_, _ = q.pairs(200)
	}
}

// ベンチマーク: nackQueue.pairs (連続シーケンス番号)
func BenchmarkNackQueuePairsConsecutive(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		q := newNACKQueue()
		// 連続した17パケットのロス（1つのNACKペアに収まらない）
		for i := range 17 {
			q.push(uint32(i + 10))
		}
		_, _ = q.pairs(100)
	}
}

// ベンチマーク: nackQueue.pairs (散発的なロス)
func BenchmarkNackQueuePairsSparse(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		q := newNACKQueue()
		// 散発的なパケットロス
		for _, sn := range []uint32{5, 15, 25, 35, 45, 55, 65, 75} {
			q.push(sn)
		}
		_, _ = q.pairs(100)
	}
}

// ベンチマーク: nackQueue.pairs (キーフレーム要求発生)
func BenchmarkNackQueuePairsWithKeyframe(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		q := newNACKQueue()
		// 3回以上NACKされたパケットを作成
		for i := range 10 {
			q.push(uint32(i))
		}
		// 複数回pairsを呼び出してnacked回数を増やす
		_, _ = q.pairs(100)
		_, _ = q.pairs(100)
		_, _ = q.pairs(100)
		_, _ = q.pairs(100) // キーフレーム要求が発生
	}
}

// ベンチマーク: nackPairBuilder.add
func BenchmarkNackPairBuilderAdd(b *testing.B) {
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		builder := &nackPairBuilder{}
		for j := range 20 {
			builder.add(uint32(j))
		}
		_ = builder.build()
	}
}

// ベンチマーク: nackPairBuilder (ビットマップ構築)
func BenchmarkNackPairBuilderBitmap(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		builder := &nackPairBuilder{}
		// 連続した16パケット（1つのNACKペアで表現可能）
		for j := range 16 {
			builder.add(uint32(j + 100))
		}
		_ = builder.build()
	}
}

// ベンチマーク: newNACKQueue
func BenchmarkNewNACKQueue(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		_ = newNACKQueue()
	}
}

// ベンチマーク: 現実的なシナリオ (5%パケットロス)
func BenchmarkNackQueueRealisticScenario(b *testing.B) {
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		q := newNACKQueue()
		headSN := uint32(0)

		// 100パケット中5パケットロス
		for j := range 100 {
			sn := uint32(j + i*100)
			if j%20 == 7 { // 5%ロス
				q.push(sn)
			}
			headSN = sn
		}

		_, _ = q.pairs(headSN)
	}
}

// ベンチマーク: push と remove の混合操作
func BenchmarkNackQueueMixedOperations(b *testing.B) {
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		q := newNACKQueue()
		base := uint32(i * 100)

		// push
		for j := range 20 {
			q.push(base + uint32(j))
		}
		// remove (再送成功をシミュレート)
		for j := range 10 {
			q.remove(base + uint32(j*2))
		}
		// pairs
		_, _ = q.pairs(base + 100)
	}
}
