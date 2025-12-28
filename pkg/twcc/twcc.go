package twcc

import (
	"encoding/binary"
	"math"
	"math/rand"
	"sort"
	"sync"

	"github.com/gammazero/deque"
	"github.com/pion/rtcp"
)

// TWCCフィードバックパケットのヘッダフィールドオフセット
const (
	baseSequenceNumberOffset = 8
	packetStatusCountOffset  = 10
	referenceTimeOffset      = 12

	tccReportDelta          = 1e8  // 通常のフィードバック送信間隔（100ms）
	tccReportDeltaAfterMark = 50e6 // RTPマーカビット後の送信間隔（50ms）
)

// rtpExtInfo は拡張シーケンス番号と受信時刻のペアを保持する
type rtpExtInfo struct {
	ExtTSN    uint32 // 拡張シーケンス番号（サイクルカウント + 16ビットSN）
	Timestamp int64  // 受信時刻（マイクロ秒）
}

// Responder はTWCC(Transport Wide Congestion Control)フィードバックを生成する
// 仕様: https://tools.ietf.org/html/draft-holmer-rmcat-transport-wide-cc-extensions-01
type Responder struct {
	mu sync.Mutex

	extInfo     []rtpExtInfo
	lastReport  int64
	cycles      uint32
	lastExtSN   uint32
	pktCtn      uint8
	lastSn      uint16
	lastExtInfo uint16
	mSSRC       uint32 // フィードバック対象のメディアソースSSRC
	sSSRC       uint32 // 送信側SSRC

	len      uint16
	deltaLen uint16
	payload  [100]byte
	deltas   [200]byte
	chunk    uint16

	onFeedback func(packet rtcp.RawPacket)
}

func NewTransportWideCCResponder(ssrc uint32) *Responder {
	return &Responder{
		extInfo: make([]rtpExtInfo, 0, 101),
		sSSRC:   rand.Uint32(),
		mSSRC:   ssrc,
	}
}

// Push はRTPパケットの受信情報を登録し、条件を満たせばフィードバックを送信する
func (t *Responder) Push(sn uint16, timeNS int64, marker bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.updateCycles(sn)
	t.addPacketInfo(sn, timeNS)

	if t.lastReport == 0 {
		t.lastReport = timeNS
	}
	t.lastSn = sn

	if t.shouldSendFeedback(timeNS, marker) {
		t.sendFeedback(timeNS)
	}
}

// updateCycles は16ビットシーケンス番号のオーバーフローを検出しサイクルカウントを更新する
func (t *Responder) updateCycles(sn uint16) {
	if sn < 0x0fff && t.lastSn > 0xf000 {
		t.cycles += 1 << 16
	}
}

// addPacketInfo は拡張シーケンス番号と受信時刻をバッファに追加する
func (t *Responder) addPacketInfo(sn uint16, timeNS int64) {
	t.extInfo = append(t.extInfo, rtpExtInfo{
		ExtTSN:    t.cycles | uint32(sn),
		Timestamp: timeNS / 1e3,
	})
}

// shouldSendFeedback はフィードバック送信条件を判定する
// 条件: 20パケット以上 かつ (100ms経過 or 100パケット超 or マーカービット+50ms経過)
func (t *Responder) shouldSendFeedback(timeNS int64, marker bool) bool {
	if len(t.extInfo) <= 20 || t.mSSRC == 0 {
		return false
	}

	delta := timeNS - t.lastReport
	return delta >= tccReportDelta || len(t.extInfo) > 100 || (marker && delta >= tccReportDeltaAfterMark)
}

// sendFeedback はTWCCフィードバックパケットを生成しコールバックで送信する
func (t *Responder) sendFeedback(timeNS int64) {
	if pkt := t.buildTransportCCPacket(); pkt != nil {
		t.onFeedback(pkt)
	}
	t.lastReport = timeNS
}

func (t *Responder) OnFeedback(f func(p rtcp.RawPacket)) {
	t.onFeedback = f
}

// buildTransportCCPacket は蓄積されたパケット情報からTWCCフィードバックを構築する
func (t *Responder) buildTransportCCPacket() rtcp.RawPacket {
	if len(t.extInfo) == 0 {
		return nil
	}

	sort.Slice(t.extInfo, func(i, j int) bool {
		return t.extInfo[i].ExtTSN < t.extInfo[j].ExtTSN
	})

	// ロストパケット（Timestamp=0）を挿入しながらパケットリストを構築
	tccPackets := make([]rtpExtInfo, 0, int(float64(len(t.extInfo))*1.2))
	for _, tccExtInfo := range t.extInfo {
		if tccExtInfo.ExtTSN < t.lastExtSN {
			continue
		}

		// ギャップ検出: ロストパケットのエントリを挿入
		if t.lastExtSN != 0 {
			for j := t.lastExtSN + 1; j < tccExtInfo.ExtTSN; j++ {
				tccPackets = append(tccPackets, rtpExtInfo{ExtTSN: j})
			}
		}

		t.lastExtSN = tccExtInfo.ExtTSN
		tccPackets = append(tccPackets, tccExtInfo)
	}

	t.extInfo = t.extInfo[:0]

	// パケットステータスの圧縮エンコーディング
	firstRecv := false
	same := true
	timestamp := int64(0)
	lastStatus := rtcp.TypeTCCPacketReceivedWithoutDelta
	maxStatus := rtcp.TypeTCCPacketNotReceived

	var statusList deque.Deque[uint16]
	statusList.SetBaseCap(3)

	for _, stat := range tccPackets {
		status := rtcp.TypeTCCPacketNotReceived

		if stat.Timestamp != 0 {
			var delta int64

			if !firstRecv {
				firstRecv = true
				refTime := stat.Timestamp / 64e3
				timestamp = refTime * 64e3
				t.writeHeader(
					uint16(tccPackets[0].ExtTSN),
					uint16(len(tccPackets)),
					uint32(refTime),
				)
				t.pktCtn++
			}

			// デルタ計算（250マイクロ秒単位）
			delta = (stat.Timestamp - timestamp) / 250

			if delta < 0 || delta > 255 {
				status = rtcp.TypeTCCPacketReceivedLargeDelta
				rDelta := int16(delta)
				if int64(rDelta) != delta {
					if rDelta > 0 {
						rDelta = math.MaxInt16
					} else {
						rDelta = math.MinInt16
					}
				}
				t.writeDelta(status, uint16(rDelta))
			} else {
				status = rtcp.TypeTCCPacketReceivedSmallDelta
				t.writeDelta(status, uint16(delta))
			}

			timestamp = stat.Timestamp
		}

		// RLEとStatus Symbol Chunkの使い分け
		if same && status != lastStatus && lastStatus != rtcp.TypeTCCPacketReceivedWithoutDelta {
			if statusList.Len() > 7 {
				t.writeRunLengthChunk(lastStatus, uint16(statusList.Len()))
				statusList.Clear()
				lastStatus = rtcp.TypeTCCPacketReceivedWithoutDelta
				maxStatus = rtcp.TypeTCCPacketNotReceived
				same = true
			} else {
				same = false
			}
		}

		statusList.PushBack(status)
		if status > maxStatus {
			maxStatus = status
		}
		lastStatus = status

		// 2ビットシンボルで7個溜まった場合
		if !same && maxStatus == rtcp.TypeTCCPacketReceivedLargeDelta && statusList.Len() > 6 {
			for i := range 7 {
				t.createStatusSymbolChunk(rtcp.TypeTCCSymbolSizeTwoBit, statusList.PopFront(), i)
			}
			t.writeStatusSymbolChunk(rtcp.TypeTCCSymbolSizeTwoBit)
			lastStatus = rtcp.TypeTCCPacketReceivedWithoutDelta
			maxStatus = rtcp.TypeTCCPacketNotReceived
			same = true

			for i := 0; i < statusList.Len(); i++ {
				status = statusList.At(i)
				if status > maxStatus {
					maxStatus = status
				}
				if same && lastStatus != rtcp.TypeTCCPacketReceivedWithoutDelta && status != lastStatus {
					same = false
				}
				lastStatus = status
			}
		} else if !same && statusList.Len() > 13 {
			// 1ビットシンボルで14個溜まった場合
			for i := range 14 {
				t.createStatusSymbolChunk(rtcp.TypeTCCSymbolSizeOneBit, statusList.PopFront(), i)
			}
			t.writeStatusSymbolChunk(rtcp.TypeTCCSymbolSizeOneBit)
			lastStatus = rtcp.TypeTCCPacketReceivedWithoutDelta
			maxStatus = rtcp.TypeTCCPacketNotReceived
			same = true
		}
	}

	// 残りのステータスを処理
	if statusList.Len() > 0 {
		if same {
			t.writeRunLengthChunk(lastStatus, uint16(statusList.Len()))
		} else if maxStatus == rtcp.TypeTCCPacketReceivedLargeDelta {
			for i := 0; i < statusList.Len(); i++ {
				t.createStatusSymbolChunk(rtcp.TypeTCCSymbolSizeTwoBit, statusList.PopFront(), i)
			}
			t.writeStatusSymbolChunk(rtcp.TypeTCCSymbolSizeTwoBit)
		} else {
			for i := 0; i < statusList.Len(); i++ {
				t.createStatusSymbolChunk(rtcp.TypeTCCSymbolSizeOneBit, statusList.PopFront(), i)
			}
			t.writeStatusSymbolChunk(rtcp.TypeTCCSymbolSizeOneBit)
		}
	}

	// RTCPパケット構築（4バイト境界にアライメント）
	pLen := t.len + t.deltaLen + 4
	pad := pLen%4 != 0
	var padSize uint8
	for pLen%4 != 0 {
		padSize++
		pLen++
	}

	hdr := rtcp.Header{
		Padding: pad,
		Length:  (pLen / 4) - 1,
		Count:   rtcp.FormatTCC,
		Type:    rtcp.TypeTransportSpecificFeedback,
	}
	hb, _ := hdr.Marshal()

	pkt := make(rtcp.RawPacket, pLen)
	copy(pkt, hb)
	copy(pkt[4:], t.payload[:t.len])
	copy(pkt[4+t.len:], t.deltas[:t.deltaLen])
	if pad {
		pkt[len(pkt)-1] = padSize
	}

	t.deltaLen = 0

	return pkt
}

// writeHeader はTWCCフィードバックのヘッダ部（16バイト）を構築する
//
//	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//	|                     SSRC of packet sender                     |
//	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//	|                     SSRC of media source                      |
//	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//	|     base sequence number      |      packet status count      |
//	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//	|         reference time (24bits)           | fb pkt count (8)  |
//	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
func (t *Responder) writeHeader(bSN, packetCount uint16, refTime uint32) {
	binary.BigEndian.PutUint32(t.payload[0:], t.sSSRC)
	binary.BigEndian.PutUint32(t.payload[4:], t.mSSRC)
	binary.BigEndian.PutUint16(t.payload[baseSequenceNumberOffset:], bSN)
	binary.BigEndian.PutUint16(t.payload[packetStatusCountOffset:], packetCount)
	binary.BigEndian.PutUint32(t.payload[referenceTimeOffset:], refTime<<8|uint32(t.pktCtn))
	t.len = 16
}

// writeRunLengthChunk はRLE形式のチャンクを書き込む
// フォーマット: |0|S(2bits)|Run Length(13bits)|
func (t *Responder) writeRunLengthChunk(symbol uint16, runLength uint16) {
	binary.BigEndian.PutUint16(t.payload[t.len:], symbol<<13|runLength)
	t.len += 2
}

// createStatusSymbolChunk はStatus Symbol Chunkにシンボルを追加する
func (t *Responder) createStatusSymbolChunk(symbolSize, symbol uint16, i int) {
	numOfBits := symbolSize + 1
	t.chunk = setNBitsOfUint16(t.chunk, numOfBits, numOfBits*uint16(i)+2, symbol)
}

// writeStatusSymbolChunk は構築済みのStatus Symbol Chunkをペイロードに書き込む
// フォーマット: |1|S(1bit)|symbol list...|
func (t *Responder) writeStatusSymbolChunk(symbolSize uint16) {
	t.chunk = setNBitsOfUint16(t.chunk, 1, 0, 1)
	t.chunk = setNBitsOfUint16(t.chunk, 1, 1, symbolSize)
	binary.BigEndian.PutUint16(t.payload[t.len:], t.chunk)
	t.chunk = 0
	t.len += 2
}

// writeDelta はタイムスタンプデルタをバッファに書き込む
// 小デルタ: 1バイト(0-255)、大デルタ: 2バイト符号付き
func (t *Responder) writeDelta(deltaType, delta uint16) {
	if deltaType == rtcp.TypeTCCPacketReceivedSmallDelta {
		t.deltas[t.deltaLen] = byte(delta)
		t.deltaLen++
		return
	}
	binary.BigEndian.PutUint16(t.deltas[t.deltaLen:], delta)
	t.deltaLen += 2
}

// setNBitsOfUint16 は指定ビット位置にビット列を設定する
func setNBitsOfUint16(src, size, startIndex, val uint16) uint16 {
	if startIndex+size > 16 {
		return 0
	}
	val &= (1 << size) - 1
	return src | (val << (16 - size - startIndex))
}
