package twcc

import (
	"testing"

	"github.com/pion/rtcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTransportWideCCResponder(t *testing.T) {
	r := NewTransportWideCCResponder(12345)

	assert.NotNil(t, r)
	assert.Equal(t, uint32(12345), r.mSSRC)
	assert.NotEqual(t, uint32(0), r.sSSRC) // ランダム生成されるので0ではない
	assert.NotNil(t, r.extInfo)
	assert.Equal(t, 0, len(r.extInfo))
}

func TestResponder_OnFeedback(t *testing.T) {
	r := NewTransportWideCCResponder(12345)

	r.OnFeedback(func(p rtcp.RawPacket) {})

	assert.NotNil(t, r.onFeedback)
}

func TestResponder_Push_AddsPacketInfo(t *testing.T) {
	r := NewTransportWideCCResponder(12345)
	r.OnFeedback(func(p rtcp.RawPacket) {})

	r.Push(100, 1000000, false)

	assert.Equal(t, 1, len(r.extInfo))
	assert.Equal(t, uint32(100), r.extInfo[0].ExtTSN)
	assert.Equal(t, int64(1000), r.extInfo[0].Timestamp) // nanosec to microsec
	assert.Equal(t, uint16(100), r.lastSn)
}

func TestResponder_Push_MultiplePackets(t *testing.T) {
	r := NewTransportWideCCResponder(12345)
	r.OnFeedback(func(p rtcp.RawPacket) {})

	for i := range 10 {
		r.Push(uint16(i), int64(i)*1000000, false)
	}

	assert.Equal(t, 10, len(r.extInfo))
	assert.Equal(t, uint16(9), r.lastSn)
}

func TestResponder_updateCycles(t *testing.T) {
	tests := []struct {
		name           string
		lastSn         uint16
		newSn          uint16
		initialCycles  uint32
		expectedCycles uint32
	}{
		{
			name:           "通常のインクリメント",
			lastSn:         100,
			newSn:          101,
			initialCycles:  0,
			expectedCycles: 0,
		},
		{
			name:           "ラップアラウンド検出",
			lastSn:         0xFFFF,
			newSn:          0x0001,
			initialCycles:  0,
			expectedCycles: 1 << 16,
		},
		{
			name:           "境界付近でラップアラウンドなし",
			lastSn:         0xF001,
			newSn:          0x0FFF,
			initialCycles:  0,
			expectedCycles: 0, // 0x0FFF > 0x0fff なのでラップアラウンドしない
		},
		{
			name:           "ラップアラウンドなし（境界内）",
			lastSn:         0x1000,
			newSn:          0x2000,
			initialCycles:  0,
			expectedCycles: 0,
		},
		{
			name:           "2回目のラップアラウンド",
			lastSn:         0xF500,
			newSn:          0x0100,
			initialCycles:  1 << 16,
			expectedCycles: 2 << 16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewTransportWideCCResponder(12345)
			r.lastSn = tt.lastSn
			r.cycles = tt.initialCycles

			r.updateCycles(tt.newSn)

			assert.Equal(t, tt.expectedCycles, r.cycles)
		})
	}
}

func TestResponder_addPacketInfo(t *testing.T) {
	r := NewTransportWideCCResponder(12345)
	r.cycles = 1 << 16 // サイクル1

	r.addPacketInfo(100, 5000000000) // 5秒（ナノ秒）

	require.Equal(t, 1, len(r.extInfo))
	// 拡張SN = cycles | sn = 65536 | 100 = 65636
	assert.Equal(t, uint32(65636), r.extInfo[0].ExtTSN)
	// タイムスタンプ = 5000000000 / 1000 = 5000000 マイクロ秒
	assert.Equal(t, int64(5000000), r.extInfo[0].Timestamp)
}

func TestResponder_shouldSendFeedback(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*Responder)
		timeNS   int64
		marker   bool
		expected bool
		desc     string
	}{
		{
			name: "パケット数が20以下",
			setup: func(r *Responder) {
				for i := range 20 {
					r.extInfo = append(r.extInfo, rtpExtInfo{ExtTSN: uint32(i)})
				}
				r.lastReport = 0
			},
			timeNS:   200e6,
			marker:   false,
			expected: false,
			desc:     "20パケット以下ではフィードバックしない",
		},
		{
			name: "パケット数が21でmSSRCが0",
			setup: func(r *Responder) {
				r.mSSRC = 0
				for i := range 21 {
					r.extInfo = append(r.extInfo, rtpExtInfo{ExtTSN: uint32(i)})
				}
			},
			timeNS:   200e6,
			marker:   false,
			expected: false,
			desc:     "mSSRCが0ではフィードバックしない",
		},
		{
			name: "100ms経過で送信",
			setup: func(r *Responder) {
				for i := range 21 {
					r.extInfo = append(r.extInfo, rtpExtInfo{ExtTSN: uint32(i)})
				}
				r.lastReport = 0
			},
			timeNS:   100e6, // 100ms
			marker:   false,
			expected: true,
			desc:     "100ms経過でフィードバック送信",
		},
		{
			name: "100パケット超で送信",
			setup: func(r *Responder) {
				for i := range 101 {
					r.extInfo = append(r.extInfo, rtpExtInfo{ExtTSN: uint32(i)})
				}
				r.lastReport = 50e6
			},
			timeNS:   60e6, // 10ms経過（100msには達していない）
			marker:   false,
			expected: true,
			desc:     "100パケット超でフィードバック送信",
		},
		{
			name: "マーカービット+50ms経過で送信",
			setup: func(r *Responder) {
				for i := range 21 {
					r.extInfo = append(r.extInfo, rtpExtInfo{ExtTSN: uint32(i)})
				}
				r.lastReport = 0
			},
			timeNS:   50e6, // 50ms
			marker:   true,
			expected: true,
			desc:     "マーカービット+50ms経過でフィードバック送信",
		},
		{
			name: "マーカービットあるが50ms未満",
			setup: func(r *Responder) {
				for i := range 21 {
					r.extInfo = append(r.extInfo, rtpExtInfo{ExtTSN: uint32(i)})
				}
				r.lastReport = 0
			},
			timeNS:   40e6, // 40ms
			marker:   true,
			expected: false,
			desc:     "マーカービットがあっても50ms未満では送信しない",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewTransportWideCCResponder(12345)
			tt.setup(r)

			result := r.shouldSendFeedback(tt.timeNS, tt.marker)

			assert.Equal(t, tt.expected, result, tt.desc)
		})
	}
}

func TestResponder_buildPacketList_SortsPackets(t *testing.T) {
	r := NewTransportWideCCResponder(12345)
	r.OnFeedback(func(p rtcp.RawPacket) {})

	// 連続したパケットを順不同で追加（ギャップなし）
	r.extInfo = []rtpExtInfo{
		{ExtTSN: 3, Timestamp: 3000},
		{ExtTSN: 1, Timestamp: 1000},
		{ExtTSN: 4, Timestamp: 4000},
		{ExtTSN: 2, Timestamp: 2000},
	}

	packets := r.buildPacketList()

	// ソートされ、連続しているのでロストパケットは挿入されない
	require.Equal(t, 4, len(packets))
	assert.Equal(t, uint32(1), packets[0].ExtTSN)
	assert.Equal(t, int64(1000), packets[0].Timestamp)
	assert.Equal(t, uint32(2), packets[1].ExtTSN)
	assert.Equal(t, int64(2000), packets[1].Timestamp)
	assert.Equal(t, uint32(3), packets[2].ExtTSN)
	assert.Equal(t, int64(3000), packets[2].Timestamp)
	assert.Equal(t, uint32(4), packets[3].ExtTSN)
	assert.Equal(t, int64(4000), packets[3].Timestamp)
}

func TestResponder_buildPacketList_SortsAndInsertsLostPackets(t *testing.T) {
	r := NewTransportWideCCResponder(12345)
	r.OnFeedback(func(p rtcp.RawPacket) {})

	// ギャップのあるパケットを順不同で追加
	r.extInfo = []rtpExtInfo{
		{ExtTSN: 5, Timestamp: 5000},
		{ExtTSN: 2, Timestamp: 2000},
		{ExtTSN: 8, Timestamp: 8000},
		{ExtTSN: 1, Timestamp: 1000},
	}

	packets := r.buildPacketList()

	// ソートされ、ギャップ（3,4,6,7）がロストとして挿入される
	// 1, 2, 3(lost), 4(lost), 5, 6(lost), 7(lost), 8
	require.Equal(t, 8, len(packets))
	assert.Equal(t, uint32(1), packets[0].ExtTSN)
	assert.Equal(t, int64(1000), packets[0].Timestamp)
	assert.Equal(t, uint32(2), packets[1].ExtTSN)
	assert.Equal(t, int64(2000), packets[1].Timestamp)
	assert.Equal(t, uint32(3), packets[2].ExtTSN)
	assert.Equal(t, int64(0), packets[2].Timestamp) // lost
	assert.Equal(t, uint32(4), packets[3].ExtTSN)
	assert.Equal(t, int64(0), packets[3].Timestamp) // lost
	assert.Equal(t, uint32(5), packets[4].ExtTSN)
	assert.Equal(t, int64(5000), packets[4].Timestamp)
	assert.Equal(t, uint32(6), packets[5].ExtTSN)
	assert.Equal(t, int64(0), packets[5].Timestamp) // lost
	assert.Equal(t, uint32(7), packets[6].ExtTSN)
	assert.Equal(t, int64(0), packets[6].Timestamp) // lost
	assert.Equal(t, uint32(8), packets[7].ExtTSN)
	assert.Equal(t, int64(8000), packets[7].Timestamp)
}

func TestResponder_buildPacketList_InsertsLostPackets(t *testing.T) {
	r := NewTransportWideCCResponder(12345)
	r.OnFeedback(func(p rtcp.RawPacket) {})

	// ギャップのあるパケット（1, 2, 5）
	r.extInfo = []rtpExtInfo{
		{ExtTSN: 1, Timestamp: 1000},
		{ExtTSN: 2, Timestamp: 2000},
		{ExtTSN: 5, Timestamp: 5000},
	}

	packets := r.buildPacketList()

	// 3, 4 がロストパケットとして挿入されている
	require.Equal(t, 5, len(packets))
	assert.Equal(t, uint32(1), packets[0].ExtTSN)
	assert.Equal(t, uint32(2), packets[1].ExtTSN)
	assert.Equal(t, uint32(3), packets[2].ExtTSN)
	assert.Equal(t, int64(0), packets[2].Timestamp) // ロストパケットはTimestamp=0
	assert.Equal(t, uint32(4), packets[3].ExtTSN)
	assert.Equal(t, int64(0), packets[3].Timestamp)
	assert.Equal(t, uint32(5), packets[4].ExtTSN)
}

func TestResponder_buildPacketList_SkipsOldPackets(t *testing.T) {
	r := NewTransportWideCCResponder(12345)
	r.OnFeedback(func(p rtcp.RawPacket) {})
	r.lastExtSN = 5 // 5以下のパケットはスキップ（すでに処理済み）

	r.extInfo = []rtpExtInfo{
		{ExtTSN: 3, Timestamp: 3000},
		{ExtTSN: 5, Timestamp: 5000},
		{ExtTSN: 7, Timestamp: 7000},
		{ExtTSN: 10, Timestamp: 10000},
	}

	packets := r.buildPacketList()

	// 3と5（lastExtSN以下）はスキップ、6がロスト、7受信、8,9ロスト、10受信
	require.Equal(t, 5, len(packets))
	assert.Equal(t, uint32(6), packets[0].ExtTSN)
	assert.Equal(t, int64(0), packets[0].Timestamp) // lost
	assert.Equal(t, uint32(7), packets[1].ExtTSN)
	assert.Equal(t, int64(7000), packets[1].Timestamp)
	assert.Equal(t, uint32(8), packets[2].ExtTSN)
	assert.Equal(t, int64(0), packets[2].Timestamp) // lost
	assert.Equal(t, uint32(9), packets[3].ExtTSN)
	assert.Equal(t, int64(0), packets[3].Timestamp) // lost
	assert.Equal(t, uint32(10), packets[4].ExtTSN)
	assert.Equal(t, int64(10000), packets[4].Timestamp)
}

func TestSetNBitsOfUint16(t *testing.T) {
	tests := []struct {
		name       string
		src        uint16
		size       uint16
		startIndex uint16
		val        uint16
		expected   uint16
	}{
		{
			name:       "先頭1ビットに1を設定",
			src:        0,
			size:       1,
			startIndex: 0,
			val:        1,
			expected:   0x8000, // 1000 0000 0000 0000
		},
		{
			name:       "2ビット目から2ビットに3を設定",
			src:        0,
			size:       2,
			startIndex: 2,
			val:        3,
			expected:   0x3000, // 0011 0000 0000 0000
		},
		{
			name:       "既存値に追加",
			src:        0x8000,
			size:       2,
			startIndex: 2,
			val:        2,
			expected:   0xA000, // 1010 0000 0000 0000
		},
		{
			name:       "境界外（16ビット超え）",
			src:        0xFFFF,
			size:       2,
			startIndex: 15,
			val:        3,
			expected:   0, // 境界外は0を返す
		},
		{
			name:       "値のマスク",
			src:        0,
			size:       2,
			startIndex: 0,
			val:        0xFF,   // 2ビットを超える値
			expected:   0xC000, // 0xFF & 0x3 = 3 → 1100 0000 0000 0000
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := setNBitsOfUint16(tt.src, tt.size, tt.startIndex, tt.val)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClampInt16(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected int16
	}{
		{
			name:     "正常範囲内（正）",
			input:    100,
			expected: 100,
		},
		{
			name:     "正常範囲内（負）",
			input:    -100,
			expected: -100,
		},
		{
			name:     "最大値",
			input:    32767,
			expected: 32767,
		},
		{
			name:     "最小値",
			input:    -32768,
			expected: -32768,
		},
		{
			name:     "最大値超過",
			input:    100000,
			expected: 32767,
		},
		{
			name:     "最小値未満",
			input:    -100000,
			expected: -32768,
		},
		{
			name:     "0",
			input:    0,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := clampInt16(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResponder_buildTransportCCPacket_EmptyInfo(t *testing.T) {
	r := NewTransportWideCCResponder(12345)
	r.OnFeedback(func(p rtcp.RawPacket) {})

	packet := r.buildTransportCCPacket()

	assert.Nil(t, packet)
}

func TestResponder_buildTransportCCPacket_SinglePacket(t *testing.T) {
	r := NewTransportWideCCResponder(12345)
	r.OnFeedback(func(p rtcp.RawPacket) {})

	r.extInfo = []rtpExtInfo{
		{ExtTSN: 1, Timestamp: 64000}, // 64ms
	}

	packet := r.buildTransportCCPacket()

	require.NotNil(t, packet)
	assert.True(t, len(packet) >= 20) // ヘッダ(4) + ペイロード(16) + α
}

func TestResponder_buildTransportCCPacket_MultiplePackets(t *testing.T) {
	r := NewTransportWideCCResponder(12345)
	r.OnFeedback(func(p rtcp.RawPacket) {})

	// 連続したパケット
	for i := range 10 {
		r.extInfo = append(r.extInfo, rtpExtInfo{
			ExtTSN:    uint32(i + 1),
			Timestamp: int64(64000 + i*1000),
		})
	}

	packet := r.buildTransportCCPacket()

	require.NotNil(t, packet)
	// extInfoがクリアされていることを確認
	assert.Equal(t, 0, len(r.extInfo))
}

func TestResponder_Push_SendsFeedback(t *testing.T) {
	r := NewTransportWideCCResponder(12345)

	feedbackCount := 0
	r.OnFeedback(func(p rtcp.RawPacket) {
		feedbackCount++
	})

	// 100ms以上経過 + 21パケット以上でフィードバック送信
	for i := range 25 {
		// 最初のパケットから100ms以上経過させる
		timeNS := int64(i) * 5e6 // 5ms間隔
		if i == 24 {
			timeNS = 150e6 // 150ms
		}
		r.Push(uint16(i), timeNS, false)
	}

	assert.GreaterOrEqual(t, feedbackCount, 1)
}

func TestResponder_calculatePadding(t *testing.T) {
	tests := []struct {
		name            string
		length          uint16
		deltaLen        uint16
		expectedPLen    uint16
		expectedPadSize uint8
	}{
		{
			name:            "パディング不要（4の倍数）",
			length:          16,
			deltaLen:        0,
			expectedPLen:    20,
			expectedPadSize: 0,
		},
		{
			name:            "1バイトパディング",
			length:          16,
			deltaLen:        3,
			expectedPLen:    24,
			expectedPadSize: 1,
		},
		{
			name:            "2バイトパディング",
			length:          16,
			deltaLen:        2,
			expectedPLen:    24,
			expectedPadSize: 2,
		},
		{
			name:            "3バイトパディング",
			length:          16,
			deltaLen:        1,
			expectedPLen:    24,
			expectedPadSize: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewTransportWideCCResponder(12345)
			r.length = tt.length
			r.deltaLen = tt.deltaLen

			pLen, padSize := r.calculatePadding()

			assert.Equal(t, tt.expectedPLen, pLen)
			assert.Equal(t, tt.expectedPadSize, padSize)
		})
	}
}

func TestResponder_writeHeader(t *testing.T) {
	r := NewTransportWideCCResponder(12345)
	r.sSSRC = 0x11223344
	r.mSSRC = 0x55667788

	r.writeHeader(100, 50, 0x123456)

	// sSSRC
	assert.Equal(t, byte(0x11), r.payload[0])
	assert.Equal(t, byte(0x22), r.payload[1])
	assert.Equal(t, byte(0x33), r.payload[2])
	assert.Equal(t, byte(0x44), r.payload[3])

	// mSSRC
	assert.Equal(t, byte(0x55), r.payload[4])
	assert.Equal(t, byte(0x66), r.payload[5])
	assert.Equal(t, byte(0x77), r.payload[6])
	assert.Equal(t, byte(0x88), r.payload[7])

	// base sequence number
	assert.Equal(t, byte(0x00), r.payload[8])
	assert.Equal(t, byte(0x64), r.payload[9]) // 100

	// packet status count
	assert.Equal(t, byte(0x00), r.payload[10])
	assert.Equal(t, byte(0x32), r.payload[11]) // 50

	// length
	assert.Equal(t, uint16(16), r.length)
}

func TestResponder_writeRunLengthChunk(t *testing.T) {
	r := NewTransportWideCCResponder(12345)
	r.length = 16

	// symbol=1 (小デルタ), runLength=100
	r.writeRunLengthChunk(1, 100)

	// |0|S(2bits)|Run Length(13bits)|
	// symbol=1 → 01
	// runLength=100 → 0000001100100
	// 結果: 0 01 0000001100100 = 0010 0000 0110 0100 = 0x2064
	assert.Equal(t, byte(0x20), r.payload[16])
	assert.Equal(t, byte(0x64), r.payload[17])
	assert.Equal(t, uint16(18), r.length)
}

func TestResponder_writeDelta(t *testing.T) {
	tests := []struct {
		name          string
		deltaType     uint16
		delta         uint16
		expectedBytes []byte
		expectedLen   uint16
	}{
		{
			name:          "小デルタ",
			deltaType:     1, // TypeTCCPacketReceivedSmallDelta
			delta:         100,
			expectedBytes: []byte{100},
			expectedLen:   1,
		},
		{
			name:          "大デルタ",
			deltaType:     2, // TypeTCCPacketReceivedLargeDelta
			delta:         1000,
			expectedBytes: []byte{0x03, 0xE8},
			expectedLen:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewTransportWideCCResponder(12345)
			r.deltaLen = 0

			r.writeDelta(tt.deltaType, tt.delta)

			assert.Equal(t, tt.expectedLen, r.deltaLen)
			for i, b := range tt.expectedBytes {
				assert.Equal(t, b, r.deltas[i])
			}
		})
	}
}

func TestStatusEncoder_reset(t *testing.T) {
	r := NewTransportWideCCResponder(12345)
	enc := newStatusEncoder(r)

	// 状態を変更
	enc.statusList.PushBack(1)
	enc.statusList.PushBack(2)
	enc.lastStatus = 2
	enc.maxStatus = 2
	enc.same = false

	enc.reset()

	assert.Equal(t, 0, enc.statusList.Len())
	assert.Equal(t, uint16(3), enc.lastStatus) // TypeTCCPacketReceivedWithoutDelta
	assert.Equal(t, uint16(0), enc.maxStatus)  // TypeTCCPacketNotReceived
	assert.True(t, enc.same)
}

func TestNewStatusEncoder(t *testing.T) {
	r := NewTransportWideCCResponder(12345)
	enc := newStatusEncoder(r)

	assert.Equal(t, r, enc.resp)
	assert.Equal(t, uint16(3), enc.lastStatus) // TypeTCCPacketReceivedWithoutDelta
	assert.Equal(t, uint16(0), enc.maxStatus)  // TypeTCCPacketNotReceived
	assert.True(t, enc.same)
	assert.False(t, enc.firstRecv)
}
