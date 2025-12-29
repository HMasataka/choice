package sfu

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPacketMeta_VP8PayloadMeta(t *testing.T) {
	t.Run("設定と取得", func(t *testing.T) {
		pm := &PacketMeta{}

		pm.setVP8PayloadMeta(0x12, 0x3456)

		tlz0Idx, picID := pm.getVP8PayloadMeta()
		assert.Equal(t, uint8(0x12), tlz0Idx)
		assert.Equal(t, uint16(0x3456), picID)
	})

	t.Run("境界値", func(t *testing.T) {
		pm := &PacketMeta{}

		// 最大値
		pm.setVP8PayloadMeta(0xFF, 0xFFFF)
		tlz0Idx, picID := pm.getVP8PayloadMeta()
		assert.Equal(t, uint8(0xFF), tlz0Idx)
		assert.Equal(t, uint16(0xFFFF), picID)

		// 最小値
		pm.setVP8PayloadMeta(0x00, 0x0000)
		tlz0Idx, picID = pm.getVP8PayloadMeta()
		assert.Equal(t, uint8(0x00), tlz0Idx)
		assert.Equal(t, uint16(0x0000), picID)
	})
}

func TestNewSequencer(t *testing.T) {
	seq := newSequencer(100)

	assert.NotNil(t, seq)
	assert.Equal(t, 100, seq.max)
	assert.Len(t, seq.seq, 100)
	assert.False(t, seq.init)
	assert.Equal(t, 0, seq.step)
	assert.Greater(t, seq.startTime, int64(0))
}

func TestSequencer_Push(t *testing.T) {
	t.Run("初回push", func(t *testing.T) {
		seq := newSequencer(100)

		pm := seq.push(100, 200, 1000, 0, true)

		require.NotNil(t, pm)
		assert.True(t, seq.init)
		assert.Equal(t, uint16(100), pm.sourceSeqNo)
		assert.Equal(t, uint16(200), pm.targetSeqNo)
		assert.Equal(t, uint32(1000), pm.timestamp)
		assert.Equal(t, uint8(0), pm.layer)
	})

	t.Run("連続したpush", func(t *testing.T) {
		seq := newSequencer(100)

		seq.push(100, 200, 1000, 0, true)
		pm := seq.push(101, 201, 1001, 1, true)

		require.NotNil(t, pm)
		assert.Equal(t, uint16(101), pm.sourceSeqNo)
		assert.Equal(t, uint16(201), pm.targetSeqNo)
		assert.Equal(t, 2, seq.step)
	})

	t.Run("ギャップのあるpush", func(t *testing.T) {
		seq := newSequencer(100)

		seq.push(100, 200, 1000, 0, true)
		// 201, 202をスキップして203を追加
		pm := seq.push(103, 203, 1003, 0, true)

		require.NotNil(t, pm)
		assert.Equal(t, uint16(203), seq.headSN)
		// step: 1 (200) + 3 (ギャップ分) = 4
		assert.Equal(t, 4, seq.step)
	})

	t.Run("遅延パケットのpush（head=false）", func(t *testing.T) {
		seq := newSequencer(100)

		// 200, 202, 203を追加（201は欠落）
		seq.push(100, 200, 1000, 0, true)
		seq.push(102, 202, 1002, 0, true)
		seq.push(103, 203, 1003, 0, true)

		// 遅延して201を追加
		pm := seq.push(101, 201, 1001, 0, false)

		require.NotNil(t, pm)
		assert.Equal(t, uint16(101), pm.sourceSeqNo)
		assert.Equal(t, uint16(201), pm.targetSeqNo)
	})

	t.Run("範囲外の遅延パケット", func(t *testing.T) {
		seq := newSequencer(10)

		// 20個以上のパケットを追加してバッファを大きく進める
		for i := uint16(0); i < 25; i++ {
			seq.push(100+i, 200+i, 1000+uint32(i), 0, true)
		}

		// 古すぎるパケット（バッファ範囲外）
		// headSN=224, バッファサイズ10なので、214以前は範囲外
		pm := seq.push(100, 200, 1000, 0, false)

		assert.Nil(t, pm)
	})
}

func TestSequencer_CalculateIndex(t *testing.T) {
	t.Run("headパケット", func(t *testing.T) {
		seq := newSequencer(100)
		seq.init = true
		seq.headSN = 100
		seq.step = 5

		idx, ok := seq.calculateIndex(101, true)

		assert.True(t, ok)
		// calculateHeadIndexは現在のstepを返し、その後advanceStepが呼ばれる
		assert.Equal(t, 5, idx)
	})

	t.Run("遅延パケット（範囲内）", func(t *testing.T) {
		seq := newSequencer(100)
		seq.init = true
		seq.headSN = 105
		seq.step = 10

		idx, ok := seq.calculateIndex(103, false)

		assert.True(t, ok)
		assert.Equal(t, 8, idx) // step - (headSN - offSn) = 10 - 2 = 8
	})

	t.Run("遅延パケット（ラップアラウンド）", func(t *testing.T) {
		seq := newSequencer(100)
		seq.init = true
		seq.headSN = 105
		seq.step = 2 // stepが小さい

		idx, ok := seq.calculateIndex(103, false)

		assert.True(t, ok)
		// step - offset = 2 - 2 = 0
		assert.Equal(t, 0, idx)
	})
}

func TestSequencer_AdvanceStep(t *testing.T) {
	t.Run("通常の進行", func(t *testing.T) {
		seq := newSequencer(100)
		seq.step = 50

		seq.advanceStep()

		assert.Equal(t, 51, seq.step)
	})

	t.Run("ラップアラウンド", func(t *testing.T) {
		seq := newSequencer(100)
		seq.step = 99

		seq.advanceStep()

		assert.Equal(t, 0, seq.step)
	})
}

func TestSequencer_GetSeqNoPairs(t *testing.T) {
	t.Run("存在するシーケンス番号", func(t *testing.T) {
		seq := newSequencer(100)

		// パケットを追加
		for i := uint16(0); i < 5; i++ {
			seq.push(100+i, 200+i, 1000+uint32(i), uint8(i%3), true)
		}

		// 存在するシーケンス番号を検索
		metas := seq.getSeqNoPairs([]uint16{200, 202, 204})

		assert.Len(t, metas, 3)
		assert.Equal(t, uint16(100), metas[0].sourceSeqNo)
		assert.Equal(t, uint16(102), metas[1].sourceSeqNo)
		assert.Equal(t, uint16(104), metas[2].sourceSeqNo)
	})

	t.Run("存在しないシーケンス番号", func(t *testing.T) {
		seq := newSequencer(100)

		seq.push(100, 200, 1000, 0, true)

		// 存在しないシーケンス番号を検索
		metas := seq.getSeqNoPairs([]uint16{201, 202})

		assert.Len(t, metas, 0)
	})

	t.Run("再送抑制", func(t *testing.T) {
		seq := newSequencer(100)

		// 少し待ってからpush（startTimeからの経過時間を確保）
		time.Sleep(10 * time.Millisecond)

		seq.push(100, 200, 1000, 0, true)

		// 1回目のNACK
		metas1 := seq.getSeqNoPairs([]uint16{200})
		require.Len(t, metas1, 1)

		// seq.seq内のlastNackが更新されていることを確認
		idx, ok := seq.calculateIndexForLookup(200)
		require.True(t, ok)
		// 内部のseqのlastNackが更新されている（startTimeからの経過時間）
		assert.GreaterOrEqual(t, seq.seq[idx].lastNack, uint32(10))

		// すぐに2回目のNACK（抑制期間内、100ms以内）
		metas2 := seq.getSeqNoPairs([]uint16{200})
		// 2回目は抑制される
		assert.Len(t, metas2, 0)
	})

	t.Run("再送抑制期間後", func(t *testing.T) {
		seq := newSequencer(100)
		// startTimeを過去に設定して抑制期間をシミュレート
		seq.startTime = time.Now().UnixNano()/1e6 - 200

		seq.push(100, 200, 1000, 0, true)

		// 1回目のNACK
		metas1 := seq.getSeqNoPairs([]uint16{200})
		require.Len(t, metas1, 1)

		// 抑制期間を超えるまで待つ
		time.Sleep(150 * time.Millisecond)

		// 2回目のNACK（抑制期間後）
		metas2 := seq.getSeqNoPairs([]uint16{200})
		assert.Len(t, metas2, 1)
	})
}

func TestSequencer_ShouldRetransmit(t *testing.T) {
	seq := newSequencer(100)

	t.Run("初回NACK", func(t *testing.T) {
		pm := &PacketMeta{lastNack: 0}
		assert.True(t, seq.shouldRetransmit(pm, 100))
	})

	t.Run("抑制期間内", func(t *testing.T) {
		pm := &PacketMeta{lastNack: 100}
		assert.False(t, seq.shouldRetransmit(pm, 150)) // 50ms後
	})

	t.Run("抑制期間後", func(t *testing.T) {
		pm := &PacketMeta{lastNack: 100}
		assert.True(t, seq.shouldRetransmit(pm, 250)) // 150ms後
	})

	t.Run("ちょうど抑制期間", func(t *testing.T) {
		pm := &PacketMeta{lastNack: 100}
		assert.False(t, seq.shouldRetransmit(pm, 200)) // 100ms後（境界）
	})

	t.Run("抑制期間+1ms", func(t *testing.T) {
		pm := &PacketMeta{lastNack: 100}
		assert.True(t, seq.shouldRetransmit(pm, 201)) // 101ms後
	})
}

func TestSequencer_WrapAround(t *testing.T) {
	t.Run("シーケンス番号のラップアラウンド", func(t *testing.T) {
		seq := newSequencer(100)

		// 65534から開始
		seq.push(65534, 65534, 1000, 0, true)
		seq.push(65535, 65535, 1001, 0, true)
		seq.push(0, 0, 1002, 0, true) // ラップアラウンド
		seq.push(1, 1, 1003, 0, true)

		assert.Equal(t, uint16(1), seq.headSN)

		// 各パケットが正しく保存されていることを確認
		metas := seq.getSeqNoPairs([]uint16{65534, 65535, 0, 1})
		assert.Len(t, metas, 4)
	})
}

func TestSequencer_ConcurrentAccess(t *testing.T) {
	seq := newSequencer(1000)

	done := make(chan struct{})

	// 書き込みゴルーチン
	go func() {
		for i := uint16(0); i < 500; i++ {
			seq.push(i, i, uint32(i), 0, true)
		}
		close(done)
	}()

	// 読み取りゴルーチン
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				seq.getSeqNoPairs([]uint16{100, 200, 300})
			}
		}
	}()

	<-done
	// パニックなく完了すればテスト成功
}
