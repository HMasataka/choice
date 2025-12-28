package buffer

import (
	"testing"

	"github.com/pion/rtcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNACKQueue(t *testing.T) {
	q := newNACKQueue()

	assert.NotNil(t, q)
	assert.Equal(t, 0, len(q.nacks))
	assert.Equal(t, maxNackCache+1, cap(q.nacks))
	assert.Equal(t, uint32(0), q.kfSN)
}

func TestNackQueue_push(t *testing.T) {
	t.Run("空のキューにpush", func(t *testing.T) {
		q := newNACKQueue()

		q.push(100)

		require.Equal(t, 1, len(q.nacks))
		assert.Equal(t, uint32(100), q.nacks[0].sn)
		assert.Equal(t, uint8(0), q.nacks[0].nacked)
	})

	t.Run("ソート順を維持してpush", func(t *testing.T) {
		q := newNACKQueue()

		q.push(50)
		q.push(30)
		q.push(70)
		q.push(40)

		require.Equal(t, 4, len(q.nacks))
		assert.Equal(t, uint32(30), q.nacks[0].sn)
		assert.Equal(t, uint32(40), q.nacks[1].sn)
		assert.Equal(t, uint32(50), q.nacks[2].sn)
		assert.Equal(t, uint32(70), q.nacks[3].sn)
	})

	t.Run("重複するSNはpushしない", func(t *testing.T) {
		q := newNACKQueue()

		q.push(100)
		q.push(100)
		q.push(100)

		assert.Equal(t, 1, len(q.nacks))
	})

	t.Run("maxNackCacheを超えると古いエントリを削除", func(t *testing.T) {
		q := newNACKQueue()

		// maxNackCache個以上pushする
		for i := 0; i < maxNackCache+10; i++ {
			q.push(uint32(i))
		}

		// maxNackCache-1個に制限される（trimIfFullの動作）
		assert.Less(t, len(q.nacks), maxNackCache+1)
		// 最も古いエントリ（小さいSN）が削除されている
		assert.Greater(t, q.nacks[0].sn, uint32(0))
	})
}

func TestNackQueue_remove(t *testing.T) {
	t.Run("存在するSNを削除", func(t *testing.T) {
		q := newNACKQueue()
		q.push(10)
		q.push(20)
		q.push(30)

		q.remove(20)

		require.Equal(t, 2, len(q.nacks))
		assert.Equal(t, uint32(10), q.nacks[0].sn)
		assert.Equal(t, uint32(30), q.nacks[1].sn)
	})

	t.Run("存在しないSNを削除しても何も起きない", func(t *testing.T) {
		q := newNACKQueue()
		q.push(10)
		q.push(30)

		q.remove(20) // 存在しない

		assert.Equal(t, 2, len(q.nacks))
	})

	t.Run("空のキューから削除しても何も起きない", func(t *testing.T) {
		q := newNACKQueue()

		q.remove(100)

		assert.Equal(t, 0, len(q.nacks))
	})

	t.Run("先頭のSNを削除", func(t *testing.T) {
		q := newNACKQueue()
		q.push(10)
		q.push(20)
		q.push(30)

		q.remove(10)

		require.Equal(t, 2, len(q.nacks))
		assert.Equal(t, uint32(20), q.nacks[0].sn)
	})

	t.Run("末尾のSNを削除", func(t *testing.T) {
		q := newNACKQueue()
		q.push(10)
		q.push(20)
		q.push(30)

		q.remove(30)

		require.Equal(t, 2, len(q.nacks))
		assert.Equal(t, uint32(20), q.nacks[1].sn)
	})
}

func TestNackQueue_find(t *testing.T) {
	q := newNACKQueue()
	q.push(10)
	q.push(20)
	q.push(30)
	q.push(40)

	tests := []struct {
		name     string
		sn       uint32
		expected int
	}{
		{"存在するSN（先頭）", 10, 0},
		{"存在するSN（中間）", 20, 1},
		{"存在するSN（末尾）", 40, 3},
		{"存在しないSN（範囲内）", 25, 2},  // 25以上の最初の位置
		{"存在しないSN（範囲外下）", 5, 0},  // 挿入位置は先頭
		{"存在しないSN（範囲外上）", 50, 4}, // 挿入位置は末尾の後
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := q.find(tt.sn)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNackQueue_pairs(t *testing.T) {
	t.Run("空のキュー", func(t *testing.T) {
		q := newNACKQueue()

		pairs, askKF := q.pairs(100)

		assert.Nil(t, pairs)
		assert.False(t, askKF)
	})

	t.Run("単一のNACK", func(t *testing.T) {
		q := newNACKQueue()
		q.push(50)

		pairs, askKF := q.pairs(100)

		require.Equal(t, 1, len(pairs))
		assert.Equal(t, uint16(50), pairs[0].PacketID)
		assert.Equal(t, rtcp.PacketBitmap(0), pairs[0].LostPackets)
		assert.False(t, askKF)
	})

	t.Run("連続したNACK（ビットマップ使用）", func(t *testing.T) {
		q := newNACKQueue()
		// 連続した5パケット
		for i := uint32(50); i < 55; i++ {
			q.push(i)
		}

		pairs, askKF := q.pairs(100)

		require.Equal(t, 1, len(pairs))
		assert.Equal(t, uint16(50), pairs[0].PacketID)
		// 51, 52, 53, 54 がビットマップに設定される
		// ビット0=51, ビット1=52, ビット2=53, ビット3=54
		assert.Equal(t, rtcp.PacketBitmap(0x0F), pairs[0].LostPackets)
		assert.False(t, askKF)
	})

	t.Run("離れたNACK（複数ペア）", func(t *testing.T) {
		q := newNACKQueue()
		q.push(50)
		q.push(100) // 50+16を超えているので新しいペア

		pairs, askKF := q.pairs(200)

		require.Equal(t, 2, len(pairs))
		assert.Equal(t, uint16(50), pairs[0].PacketID)
		assert.Equal(t, uint16(100), pairs[1].PacketID)
		assert.False(t, askKF)
	})

	t.Run("最近のパケットはNACKしない（headSN-2以上）", func(t *testing.T) {
		q := newNACKQueue()
		q.push(98)
		q.push(99)
		q.push(50) // これだけNACK対象

		pairs, askKF := q.pairs(100)

		require.Equal(t, 1, len(pairs))
		assert.Equal(t, uint16(50), pairs[0].PacketID)
		assert.False(t, askKF)
		// 98, 99はまだキューに残っている
		assert.Equal(t, 3, len(q.nacks))
	})

	t.Run("3回NACKでキーフレーム要求", func(t *testing.T) {
		q := newNACKQueue()
		q.push(50)

		// 3回pairsを呼び出してnacked回数を増やす
		q.pairs(100)             // nacked = 1
		q.pairs(100)             // nacked = 2
		q.pairs(100)             // nacked = 3
		_, askKF := q.pairs(100) // nacked >= maxNackTimes でexpired

		assert.True(t, askKF)
		// expiredしたエントリは削除される
		assert.Equal(t, 0, len(q.nacks))
	})

	t.Run("小さいSNではキーフレーム要求しない", func(t *testing.T) {
		q := newNACKQueue()
		q.push(50)
		q.push(60)

		// 50と60を3回NACKしてexpire
		q.pairs(100)
		q.pairs(100)
		q.pairs(100)
		_, askKF1 := q.pairs(100)

		assert.True(t, askKF1)
		// 50と60の両方がexpireし、60がkfSNに設定される
		assert.Equal(t, uint32(60), q.kfSN)

		// kfSNより小さいSNではキーフレーム要求しない
		q.push(40)
		q.pairs(100)
		q.pairs(100)
		q.pairs(100)
		_, askKF2 := q.pairs(100)

		assert.False(t, askKF2) // 40 < kfSN(60) なので要求しない
	})
}

func TestNackQueue_classifyNack(t *testing.T) {
	q := newNACKQueue()

	tests := []struct {
		name     string
		nck      nack
		headSN   uint32
		expected nackStatus
	}{
		{
			name:     "NACK回数上限到達",
			nck:      nack{sn: 50, nacked: 3},
			headSN:   100,
			expected: nackExpired,
		},
		{
			name:     "最近のパケット（headSN-1）",
			nck:      nack{sn: 99, nacked: 0},
			headSN:   100,
			expected: nackTooRecent,
		},
		{
			name:     "最近のパケット（headSN-2）",
			nck:      nack{sn: 98, nacked: 0},
			headSN:   100,
			expected: nackTooRecent,
		},
		{
			name:     "NACK対象",
			nck:      nack{sn: 97, nacked: 0},
			headSN:   100,
			expected: nackPending,
		},
		{
			name:     "NACK対象（nacked回数1）",
			nck:      nack{sn: 50, nacked: 1},
			headSN:   100,
			expected: nackPending,
		},
		{
			name:     "NACK対象（nacked回数2）",
			nck:      nack{sn: 50, nacked: 2},
			headSN:   100,
			expected: nackPending,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := q.classifyNack(tt.nck, tt.headSN)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNackQueue_checkKeyframeRequest(t *testing.T) {
	t.Run("初回のキーフレーム要求", func(t *testing.T) {
		q := newNACKQueue()
		nck := nack{sn: 50, nacked: 3}

		result := q.checkKeyframeRequest(nck)

		assert.True(t, result)
		assert.Equal(t, uint32(50), q.kfSN)
	})

	t.Run("より大きいSNでキーフレーム要求", func(t *testing.T) {
		q := newNACKQueue()
		q.kfSN = 50
		nck := nack{sn: 60, nacked: 3}

		result := q.checkKeyframeRequest(nck)

		assert.True(t, result)
		assert.Equal(t, uint32(60), q.kfSN)
	})

	t.Run("同じSNではキーフレーム要求しない", func(t *testing.T) {
		q := newNACKQueue()
		q.kfSN = 50
		nck := nack{sn: 50, nacked: 3}

		result := q.checkKeyframeRequest(nck)

		assert.False(t, result)
	})

	t.Run("小さいSNではキーフレーム要求しない", func(t *testing.T) {
		q := newNACKQueue()
		q.kfSN = 50
		nck := nack{sn: 40, nacked: 3}

		result := q.checkKeyframeRequest(nck)

		assert.False(t, result)
	})
}

func TestNackPairBuilder_add(t *testing.T) {
	t.Run("単一のSN", func(t *testing.T) {
		b := &nackPairBuilder{}

		b.add(100)
		pairs := b.build()

		require.Equal(t, 1, len(pairs))
		assert.Equal(t, uint16(100), pairs[0].PacketID)
		assert.Equal(t, rtcp.PacketBitmap(0), pairs[0].LostPackets)
	})

	t.Run("連続したSN（ビットマップ）", func(t *testing.T) {
		b := &nackPairBuilder{}

		b.add(100)
		b.add(101) // ビット0
		b.add(102) // ビット1
		b.add(103) // ビット2
		pairs := b.build()

		require.Equal(t, 1, len(pairs))
		assert.Equal(t, uint16(100), pairs[0].PacketID)
		assert.Equal(t, rtcp.PacketBitmap(0x07), pairs[0].LostPackets) // 0b111
	})

	t.Run("16を超える差は新しいペア", func(t *testing.T) {
		b := &nackPairBuilder{}

		b.add(100)
		b.add(117) // 100+17 > 100+16なので新しいペア
		pairs := b.build()

		require.Equal(t, 2, len(pairs))
		assert.Equal(t, uint16(100), pairs[0].PacketID)
		assert.Equal(t, uint16(117), pairs[1].PacketID)
	})

	t.Run("ビットマップの境界（PacketID+16）", func(t *testing.T) {
		b := &nackPairBuilder{}

		b.add(100)
		b.add(116) // 100+16は同じペア（ビット15）
		pairs := b.build()

		require.Equal(t, 1, len(pairs))
		assert.Equal(t, uint16(100), pairs[0].PacketID)
		assert.Equal(t, rtcp.PacketBitmap(1<<15), pairs[0].LostPackets)
	})

	t.Run("飛び飛びのSN", func(t *testing.T) {
		b := &nackPairBuilder{}

		b.add(100)
		b.add(102) // ビット1
		b.add(105) // ビット4
		b.add(110) // ビット9
		pairs := b.build()

		require.Equal(t, 1, len(pairs))
		assert.Equal(t, uint16(100), pairs[0].PacketID)
		// ビット1, 4, 9 = 0b1000010010 = 0x212
		assert.Equal(t, rtcp.PacketBitmap(0x212), pairs[0].LostPackets)
	})
}

func TestNackQueue_trimIfFull(t *testing.T) {
	q := newNACKQueue()

	// maxNackCache個まで追加
	for i := 0; i < maxNackCache; i++ {
		q.nacks = append(q.nacks, nack{sn: uint32(i), nacked: 0})
	}

	// trimIfFullを呼び出す
	q.trimIfFull()

	// 1つ削除される
	assert.Equal(t, maxNackCache-1, len(q.nacks))
	// 先頭（最も古い）が削除される
	assert.Equal(t, uint32(1), q.nacks[0].sn)
}

func TestNackQueue_insertAt(t *testing.T) {
	t.Run("空のキューに挿入", func(t *testing.T) {
		q := newNACKQueue()

		q.insertAt(0, 100)

		require.Equal(t, 1, len(q.nacks))
		assert.Equal(t, uint32(100), q.nacks[0].sn)
	})

	t.Run("末尾に挿入", func(t *testing.T) {
		q := newNACKQueue()
		q.nacks = append(q.nacks, nack{sn: 10})
		q.nacks = append(q.nacks, nack{sn: 20})

		q.insertAt(2, 30)

		require.Equal(t, 3, len(q.nacks))
		assert.Equal(t, uint32(30), q.nacks[2].sn)
	})

	t.Run("中間に挿入", func(t *testing.T) {
		q := newNACKQueue()
		q.nacks = append(q.nacks, nack{sn: 10})
		q.nacks = append(q.nacks, nack{sn: 30})

		q.insertAt(1, 20)

		require.Equal(t, 3, len(q.nacks))
		assert.Equal(t, uint32(10), q.nacks[0].sn)
		assert.Equal(t, uint32(20), q.nacks[1].sn)
		assert.Equal(t, uint32(30), q.nacks[2].sn)
	})

	t.Run("先頭に挿入", func(t *testing.T) {
		q := newNACKQueue()
		q.nacks = append(q.nacks, nack{sn: 20})
		q.nacks = append(q.nacks, nack{sn: 30})

		q.insertAt(0, 10)

		require.Equal(t, 3, len(q.nacks))
		assert.Equal(t, uint32(10), q.nacks[0].sn)
		assert.Equal(t, uint32(20), q.nacks[1].sn)
		assert.Equal(t, uint32(30), q.nacks[2].sn)
	})
}
