package buffer

import (
	"sort"

	"github.com/pion/rtcp"
)

const maxNackTimes = 3
const maxNackCache = 100

type nack struct {
	sn     uint32
	nacked uint8
}

// nackQueue は損失パケットの追跡とNACK要求の生成を管理する
type nackQueue struct {
	nacks []nack
	kfSN  uint32 // 最後にキーフレーム要求したSN
}

func newNACKQueue() *nackQueue {
	return &nackQueue{
		nacks: make([]nack, 0, maxNackCache+1),
	}
}

func (n *nackQueue) find(extSN uint32) int {
	return sort.Search(len(n.nacks), func(i int) bool {
		return n.nacks[i].sn >= extSN
	})
}

func (n *nackQueue) remove(extSN uint32) {
	i := n.find(extSN)
	if i >= len(n.nacks) || n.nacks[i].sn != extSN {
		return
	}
	copy(n.nacks[i:], n.nacks[i+1:])
	n.nacks = n.nacks[:len(n.nacks)-1]
}

func (n *nackQueue) push(extSN uint32) {
	i := n.find(extSN)
	if i < len(n.nacks) && n.nacks[i].sn == extSN {
		return
	}
	n.insertAt(i, extSN)
	n.trimIfFull()
}

func (n *nackQueue) insertAt(i int, extSN uint32) {
	nck := nack{sn: extSN, nacked: 0}
	if i == len(n.nacks) {
		n.nacks = append(n.nacks, nck)
	} else {
		n.nacks = append(n.nacks[:i+1], n.nacks[i:]...)
		n.nacks[i] = nck
	}
}

func (n *nackQueue) trimIfFull() {
	if len(n.nacks) >= maxNackCache {
		copy(n.nacks, n.nacks[1:])
		n.nacks = n.nacks[:len(n.nacks)-1]
	}
}

// pairs はNACKペアを生成する。3回再送要求失敗時はキーフレーム要求フラグを返す
func (n *nackQueue) pairs(headSN uint32) ([]rtcp.NackPair, bool) {
	if len(n.nacks) == 0 {
		return nil, false
	}

	b := &nackPairBuilder{}
	writeIdx := 0
	askKF := false

	for _, nck := range n.nacks {
		switch n.classifyNack(nck, headSN) {
		case nackExpired:
			askKF = n.checkKeyframeRequest(nck) || askKF
		case nackTooRecent:
			n.nacks[writeIdx] = nck
			writeIdx++
		case nackPending:
			n.nacks[writeIdx] = nack{sn: nck.sn, nacked: nck.nacked + 1}
			writeIdx++
			b.add(nck.sn)
		}
	}

	n.nacks = n.nacks[:writeIdx]
	return b.build(), askKF
}

type nackStatus int

const (
	nackExpired   nackStatus = iota // NACK回数上限到達
	nackTooRecent                   // 最近のパケット（まだNACKしない）
	nackPending                     // NACK対象
)

func (n *nackQueue) classifyNack(nck nack, headSN uint32) nackStatus {
	if nck.nacked >= maxNackTimes {
		return nackExpired
	}
	if nck.sn >= headSN-2 {
		return nackTooRecent
	}
	return nackPending
}

func (n *nackQueue) checkKeyframeRequest(nck nack) bool {
	if nck.sn > n.kfSN {
		n.kfSN = nck.sn
		return true
	}
	return false
}

// nackPairBuilder はNACKペアのビットマップを構築する
type nackPairBuilder struct {
	pairs   []rtcp.NackPair
	current rtcp.NackPair
}

func (b *nackPairBuilder) add(sn uint32) {
	snU16 := uint16(sn)
	if b.current.PacketID == 0 || snU16 > b.current.PacketID+16 {
		b.flush()
		b.current.PacketID = snU16
		b.current.LostPackets = 0
		return
	}
	b.current.LostPackets |= 1 << (snU16 - b.current.PacketID - 1)
}

func (b *nackPairBuilder) flush() {
	if b.current.PacketID != 0 {
		b.pairs = append(b.pairs, b.current)
	}
}

func (b *nackPairBuilder) build() []rtcp.NackPair {
	b.flush()
	return b.pairs
}
