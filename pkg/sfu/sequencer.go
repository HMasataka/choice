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

	if !n.init {
		n.headSN = offSn
		n.init = true
	}

	step := 0
	if head {
		inc := offSn - n.headSN
		for i := uint16(1); i < inc; i++ {
			n.step++
			if n.step >= n.max {
				n.step = 0
			}
		}
		step = n.step
		n.headSN = offSn
	} else {
		step = n.step - int(n.headSN-offSn)
		if step < 0 {
			if step*-1 >= n.max {
				return nil
			}
			step = n.max + step
		}
	}

	n.seq[n.step] = packetMeta{
		sourceSeqNo: sn,
		targetSeqNo: offSn,
		timestamp:   timeStamp,
		layer:       layer,
	}

	pm := &n.seq[n.step]
	n.step++
	if n.step >= n.max {
		n.step = 0
	}

	return pm
}

func (n *sequencer) getSeqNoPairs(seqNo []uint16) []packetMeta {
	n.mu.Lock()
	defer n.mu.Unlock()

	meta := make([]packetMeta, 0, 17)
	refTime := uint32(time.Now().UnixNano()/1e6 - n.startTime)
	for _, sn := range seqNo {
		step := n.step - int(n.headSN-sn) - 1
		if step < 0 {
			if step*-1 >= n.max {
				continue
			}
			step = n.max + step
		}
		seq := &n.seq[step]
		if seq.targetSeqNo == sn {
			if seq.lastNack == 0 || refTime-seq.lastNack > ignoreRetransmission {
				seq.lastNack = refTime
				meta = append(meta, *seq)
			}
		}
	}

	return meta
}
