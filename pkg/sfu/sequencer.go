package sfu

import (
	"sync"
	"time"
)

const (
	// 最後のNACKから ignoreRetransmission ミリ秒以内の再送要求を無視する
	ignoreRetransmission = 100
)

type packetMeta struct {
	// ストリームの元のシーケンス番号。
	// パブリッシャからの元パケットを見つけるために使用する。
	sourceSeqNo uint16
	// オフセット適用後のシーケンス番号。
	// 関連するDownTrackで使用される番号で、オフセットに応じて変更され、共有してはならない。
	targetSeqNo uint16
	// 関連するDownTrack用に変換されたタイムスタンプ。
	timestamp uint32
	// このパケットが最後にNACK要求された時刻。
	// クライアントが同じパケットを複数回要求することがあるため、要求済みの
	// パケットを記録して同一パケットを重複送信しないようにする。
	// 単位はシーケンサ開始時刻からの1ms刻み。
	lastNack uint32
	// パケットの空間レイヤ
	layer uint8
	// コーデックに依存して異なる付帯情報
	misc uint32
}

func (p *packetMeta) setVP8PayloadMeta(tlz0Idx uint8, picID uint16) {
	p.misc = uint32(tlz0Idx)<<16 | uint32(picID)
}

func (p *packetMeta) getVP8PayloadMeta() (uint8, uint16) {
	return uint8(p.misc >> 16), uint16(p.misc)
}

// Sequencerは、DownTrackが受信したパケットのシーケンスを保持する
type sequencer struct {
	mu        sync.Mutex
	init      bool
	max       int
	seq       []packetMeta
	step      int
	headSN    uint16
	startTime int64
}

func newSequencer(maxTrack int) *sequencer {
	return &sequencer{
		startTime: time.Now().UnixNano() / 1e6,
		max:       maxTrack,
		seq:       make([]packetMeta, maxTrack),
	}
}

func (n *sequencer) push(sn, offSn uint16, timeStamp uint32, layer uint8, head bool) *packetMeta {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.initializeIfNeeded(offSn)

	idx, ok := n.calculateIndex(offSn, head)
	if !ok {
		return nil
	}

	n.seq[idx] = packetMeta{
		sourceSeqNo: sn,
		targetSeqNo: offSn,
		timestamp:   timeStamp,
		layer:       layer,
	}

	pm := &n.seq[idx]
	n.advanceStep()

	return pm
}

// initializeIfNeeded は初回呼び出し時にheadSNを初期化する
func (n *sequencer) initializeIfNeeded(offSn uint16) {
	if !n.init {
		n.headSN = offSn
		n.init = true
	}
}

// calculateIndex は格納先のインデックスを計算する。
// headがtrueの場合は新しいパケット、falseの場合は遅延パケットとして処理する。
// 遅延パケットがバッファ範囲外の場合は(0, false)を返す。
func (n *sequencer) calculateIndex(offSn uint16, head bool) (int, bool) {
	if head {
		return n.calculateHeadIndex(offSn), true
	}
	return n.calculateLateIndex(offSn)
}

// calculateHeadIndex は新しいパケット（head）のインデックスを計算し、
// 欠落したシーケンス番号分だけステップを進める
func (n *sequencer) calculateHeadIndex(offSn uint16) int {
	inc := offSn - n.headSN
	for i := uint16(1); i < inc; i++ {
		n.advanceStep()
	}
	n.headSN = offSn
	return n.step
}

// calculateLateIndex は遅延パケットのインデックスを計算する。
// バッファ範囲外の場合は(0, false)を返す。
func (n *sequencer) calculateLateIndex(offSn uint16) (int, bool) {
	offset := int(n.headSN - offSn)
	idx := n.step - offset

	if idx < 0 {
		if -idx >= n.max {
			return 0, false
		}
		idx += n.max
	}

	return idx, true
}

// advanceStep はステップを1つ進め、必要に応じてラップアラウンドする
func (n *sequencer) advanceStep() {
	n.step++
	if n.step >= n.max {
		n.step = 0
	}
}

// maxNackBatch はNACK要求で一度に処理するパケットの最大数。
// RFC 4585で定義されるRTCP FBメッセージのサイズ制限に基づく。
const maxNackBatch = 17

func (n *sequencer) getSeqNoPairs(seqNo []uint16) []packetMeta {
	n.mu.Lock()
	defer n.mu.Unlock()

	meta := make([]packetMeta, 0, maxNackBatch)
	refTime := n.currentRefTime()

	for _, sn := range seqNo {
		if pm := n.findAndUpdatePacketMeta(sn, refTime); pm != nil {
			meta = append(meta, *pm)
		}
	}

	return meta
}

// currentRefTime はシーケンサ開始時刻からの経過時間（ミリ秒）を返す
func (n *sequencer) currentRefTime() uint32 {
	return uint32(time.Now().UnixNano()/1e6 - n.startTime)
}

// findAndUpdatePacketMeta は指定されたシーケンス番号のパケットメタデータを検索し、
// 再送が必要な場合はlastNackを更新して返す。
// 見つからない場合、または再送抑制期間内の場合はnilを返す。
func (n *sequencer) findAndUpdatePacketMeta(sn uint16, refTime uint32) *packetMeta {
	idx, ok := n.calculateIndexForLookup(sn)
	if !ok {
		return nil
	}

	seq := &n.seq[idx]
	if seq.targetSeqNo != sn {
		return nil
	}

	if !n.shouldRetransmit(seq, refTime) {
		return nil
	}

	seq.lastNack = refTime
	return seq
}

// calculateIndexForLookup は検索用のインデックスを計算する。
// pushとは異なり、現在のstepの1つ前から検索を開始する。
// バッファ範囲外の場合は(0, false)を返す。
func (n *sequencer) calculateIndexForLookup(sn uint16) (int, bool) {
	offset := int(n.headSN-sn) + 1
	idx := n.step - offset

	if idx < 0 {
		if -idx >= n.max {
			return 0, false
		}
		idx += n.max
	}

	return idx, true
}

// shouldRetransmit は再送すべきかどうかを判定する。
// 一度もNACKされていない、または抑制期間を過ぎている場合はtrueを返す。
func (n *sequencer) shouldRetransmit(seq *packetMeta, refTime uint32) bool {
	return seq.lastNack == 0 || refTime-seq.lastNack > ignoreRetransmission
}
