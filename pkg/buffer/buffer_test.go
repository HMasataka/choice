package buffer

import (
	"io"
	"sync"
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestPools() (*sync.Pool, *sync.Pool) {
	videoPool := &sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 15000)
			return &buf
		},
	}
	audioPool := &sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 3000)
			return &buf
		},
	}
	return videoPool, audioPool
}

func createVideoCodecParams() webrtc.RTPCodecParameters {
	return webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  "video/VP8",
			ClockRate: 90000,
			RTCPFeedback: []webrtc.RTCPFeedback{
				{Type: webrtc.TypeRTCPFBNACK},
				{Type: webrtc.TypeRTCPFBGoogREMB},
			},
		},
		PayloadType: 96,
	}
}

func createAudioCodecParams() webrtc.RTPCodecParameters {
	return webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  "audio/opus",
			ClockRate: 48000,
		},
		PayloadType: 111,
	}
}

// packetNotifier tests
func TestNewPacketNotifier(t *testing.T) {
	var mu sync.Mutex
	pn := newPacketNotifier(&mu)

	assert.NotNil(t, pn)
	assert.NotNil(t, pn.cond)
}

func TestPacketNotifier_SignalAndWait(t *testing.T) {
	var mu sync.Mutex
	pn := newPacketNotifier(&mu)

	signaled := make(chan struct{})

	go func() {
		mu.Lock()
		defer mu.Unlock()
		pn.wait()
		close(signaled)
	}()

	// ゴルーチンがwaitに入る時間を待つ
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	pn.signal()
	mu.Unlock()

	select {
	case <-signaled:
		// 正常にシグナルを受信
	case <-time.After(100 * time.Millisecond):
		t.Fatal("signal was not received")
	}
}

func TestPacketNotifier_Broadcast(t *testing.T) {
	var mu sync.Mutex
	pn := newPacketNotifier(&mu)

	const numWaiters = 3
	signaled := make(chan struct{}, numWaiters)

	for i := 0; i < numWaiters; i++ {
		go func() {
			mu.Lock()
			defer mu.Unlock()
			pn.wait()
			signaled <- struct{}{}
		}()
	}

	// ゴルーチンがwaitに入る時間を待つ
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	pn.broadcast()
	mu.Unlock()

	for i := 0; i < numWaiters; i++ {
		select {
		case <-signaled:
			// 正常にシグナルを受信
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("waiter %d did not receive broadcast", i)
		}
	}
}

// NewBuffer tests
func TestNewBuffer(t *testing.T) {
	vp, ap := createTestPools()
	ssrc := uint32(12345)

	b := NewBuffer(ssrc, vp, ap)

	assert.NotNil(t, b)
	assert.Equal(t, ssrc, b.mediaSSRC)
	assert.Equal(t, vp, b.videoPool)
	assert.Equal(t, ap, b.audioPool)
	assert.NotNil(t, b.extPacketNotifier)
	assert.NotNil(t, b.pendingPacketNotifier)
	assert.False(t, b.bound)
	assert.False(t, b.closed.Load())
}

// Bind tests
func TestBuffer_Bind_Video(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	codec := createVideoCodecParams()
	params := webrtc.RTPParameters{
		Codecs: []webrtc.RTPCodecParameters{codec},
		HeaderExtensions: []webrtc.RTPHeaderExtensionParameter{
			{URI: sdp.TransportCCURI, ID: 5},
		},
	}

	b.Bind(params, Options{MaxBitRate: 1000000})

	assert.True(t, b.bound)
	assert.Equal(t, uint32(90000), b.clockRate)
	assert.Equal(t, uint64(1000000), b.maxBitrate)
	assert.Equal(t, "video/vp8", b.mime)
	assert.Equal(t, webrtc.RTPCodecTypeVideo, b.codecType)
	assert.NotNil(t, b.bucket)
	assert.True(t, b.nack)
	assert.True(t, b.remb)
	assert.Equal(t, uint8(5), b.twccExt)
}

func TestBuffer_Bind_Audio(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	codec := createAudioCodecParams()
	params := webrtc.RTPParameters{
		Codecs: []webrtc.RTPCodecParameters{codec},
		HeaderExtensions: []webrtc.RTPHeaderExtensionParameter{
			{URI: sdp.AudioLevelURI, ID: 3},
		},
	}

	b.Bind(params, Options{MaxBitRate: 64000})

	assert.True(t, b.bound)
	assert.Equal(t, uint32(48000), b.clockRate)
	assert.Equal(t, "audio/opus", b.mime)
	assert.Equal(t, webrtc.RTPCodecTypeAudio, b.codecType)
	assert.NotNil(t, b.bucket)
	assert.True(t, b.audioLevel)
	assert.Equal(t, uint8(3), b.audioExt)
}

func TestBuffer_Bind_TWCC(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	codec := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  "video/VP8",
			ClockRate: 90000,
			RTCPFeedback: []webrtc.RTCPFeedback{
				{Type: webrtc.TypeRTCPFBTransportCC},
			},
		},
		PayloadType: 96,
	}
	params := webrtc.RTPParameters{
		Codecs: []webrtc.RTPCodecParameters{codec},
		HeaderExtensions: []webrtc.RTPHeaderExtensionParameter{
			{URI: sdp.TransportCCURI, ID: 7},
		},
	}

	b.Bind(params, Options{MaxBitRate: 1000000})

	assert.True(t, b.twcc)
	assert.Equal(t, uint8(7), b.twccExt)
}

// Write tests
func TestBuffer_Write_BeforeBind(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	pkt := createRTPPacket(100, []byte{0x01, 0x02})

	_, err := b.Write(pkt)

	assert.NoError(t, err)
	assert.Len(t, b.pendingPackets, 1)
	assert.Equal(t, pkt, b.pendingPackets[0].packet)
}

func TestBuffer_Write_AfterBind(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	codec := createVideoCodecParams()
	params := webrtc.RTPParameters{
		Codecs: []webrtc.RTPCodecParameters{codec},
	}
	b.OnFeedback(func(fb []rtcp.Packet) {})
	b.Bind(params, Options{MaxBitRate: 1000000})

	// VP8ペイロード付きパケットを作成
	pkt := createVP8RTPPacket(100, true, []byte{0x01, 0x02})

	_, err := b.Write(pkt)

	assert.NoError(t, err)
	assert.Equal(t, uint32(1), b.stats.PacketCount)
}

func TestBuffer_Write_AfterClose(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)
	b.OnClose(func() {})
	b.Close()

	pkt := createRTPPacket(100, []byte{0x01})
	_, err := b.Write(pkt)

	assert.Equal(t, io.EOF, err)
}

// Read tests
func TestBuffer_Read_BeforeBind(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	pkt := createRTPPacket(100, []byte{0x01, 0x02})
	b.Write(pkt)

	buf := make([]byte, 1500)
	done := make(chan struct{})

	go func() {
		n, err := b.Read(buf)
		assert.NoError(t, err)
		assert.Equal(t, len(pkt), n)
		close(done)
	}()

	select {
	case <-done:
		// 正常に読み取り完了
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Read did not complete")
	}
}

func TestBuffer_Read_BufferTooSmall(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	pkt := createRTPPacket(100, []byte{0x01, 0x02, 0x03, 0x04, 0x05})
	b.Write(pkt)

	buf := make([]byte, 5)
	done := make(chan error)

	go func() {
		_, err := b.Read(buf)
		done <- err
	}()

	select {
	case err := <-done:
		assert.Equal(t, errBufferTooSmall, err)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Read did not complete")
	}
}

func TestBuffer_Read_AfterClose(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)
	b.OnClose(func() {})

	buf := make([]byte, 1500)
	done := make(chan error)

	go func() {
		_, err := b.Read(buf)
		done <- err
	}()

	// Readがwaitに入る時間を待つ
	time.Sleep(10 * time.Millisecond)
	b.Close()

	select {
	case err := <-done:
		assert.Equal(t, io.EOF, err)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Read did not complete after close")
	}
}

// ReadExtended tests
func TestBuffer_ReadExtended(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	codec := createVideoCodecParams()
	params := webrtc.RTPParameters{
		Codecs: []webrtc.RTPCodecParameters{codec},
	}
	b.OnFeedback(func(fb []rtcp.Packet) {})
	b.Bind(params, Options{MaxBitRate: 1000000})

	pkt := createVP8RTPPacket(100, true, []byte{0x01})
	b.Write(pkt)

	done := make(chan *ExtPacket)

	go func() {
		ep, err := b.ReadExtended()
		assert.NoError(t, err)
		done <- ep
	}()

	select {
	case ep := <-done:
		assert.NotNil(t, ep)
		assert.Equal(t, uint16(100), ep.Packet.SequenceNumber)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ReadExtended did not complete")
	}
}

func TestBuffer_ReadExtended_AfterClose(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)
	b.OnClose(func() {})

	done := make(chan error)

	go func() {
		_, err := b.ReadExtended()
		done <- err
	}()

	// ゴルーチンがwaitに入る時間を待つ
	time.Sleep(10 * time.Millisecond)
	b.Close()

	select {
	case err := <-done:
		assert.Equal(t, io.EOF, err)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ReadExtended did not complete after close")
	}
}

// Close tests
func TestBuffer_Close(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	closeCalled := false
	b.OnClose(func() {
		closeCalled = true
	})

	codec := createVideoCodecParams()
	params := webrtc.RTPParameters{
		Codecs: []webrtc.RTPCodecParameters{codec},
	}
	b.Bind(params, Options{MaxBitRate: 1000000})

	err := b.Close()

	assert.NoError(t, err)
	assert.True(t, b.closed.Load())
	assert.True(t, closeCalled)
}

func TestBuffer_Close_Multiple(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	closeCount := 0
	b.OnClose(func() {
		closeCount++
	})

	b.Close()
	b.Close()
	b.Close()

	// closeOnceにより1回のみ実行される
	assert.Equal(t, 1, closeCount)
}

// GetPacket tests
func TestBuffer_GetPacket(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	codec := createVideoCodecParams()
	params := webrtc.RTPParameters{
		Codecs: []webrtc.RTPCodecParameters{codec},
	}
	b.OnFeedback(func(fb []rtcp.Packet) {})
	b.Bind(params, Options{MaxBitRate: 1000000})

	pkt := createVP8RTPPacket(100, true, []byte{0x01, 0x02})
	b.Write(pkt)

	buf := make([]byte, 1500)
	n, err := b.GetPacket(buf, 100)

	require.NoError(t, err)
	assert.Greater(t, n, 0)
}

func TestBuffer_GetPacket_AfterClose(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)
	b.OnClose(func() {})

	codec := createVideoCodecParams()
	params := webrtc.RTPParameters{
		Codecs: []webrtc.RTPCodecParameters{codec},
	}
	b.Bind(params, Options{MaxBitRate: 1000000})
	b.Close()

	buf := make([]byte, 1500)
	_, err := b.GetPacket(buf, 100)

	assert.Equal(t, io.EOF, err)
}

// Getters tests
func TestBuffer_Getters(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	codec := createVideoCodecParams()
	params := webrtc.RTPParameters{
		Codecs: []webrtc.RTPCodecParameters{codec},
	}
	b.Bind(params, Options{MaxBitRate: 1000000})

	assert.Equal(t, uint32(12345), b.GetMediaSSRC())
	assert.Equal(t, uint32(90000), b.GetClockRate())
	assert.Equal(t, uint64(0), b.Bitrate())
	assert.Equal(t, int32(0), b.MaxTemporalLayer())
}

func TestBuffer_SetSenderReportData(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	rtpTime := uint32(1000)
	ntpTime := uint64(2000)

	b.SetSenderReportData(rtpTime, ntpTime)

	gotRTP, gotNTP, gotRecv := b.GetSenderReportData()

	assert.Equal(t, rtpTime, gotRTP)
	assert.Equal(t, ntpTime, gotNTP)
	assert.Greater(t, gotRecv, int64(0))
}

func TestBuffer_GetLatestTimestamp(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	codec := createVideoCodecParams()
	params := webrtc.RTPParameters{
		Codecs: []webrtc.RTPCodecParameters{codec},
	}
	b.OnFeedback(func(fb []rtcp.Packet) {})
	b.Bind(params, Options{MaxBitRate: 1000000})

	// パケットを書き込むことでタイムスタンプが更新される
	pkt := createVP8RTPPacket(100, true, []byte{0x01})
	b.Write(pkt)

	ts, tsTime := b.GetLatestTimestamp()

	// パケットのタイムスタンプは0なので0が返る
	assert.Equal(t, uint32(0), ts)
	assert.Greater(t, tsTime, int64(0))
}

func TestBuffer_GetStats(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	codec := createVideoCodecParams()
	params := webrtc.RTPParameters{
		Codecs: []webrtc.RTPCodecParameters{codec},
	}
	b.OnFeedback(func(fb []rtcp.Packet) {})
	b.Bind(params, Options{MaxBitRate: 1000000})

	pkt := createVP8RTPPacket(100, true, []byte{0x01, 0x02, 0x03})
	b.Write(pkt)

	stats := b.GetStats()

	assert.Equal(t, uint32(1), stats.PacketCount)
	assert.Greater(t, stats.TotalByte, uint64(0))
}

// Callback setters tests
func TestBuffer_OnTransportWideCC(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	b.OnTransportWideCC(func(sn uint16, timeNS int64, marker bool) {})

	assert.NotNil(t, b.feedbackTWCC)
}

func TestBuffer_OnFeedback(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	b.OnFeedback(func(fb []rtcp.Packet) {})

	assert.NotNil(t, b.feedbackCB)
}

func TestBuffer_OnAudioLevel(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	b.OnAudioLevel(func(level uint8) {})

	assert.NotNil(t, b.onAudioLevel)
}

// Timestamp functions tests
// IsLaterTimestamp は符号付き差分を使用してラップアラウンドを正しく処理する
func TestIsLaterTimestamp(t *testing.T) {
	tests := []struct {
		name     string
		ts1      uint32
		ts2      uint32
		expected bool
	}{
		{
			name:     "ts1 > ts2（通常）",
			ts1:      1000,
			ts2:      500,
			expected: true,
		},
		{
			name:     "ts1 < ts2（通常）",
			ts1:      500,
			ts2:      1000,
			expected: false,
		},
		{
			name:     "ts1 == ts2",
			ts1:      1000,
			ts2:      1000,
			expected: false,
		},
		{
			name:     "ラップアラウンド: ts1が0付近、ts2がMAX付近",
			ts1:      10,
			ts2:      0xFFFFFFF0, // 4294967280
			expected: true,       // 10は0xFFFFFFF0より後（ラップアラウンド考慮）
		},
		{
			name:     "ラップアラウンド: ts1がMAX付近、ts2が0付近",
			ts1:      0xFFFFFFF0,
			ts2:      10,
			expected: false, // 0xFFFFFFF0は10より前（ラップアラウンド考慮）
		},
		{
			name:     "境界: 差がちょうど0x80000000（2^31）",
			ts1:      0x80000000,
			ts2:      0,
			expected: false, // int32での差は-2^31で負
		},
		{
			name:     "境界: 差が0x7FFFFFFF（2^31-1）",
			ts1:      0x7FFFFFFF,
			ts2:      0,
			expected: true, // 差が正の最大値
		},
		{
			name:     "0と0",
			ts1:      0,
			ts2:      0,
			expected: false,
		},
		{
			name:     "最大値同士",
			ts1:      0xFFFFFFFF,
			ts2:      0xFFFFFFFF,
			expected: false,
		},
		{
			name:     "連続したタイムスタンプ",
			ts1:      101,
			ts2:      100,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsLaterTimestamp(tt.ts1, tt.ts2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper functions for creating test packets
func createVP8RTPPacket(seqNo uint16, keyframe bool, payload []byte) []byte {
	// RTPヘッダー（12バイト）
	rtpHeader := make([]byte, 12)
	rtpHeader[0] = 0x80 // V=2, P=0, X=0, CC=0
	rtpHeader[1] = 96   // M=0, PT=96
	rtpHeader[2] = byte(seqNo >> 8)
	rtpHeader[3] = byte(seqNo)
	// timestamp = 0
	// SSRC = 0

	// VP8ペイロードディスクリプタ
	vp8Desc := make([]byte, 1)
	if keyframe {
		vp8Desc[0] = 0x10 // S=1 (start of partition), PID=0
	} else {
		vp8Desc[0] = 0x00
	}

	// VP8ペイロードヘッダー（キーフレームの場合は0x9d 0x01 0x2a）
	var vp8Header []byte
	if keyframe {
		vp8Header = []byte{0x9d, 0x01, 0x2a, 0x00, 0x00} // キーフレーム開始バイト
	} else {
		vp8Header = []byte{0x00} // インターフレーム
	}

	pkt := make([]byte, 0, len(rtpHeader)+len(vp8Desc)+len(vp8Header)+len(payload))
	pkt = append(pkt, rtpHeader...)
	pkt = append(pkt, vp8Desc...)
	pkt = append(pkt, vp8Header...)
	pkt = append(pkt, payload...)

	return pkt
}

// adjustBitrateByLossRate tests
func TestBuffer_adjustBitrateByLossRate(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	t.Run("損失率2%未満: ビットレート増加", func(t *testing.T) {
		b.stats.LostRate = 0.01
		result := b.adjustBitrateByLossRate(1000000)

		// 1.09倍 + 2000
		expected := uint64(float64(1000000)*1.09) + 2000
		assert.Equal(t, expected, result)
	})

	t.Run("損失率10%超: ビットレート減少", func(t *testing.T) {
		b.stats.LostRate = 0.15
		result := b.adjustBitrateByLossRate(1000000)

		// (1 - 0.5*0.15) = 0.925
		expected := uint64(float64(1000000) * float64(1-0.5*0.15))
		assert.Equal(t, expected, result)
	})

	t.Run("損失率2-10%: ビットレート維持", func(t *testing.T) {
		b.stats.LostRate = 0.05
		result := b.adjustBitrateByLossRate(1000000)

		assert.Equal(t, uint64(1000000), result)
	})
}

// clampBitrate tests
func TestBuffer_clampBitrate(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)
	b.maxBitrate = 2000000

	t.Run("最大値超過", func(t *testing.T) {
		result := b.clampBitrate(3000000)
		assert.Equal(t, uint64(2000000), result)
	})

	t.Run("最小値未満", func(t *testing.T) {
		result := b.clampBitrate(50000)
		assert.Equal(t, uint64(minBufferSize), result)
	})

	t.Run("範囲内", func(t *testing.T) {
		result := b.clampBitrate(1500000)
		assert.Equal(t, uint64(1500000), result)
	})
}

// calcTotalLost tests
func TestBuffer_calcTotalLost(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	t.Run("パケットロスあり", func(t *testing.T) {
		b.stats.PacketCount = 90
		result := b.calcTotalLost(100)
		assert.Equal(t, uint32(10), result)
	})

	t.Run("パケットロスなし", func(t *testing.T) {
		b.stats.PacketCount = 100
		result := b.calcTotalLost(100)
		assert.Equal(t, uint32(0), result)
	})

	t.Run("PacketCountが0", func(t *testing.T) {
		b.stats.PacketCount = 0
		result := b.calcTotalLost(100)
		assert.Equal(t, uint32(0), result)
	})

	t.Run("expectedより多く受信", func(t *testing.T) {
		b.stats.PacketCount = 110
		result := b.calcTotalLost(100)
		assert.Equal(t, uint32(0), result)
	})
}

// calcFractionLost tests
func TestBuffer_calcFractionLost(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	t.Run("損失あり", func(t *testing.T) {
		b.stats.LastExpected = 0
		b.stats.LastReceived = 0
		b.stats.PacketCount = 90

		result := b.calcFractionLost(100)

		// (10 << 8) / 100 = 25
		assert.Equal(t, uint8(25), result)
		assert.Equal(t, uint32(100), b.stats.LastExpected)
		assert.Equal(t, uint32(90), b.stats.LastReceived)
	})

	t.Run("損失なし", func(t *testing.T) {
		b.stats.LastExpected = 100
		b.stats.LastReceived = 90
		b.stats.PacketCount = 100

		result := b.calcFractionLost(110)

		assert.Equal(t, uint8(0), result)
	})

	t.Run("expectedIntervalが0", func(t *testing.T) {
		b.stats.LastExpected = 100
		b.stats.LastReceived = 100
		b.stats.PacketCount = 100

		result := b.calcFractionLost(100)

		assert.Equal(t, uint8(0), result)
	})
}

// calcDLSR tests
func TestBuffer_calcDLSR(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	t.Run("SenderReportなし", func(t *testing.T) {
		result := b.calcDLSR()
		assert.Equal(t, uint32(0), result)
	})

	t.Run("SenderReport受信済み", func(t *testing.T) {
		// 100ms前にSRを受信したと設定
		b.SetSenderReportData(1000, 2000)

		// 少し待つ
		time.Sleep(10 * time.Millisecond)

		result := b.calcDLSR()
		assert.Greater(t, result, uint32(0))
	})
}

// extendedSequenceNumber tests
func TestBuffer_extendedSequenceNumber(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	t.Run("通常のケース", func(t *testing.T) {
		b.cycles = 0x10000 // 1サイクル
		b.maxSeqNo = 100

		result := b.extendedSequenceNumber(50)
		assert.Equal(t, uint32(0x10000|50), result)
	})

	t.Run("ラップアラウンド境界をまたぐ", func(t *testing.T) {
		b.cycles = 0x10000
		b.maxSeqNo = 100 // 小さい値

		// snがmaxSeqNoより大きく、境界をまたぐ場合
		result := b.extendedSequenceNumber(0x8100) // 上位ビットあり

		// isCrossingWrapAroundBoundaryがtrueの場合、cycles-maxSequenceNumberを使う
		expected := (b.cycles - maxSequenceNumber) | uint32(0x8100)
		assert.Equal(t, expected, result)
	})
}

// processPendingPackets tests
func TestBuffer_processPendingPackets(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	// バインド前にパケットを書き込む
	pkt1 := createRTPPacket(100, []byte{0x01})
	pkt2 := createRTPPacket(101, []byte{0x02})
	b.Write(pkt1)
	b.Write(pkt2)

	assert.Len(t, b.pendingPackets, 2)

	// バインドすると保留パケットが処理される
	codec := createVideoCodecParams()
	params := webrtc.RTPParameters{
		Codecs: []webrtc.RTPCodecParameters{codec},
	}
	b.OnFeedback(func(fb []rtcp.Packet) {})
	b.Bind(params, Options{MaxBitRate: 1000000})

	// pendingPacketsはクリアされる
	assert.Nil(t, b.pendingPackets)
}

// buildNACKPacket tests
func TestBuffer_buildNACKPacket(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	codec := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  "video/VP8",
			ClockRate: 90000,
			RTCPFeedback: []webrtc.RTCPFeedback{
				{Type: webrtc.TypeRTCPFBNACK},
			},
		},
		PayloadType: 96,
	}
	params := webrtc.RTPParameters{
		Codecs: []webrtc.RTPCodecParameters{codec},
	}
	b.OnFeedback(func(fb []rtcp.Packet) {})
	b.Bind(params, Options{MaxBitRate: 1000000})

	t.Run("NACKなし", func(t *testing.T) {
		result := b.buildNACKPacket()
		assert.Nil(t, result)
	})
}

// getRTCP tests
func TestBuffer_getRTCP(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	// stats設定
	b.cycles = 0
	b.maxSeqNo = 100
	b.baseSequenceNumber = 0
	b.stats.PacketCount = 100
	b.clockRate = 90000

	t.Run("REMBなしTWCCなし", func(t *testing.T) {
		b.remb = false
		b.twcc = false

		packets := b.getRTCP()

		assert.Len(t, packets, 1) // ReceiverReportのみ
	})

	t.Run("REMBありTWCCなし", func(t *testing.T) {
		b.remb = true
		b.twcc = false
		b.maxBitrate = 1000000
		b.bitrate = 500000

		packets := b.getRTCP()

		assert.Len(t, packets, 2) // ReceiverReport + REMB
	})

	t.Run("REMBありTWCCあり", func(t *testing.T) {
		b.remb = true
		b.twcc = true

		packets := b.getRTCP()

		assert.Len(t, packets, 1) // TWCCがあればREMBは送らない
	})
}

// buildREMBPacket tests
func TestBuffer_buildREMBPacket(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)
	b.mediaSSRC = 12345
	b.maxBitrate = 2000000
	b.bitrate = 1000000

	t.Run("損失率低い場合ビットレート増加", func(t *testing.T) {
		b.stats.LostRate = 0.01
		b.stats.TotalByte = 1000

		result := b.buildREMBPacket()

		assert.NotNil(t, result)
		assert.Equal(t, []uint32{uint32(12345)}, result.SSRCs)
		assert.Equal(t, uint64(0), b.stats.TotalByte) // リセットされる
	})
}

// buildReceptionReport tests
func TestBuffer_buildReceptionReport(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)
	b.mediaSSRC = 12345
	b.cycles = 0
	b.maxSeqNo = 100
	b.baseSequenceNumber = 0
	b.stats.PacketCount = 90
	b.stats.Jitter = 10.5
	b.clockRate = 90000

	report := b.buildReceptionReport()

	assert.Equal(t, uint32(12345), report.SSRC)
	assert.Equal(t, uint32(100), report.LastSequenceNumber)
	assert.Equal(t, uint32(10), report.Jitter)
	assert.Equal(t, uint32(11), report.TotalLost) // 101 - 90 = 11
}

// handleMissingForNACK tests
func TestBuffer_handleMissingForNACK(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	codec := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  "video/VP8",
			ClockRate: 90000,
			RTCPFeedback: []webrtc.RTCPFeedback{
				{Type: webrtc.TypeRTCPFBNACK},
			},
		},
		PayloadType: 96,
	}
	params := webrtc.RTPParameters{
		Codecs: []webrtc.RTPCodecParameters{codec},
	}
	b.OnFeedback(func(fb []rtcp.Packet) {})
	b.Bind(params, Options{MaxBitRate: 1000000})

	// 初期パケット
	b.maxSeqNo = 100
	b.cycles = 0

	// シーケンス番号ギャップを作成
	b.handleMissingForNACK(105)

	// 101, 102, 103, 104がNACKキューに追加される
	assert.Len(t, b.nacker.nacks, 4)
}

// updateJitter tests
func TestBuffer_updateJitter(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)
	b.clockRate = 90000

	t.Run("初回呼び出し", func(t *testing.T) {
		b.lastTransit = 0
		b.stats.Jitter = 0

		b.updateJitter(1000, time.Now().UnixNano())

		assert.Equal(t, float64(0), b.stats.Jitter)
		assert.NotEqual(t, uint32(0), b.lastTransit)
	})

	t.Run("2回目以降", func(t *testing.T) {
		b.lastTransit = 1000
		b.stats.Jitter = 0

		b.updateJitter(2000, time.Now().UnixNano())

		// ジッターが更新される
		assert.NotEqual(t, uint32(0), b.lastTransit)
	})
}

// handleTWCC tests
func TestBuffer_handleTWCC_WithExtension(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	codec := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  "video/VP8",
			ClockRate: 90000,
			RTCPFeedback: []webrtc.RTCPFeedback{
				{Type: webrtc.TypeRTCPFBTransportCC},
			},
		},
		PayloadType: 96,
	}
	params := webrtc.RTPParameters{
		Codecs: []webrtc.RTPCodecParameters{codec},
		HeaderExtensions: []webrtc.RTPHeaderExtensionParameter{
			{URI: sdp.TransportCCURI, ID: 5},
		},
	}

	var twccCalled bool
	var receivedSN uint16
	b.OnTransportWideCC(func(sn uint16, timeNS int64, marker bool) {
		twccCalled = true
		receivedSN = sn
	})
	b.OnFeedback(func(fb []rtcp.Packet) {})
	b.Bind(params, Options{MaxBitRate: 1000000})

	// TWCC拡張付きパケットを作成
	pkt := createRTPPacketWithTWCC(100, 5, 1234)
	b.Write(pkt)

	assert.True(t, twccCalled)
	assert.Equal(t, uint16(1234), receivedSN)
}

// handleAudioLevel tests
func TestBuffer_handleAudioLevel_WithExtension(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)

	codec := createAudioCodecParams()
	params := webrtc.RTPParameters{
		Codecs: []webrtc.RTPCodecParameters{codec},
		HeaderExtensions: []webrtc.RTPHeaderExtensionParameter{
			{URI: sdp.AudioLevelURI, ID: 3},
		},
	}

	var levelReceived uint8
	b.OnAudioLevel(func(level uint8) {
		levelReceived = level
	})
	b.OnFeedback(func(fb []rtcp.Packet) {})
	b.Bind(params, Options{MaxBitRate: 64000})

	// AudioLevel拡張付きパケットを作成
	pkt := createRTPPacketWithAudioLevel(100, 3, 50)
	b.Write(pkt)

	assert.Equal(t, uint8(50), levelReceived)
}

// initialProbe tests
func TestBuffer_initialProbe(t *testing.T) {
	vp, ap := createTestPools()
	b := NewBuffer(12345, vp, ap)
	b.mime = "video/vp8"
	b.baseSequenceNumber = 100
	b.minPacketProbe = 0

	t.Run("初期プローブ中", func(t *testing.T) {
		ep := &ExtPacket{
			Payload: VP8{TID: 2},
		}

		b.initialProbe(99, ep)

		// baseSequenceNumberが更新される
		assert.Equal(t, uint16(99), b.baseSequenceNumber)
		assert.Equal(t, int32(2), b.maxTemporalLayer)
		assert.Equal(t, 1, b.minPacketProbe)
	})

	t.Run("プローブ完了後", func(t *testing.T) {
		b.minPacketProbe = 25
		originalBase := b.baseSequenceNumber

		ep := &ExtPacket{
			Payload: VP8{TID: 3},
		}

		b.initialProbe(98, ep)

		// 何も更新されない
		assert.Equal(t, originalBase, b.baseSequenceNumber)
		assert.Equal(t, 25, b.minPacketProbe)
	})
}

// Helper: TWCC拡張付きRTPパケット
func createRTPPacketWithTWCC(seqNo uint16, extID uint8, twccSN uint16) []byte {
	// RTPヘッダー（12バイト）+ Extension
	// V=2, P=0, X=1 (extension present), CC=0
	rtpHeader := make([]byte, 12)
	rtpHeader[0] = 0x90 // V=2, X=1
	rtpHeader[1] = 96   // PT=96
	rtpHeader[2] = byte(seqNo >> 8)
	rtpHeader[3] = byte(seqNo)

	// Extension Header (RFC 5285 one-byte header)
	// Profile = 0xBEDE
	extHeader := []byte{
		0xBE, 0xDE, // Profile
		0x00, 0x01, // Length = 1 (4 bytes)
	}

	// Extension data: ID (4 bits) + Length-1 (4 bits) + data
	// TWCC uses 2 bytes
	extData := []byte{
		(extID << 4) | 0x01, // ID + Length-1 (2 bytes - 1 = 1)
		byte(twccSN >> 8),
		byte(twccSN),
		0x00, // padding
	}

	// VP8ペイロード
	vp8Desc := []byte{0x10}                           // S=1
	vp8Header := []byte{0x9d, 0x01, 0x2a, 0x00, 0x00} // keyframe
	payload := []byte{0x01}

	pkt := make([]byte, 0, len(rtpHeader)+len(extHeader)+len(extData)+len(vp8Desc)+len(vp8Header)+len(payload))
	pkt = append(pkt, rtpHeader...)
	pkt = append(pkt, extHeader...)
	pkt = append(pkt, extData...)
	pkt = append(pkt, vp8Desc...)
	pkt = append(pkt, vp8Header...)
	pkt = append(pkt, payload...)

	return pkt
}

// Helper: AudioLevel拡張付きRTPパケット
func createRTPPacketWithAudioLevel(seqNo uint16, extID uint8, level uint8) []byte {
	// RTPヘッダー（12バイト）+ Extension
	rtpHeader := make([]byte, 12)
	rtpHeader[0] = 0x90 // V=2, X=1
	rtpHeader[1] = 111  // PT=111 (Opus)
	rtpHeader[2] = byte(seqNo >> 8)
	rtpHeader[3] = byte(seqNo)

	// Extension Header
	extHeader := []byte{
		0xBE, 0xDE,
		0x00, 0x01, // Length = 1 (4 bytes)
	}

	// AudioLevel extension: 1 byte (V=0, Level=level)
	extData := []byte{
		(extID << 4) | 0x00, // ID + Length-1 (1 byte - 1 = 0)
		level & 0x7F,        // V=0, Level
		0x00, 0x00,          // padding
	}

	// Audio payload
	payload := []byte{0x01, 0x02, 0x03}

	pkt := make([]byte, 0, len(rtpHeader)+len(extHeader)+len(extData)+len(payload))
	pkt = append(pkt, rtpHeader...)
	pkt = append(pkt, extHeader...)
	pkt = append(pkt, extData...)
	pkt = append(pkt, payload...)

	return pkt
}
