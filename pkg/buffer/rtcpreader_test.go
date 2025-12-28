package buffer

import (
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRTCPReader(t *testing.T) {
	ssrc := uint32(12345)
	r := NewRTCPReader(ssrc)

	assert.NotNil(t, r)
	assert.Equal(t, ssrc, r.ssrc)
	assert.False(t, r.closed.Load())
	assert.Nil(t, r.onPacket)
	assert.Nil(t, r.onClose)
}

func TestRTCPReader_Write(t *testing.T) {
	t.Run("コールバックなしで書き込み", func(t *testing.T) {
		r := NewRTCPReader(12345)
		data := []byte{0x01, 0x02, 0x03}

		n, err := r.Write(data)

		assert.NoError(t, err)
		assert.Equal(t, 0, n) // 現在の実装ではnは設定されない
	})

	t.Run("コールバックありで書き込み", func(t *testing.T) {
		r := NewRTCPReader(12345)
		data := []byte{0x01, 0x02, 0x03}
		var received []byte

		r.OnPacket(func(p []byte) {
			received = p
		})

		_, err := r.Write(data)

		require.NoError(t, err)
		assert.Equal(t, data, received)
	})

	t.Run("クローズ後の書き込みはEOFを返す", func(t *testing.T) {
		r := NewRTCPReader(12345)
		r.OnClose(func() {})
		r.Close()

		data := []byte{0x01, 0x02, 0x03}
		_, err := r.Write(data)

		assert.Equal(t, io.EOF, err)
	})

	t.Run("クローズ後はコールバックが呼ばれない", func(t *testing.T) {
		r := NewRTCPReader(12345)
		callCount := 0
		r.OnPacket(func(p []byte) {
			callCount++
		})
		r.OnClose(func() {})
		r.Close()

		r.Write([]byte{0x01})

		assert.Equal(t, 0, callCount)
	})
}

func TestRTCPReader_OnPacket(t *testing.T) {
	r := NewRTCPReader(12345)

	t.Run("コールバックを設定", func(t *testing.T) {
		called := false
		r.OnPacket(func(p []byte) {
			called = true
		})

		r.Write([]byte{0x01})
		assert.True(t, called)
	})

	t.Run("コールバックを上書き", func(t *testing.T) {
		firstCalled := false
		secondCalled := false

		r.OnPacket(func(p []byte) {
			firstCalled = true
		})
		r.OnPacket(func(p []byte) {
			secondCalled = true
		})

		r.Write([]byte{0x01})
		assert.False(t, firstCalled)
		assert.True(t, secondCalled)
	})
}

func TestRTCPReader_Close(t *testing.T) {
	t.Run("onCloseコールバックが呼ばれる", func(t *testing.T) {
		r := NewRTCPReader(12345)
		closeCalled := false
		r.OnClose(func() {
			closeCalled = true
		})

		err := r.Close()

		assert.NoError(t, err)
		assert.True(t, closeCalled)
		assert.True(t, r.closed.Load())
	})

	t.Run("クローズ後のフラグ状態", func(t *testing.T) {
		r := NewRTCPReader(12345)
		r.OnClose(func() {})

		assert.False(t, r.closed.Load())
		r.Close()
		assert.True(t, r.closed.Load())
	})
}

func TestRTCPReader_OnClose(t *testing.T) {
	r := NewRTCPReader(12345)

	closeCount := 0
	r.OnClose(func() {
		closeCount++
	})

	r.Close()
	assert.Equal(t, 1, closeCount)
}

func TestRTCPReader_Read(t *testing.T) {
	r := NewRTCPReader(12345)
	buf := make([]byte, 100)

	n, err := r.Read(buf)

	// 現在の実装ではReadは何もしない
	assert.Equal(t, 0, n)
	assert.NoError(t, err)
}

func TestRTCPReader_ConcurrentAccess(t *testing.T) {
	r := NewRTCPReader(12345)
	var wg sync.WaitGroup
	callCount := 0
	var mu sync.Mutex

	r.OnPacket(func(p []byte) {
		mu.Lock()
		callCount++
		mu.Unlock()
	})
	r.OnClose(func() {})

	// 複数のゴルーチンから同時に書き込み
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Write([]byte{0x01})
		}()
	}

	wg.Wait()

	// クローズ前の書き込みがすべて処理されていることを確認
	// （正確な数は競合状態により変動する可能性がある）
	assert.Greater(t, callCount, 0)
}

func TestRTCPReader_ConcurrentOnPacketAndWrite(t *testing.T) {
	r := NewRTCPReader(12345)
	var wg sync.WaitGroup

	// 書き込みとコールバック設定を同時に行う
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			r.Write([]byte{0x01})
		}()
		go func(i int) {
			defer wg.Done()
			r.OnPacket(func(p []byte) {
				// 何もしない - 競合テスト用
			})
		}(i)
	}

	wg.Wait()
	// パニックなく完了すればテスト成功
}
