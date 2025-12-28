package buffer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestBucket(size int) *Bucket {
	buf := make([]byte, size)
	return NewBucket(&buf)
}

func createRTPPacket(seqNo uint16, payload []byte) []byte {
	// 簡易RTPパケット: 12バイトヘッダ + ペイロード
	pkt := make([]byte, 12+len(payload))
	pkt[0] = 0x80 // V=2
	pkt[1] = 96   // PT=96
	pkt[2] = byte(seqNo >> 8)
	pkt[3] = byte(seqNo)
	// timestamp, ssrc は0
	copy(pkt[12:], payload)
	return pkt
}

func TestNewBucket(t *testing.T) {
	buf := make([]byte, 15000) // 10 * maxPktSize
	b := NewBucket(&buf)

	assert.NotNil(t, b)
	assert.Equal(t, 15000, len(b.buf))
	assert.Equal(t, 9, b.maxSteps) // floor(15000/1500) - 1
	assert.False(t, b.init)
	assert.Equal(t, 0, b.step)
}

func TestBucket_AddPacket_First(t *testing.T) {
	b := createTestBucket(15000)
	pkt := createRTPPacket(100, []byte{0x01, 0x02, 0x03})

	result, err := b.AddPacket(pkt, 100, true)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, b.init)
	assert.Equal(t, uint16(100), b.headSequenceNumber)
}

func TestBucket_AddPacket_Sequential(t *testing.T) {
	b := createTestBucket(15000)

	// 連続した3パケットを追加
	for i := uint16(0); i < 3; i++ {
		pkt := createRTPPacket(100+i, []byte{byte(i)})
		_, err := b.AddPacket(pkt, 100+i, true)
		require.NoError(t, err)
	}

	assert.Equal(t, uint16(102), b.headSequenceNumber)
	assert.Equal(t, 3, b.step)
}

func TestBucket_AddPacket_WithGap(t *testing.T) {
	b := createTestBucket(15000)

	// パケット100を追加
	pkt1 := createRTPPacket(100, []byte{0x01})
	_, err := b.AddPacket(pkt1, 100, true)
	require.NoError(t, err)

	// パケット103を追加（101, 102をスキップ）
	pkt2 := createRTPPacket(103, []byte{0x02})
	_, err = b.AddPacket(pkt2, 103, true)
	require.NoError(t, err)

	assert.Equal(t, uint16(103), b.headSequenceNumber)
	// step: 1 (100) + 3 (ギャップ分) = 4
	assert.Equal(t, 4, b.step)
}

func TestBucket_AddPacket_Late(t *testing.T) {
	b := createTestBucket(15000)

	// パケット100, 101, 102を追加
	for i := uint16(0); i < 3; i++ {
		pkt := createRTPPacket(100+i, []byte{byte(i)})
		_, err := b.AddPacket(pkt, 100+i, true)
		require.NoError(t, err)
	}

	// 遅延パケット99を追加（latest=false）
	pkt := createRTPPacket(99, []byte{0xFF})
	result, err := b.AddPacket(pkt, 99, false)

	require.NoError(t, err)
	assert.NotNil(t, result)
	// headSequenceNumberは変わらない
	assert.Equal(t, uint16(102), b.headSequenceNumber)
}

func TestBucket_AddPacket_TooOld(t *testing.T) {
	b := createTestBucket(15000)

	// パケットを多数追加してバッファを進める
	for i := uint16(0); i < 20; i++ {
		pkt := createRTPPacket(100+i, []byte{byte(i)})
		_, err := b.AddPacket(pkt, 100+i, true)
		require.NoError(t, err)
	}

	// 古すぎるパケット（バッファ範囲外）
	pkt := createRTPPacket(90, []byte{0xFF})
	_, err := b.AddPacket(pkt, 90, false)

	assert.Equal(t, errPacketTooOld, err)
}

func TestBucket_AddPacket_Duplicate(t *testing.T) {
	b := createTestBucket(15000)

	// パケット100を追加
	pkt := createRTPPacket(100, []byte{0x01})
	_, err := b.AddPacket(pkt, 100, true)
	require.NoError(t, err)

	// 同じシーケンス番号で再度追加（latest=false）
	pkt2 := createRTPPacket(100, []byte{0x02})
	_, err = b.AddPacket(pkt2, 100, false)

	assert.Equal(t, errRTXPacket, err)
}

func TestBucket_GetPacket(t *testing.T) {
	b := createTestBucket(15000)

	// パケットを追加
	payload := []byte{0x01, 0x02, 0x03, 0x04}
	pkt := createRTPPacket(100, payload)
	_, err := b.AddPacket(pkt, 100, true)
	require.NoError(t, err)

	// パケットを取得
	buf := make([]byte, 1500)
	n, err := b.GetPacket(buf, 100)

	require.NoError(t, err)
	assert.Equal(t, len(pkt), n)
	assert.Equal(t, pkt, buf[:n])
}

func TestBucket_GetPacket_NotFound(t *testing.T) {
	b := createTestBucket(15000)

	// パケット100を追加
	pkt := createRTPPacket(100, []byte{0x01})
	_, err := b.AddPacket(pkt, 100, true)
	require.NoError(t, err)

	// 存在しないパケットを取得
	buf := make([]byte, 1500)
	_, err = b.GetPacket(buf, 101)

	assert.Equal(t, errPacketNotFound, err)
}

func TestBucket_GetPacket_BufferTooSmall(t *testing.T) {
	b := createTestBucket(15000)

	// 大きめのパケットを追加
	payload := make([]byte, 100)
	pkt := createRTPPacket(100, payload)
	_, err := b.AddPacket(pkt, 100, true)
	require.NoError(t, err)

	// 小さすぎるバッファで取得
	buf := make([]byte, 10)
	_, err = b.GetPacket(buf, 100)

	assert.Equal(t, errBufferTooSmall, err)
}

func TestBucket_GetPacket_AfterWrapAround(t *testing.T) {
	b := createTestBucket(15000) // maxSteps = 9

	// 10パケット追加（リングバッファが一周）
	for i := uint16(0); i < 10; i++ {
		pkt := createRTPPacket(100+i, []byte{byte(i)})
		_, err := b.AddPacket(pkt, 100+i, true)
		require.NoError(t, err)
	}

	// 最新のパケット（109）は取得可能
	buf := make([]byte, 1500)
	n, err := b.GetPacket(buf, 109)
	require.NoError(t, err)
	assert.Greater(t, n, 0)

	// 最も古いパケット（100）は上書きされている可能性
	_, err = b.GetPacket(buf, 100)
	// 上書きされていればerrPacketNotFoundになる
	if err != nil {
		assert.Equal(t, errPacketNotFound, err)
	}
}

func TestBucket_position(t *testing.T) {
	b := createTestBucket(15000) // maxSteps = 9

	// パケットを追加してstepを進める
	for i := uint16(0); i < 5; i++ {
		pkt := createRTPPacket(100+i, []byte{byte(i)})
		_, err := b.AddPacket(pkt, 100+i, true)
		require.NoError(t, err)
	}

	// headSequenceNumber=104, step=5

	t.Run("最新のパケット", func(t *testing.T) {
		pos, ok := b.position(104)
		assert.True(t, ok)
		assert.Equal(t, 4, pos) // step-1
	})

	t.Run("1つ前のパケット", func(t *testing.T) {
		pos, ok := b.position(103)
		assert.True(t, ok)
		assert.Equal(t, 3, pos)
	})

	t.Run("最も古いパケット", func(t *testing.T) {
		pos, ok := b.position(100)
		assert.True(t, ok)
		assert.Equal(t, 0, pos)
	})

	t.Run("範囲外（古すぎる）", func(t *testing.T) {
		// headSequenceNumber=104, step=5, maxSteps=9
		// back = 104 - 89 + 1 = 16
		// position = 5 - 16 = -11
		// -position = 11 > maxSteps+1 (10) → false
		_, ok := b.position(89)
		assert.False(t, ok)
	})
}

func TestBucket_position_WrapAround(t *testing.T) {
	b := createTestBucket(4500) // maxSteps = 2 (3スロット)

	// 5パケット追加（2周以上）
	for i := uint16(0); i < 5; i++ {
		pkt := createRTPPacket(100+i, []byte{byte(i)})
		_, err := b.AddPacket(pkt, 100+i, true)
		require.NoError(t, err)
	}

	// headSequenceNumber=104, step=5%3=2

	t.Run("最新のパケット", func(t *testing.T) {
		pos, ok := b.position(104)
		assert.True(t, ok)
		assert.Equal(t, 1, pos) // (step-1) = 1
	})

	t.Run("ラップアラウンド後の位置", func(t *testing.T) {
		pos, ok := b.position(103)
		assert.True(t, ok)
		assert.Equal(t, 0, pos)
	})
}

func TestBucket_advanceStep(t *testing.T) {
	b := createTestBucket(15000) // maxSteps = 9

	t.Run("通常のadvance", func(t *testing.T) {
		b.step = 0
		b.advanceStep(3)
		assert.Equal(t, 3, b.step)
	})

	t.Run("ラップアラウンド", func(t *testing.T) {
		b.step = 8
		b.advanceStep(5)
		// (8 + 5) % 10 = 3
		assert.Equal(t, 3, b.step)
	})

	t.Run("n=0は何もしない", func(t *testing.T) {
		b.step = 5
		b.advanceStep(0)
		assert.Equal(t, 5, b.step)
	})
}

func TestReadSequenceNumber(t *testing.T) {
	// RTPヘッダーのシーケンス番号はオフセット2-3
	buf := []byte{0x00, 0x00, 0x00, 0x00, 0x12, 0x34}
	// offset=0の場合、buf[4:6] = 0x1234

	sn := readSequenceNumber(buf, 0)
	assert.Equal(t, uint16(0x1234), sn)
}

func TestReadPacketSize(t *testing.T) {
	buf := []byte{0x01, 0x00, 0x00, 0x00}
	// offset=0の場合、buf[0:2] = 0x0100 = 256

	size := readPacketSize(buf, 0)
	assert.Equal(t, 256, size)
}

func TestBucket_push(t *testing.T) {
	b := createTestBucket(15000)
	b.init = true
	b.headSequenceNumber = 99

	pkt := createRTPPacket(100, []byte{0x01, 0x02})

	result := b.push(pkt)

	assert.NotNil(t, result)
	assert.Equal(t, pkt, result)
	assert.Equal(t, 1, b.step) // 0から1に進む
}

func TestBucket_get(t *testing.T) {
	b := createTestBucket(15000)

	// パケットを追加
	pkt := createRTPPacket(100, []byte{0xAB, 0xCD})
	_, err := b.AddPacket(pkt, 100, true)
	require.NoError(t, err)

	t.Run("存在するパケット", func(t *testing.T) {
		result := b.get(100)
		assert.Equal(t, pkt, result)
	})

	t.Run("存在しないパケット", func(t *testing.T) {
		result := b.get(101)
		assert.Nil(t, result)
	})

	t.Run("範囲外のパケット", func(t *testing.T) {
		result := b.get(50)
		assert.Nil(t, result)
	})
}

func TestBucket_set(t *testing.T) {
	b := createTestBucket(15000)

	// まず最新パケットを追加
	pkt1 := createRTPPacket(105, []byte{0x01})
	_, err := b.AddPacket(pkt1, 105, true)
	require.NoError(t, err)

	t.Run("遅延パケットを正常に設定", func(t *testing.T) {
		pkt := createRTPPacket(103, []byte{0x03})
		result, err := b.set(103, pkt)

		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("古すぎるパケット", func(t *testing.T) {
		pkt := createRTPPacket(50, []byte{0xFF})
		_, err := b.set(50, pkt)

		assert.Equal(t, errPacketTooOld, err)
	})

	t.Run("重複パケット", func(t *testing.T) {
		// 既に追加されているパケット
		pkt := createRTPPacket(105, []byte{0xFF})
		_, err := b.set(105, pkt)

		assert.Equal(t, errRTXPacket, err)
	})
}

func TestBucket_SequenceNumberWrapAround(t *testing.T) {
	b := createTestBucket(15000)

	// シーケンス番号65534から開始
	startSN := uint16(65534)
	for i := uint16(0); i < 5; i++ {
		sn := startSN + i // 65534, 65535, 0, 1, 2
		pkt := createRTPPacket(sn, []byte{byte(i)})
		_, err := b.AddPacket(pkt, sn, true)
		require.NoError(t, err)
	}

	// ラップアラウンド後のパケットが取得可能
	buf := make([]byte, 1500)
	n, err := b.GetPacket(buf, 0)
	require.NoError(t, err)
	assert.Greater(t, n, 0)
}
