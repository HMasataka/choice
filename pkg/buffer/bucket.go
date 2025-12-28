package buffer

import (
	"encoding/binary"
	"math"
)

const maxPktSize = 1500

// RTPヘッダーからシーケンス番号を取得します。
// RTPヘッダーフォーマット: [0-1]:V/P/X/CC, [2-3]:M/PT, [4-5]:Sequence Number
func readSequenceNumber(buf []byte, offset int) uint16 {
	return binary.BigEndian.Uint16(buf[offset+4 : offset+6])
}

func readPacketSize(buf []byte, offset int) int {
	return int(binary.BigEndian.Uint16(buf[offset : offset+2]))
}

// Bucket はRTPパケットのリングバッファ
type Bucket struct {
	buf                []byte
	init               bool
	step               int
	headSequenceNumber uint16
	maxSteps           int
}

func NewBucket(buf *[]byte) *Bucket {
	return &Bucket{
		buf:      *buf,
		maxSteps: int(math.Floor(float64(len(*buf))/float64(maxPktSize))) - 1,
	}
}

func (b *Bucket) AddPacket(pkt []byte, sequenceNumber uint16, latest bool) ([]byte, error) {
	if !b.init {
		b.headSequenceNumber = sequenceNumber - 1
		b.init = true
	}

	if !latest {
		return b.set(sequenceNumber, pkt)
	}

	diff := sequenceNumber - b.headSequenceNumber
	if diff > 1 {
		b.advanceStep(diff - 1)
	}
	b.headSequenceNumber = sequenceNumber

	return b.push(pkt), nil
}

func (b *Bucket) advanceStep(n uint16) {
	if n == 0 {
		return
	}
	slots := b.maxSteps + 1
	b.step = (b.step + int(n)) % slots
}

func (b *Bucket) GetPacket(buf []byte, sequenceNumber uint16) (int, error) {
	packet := b.get(sequenceNumber)
	if packet == nil {
		return 0, errPacketNotFound
	}

	n := len(packet)
	if cap(buf) < n {
		return 0, errBufferTooSmall
	}

	if len(buf) < n {
		buf = buf[:n]
	}

	copy(buf, packet)

	return n, nil
}

func (b *Bucket) push(packet []byte) []byte {
	slotOffset := b.step * maxPktSize
	binary.BigEndian.PutUint16(b.buf[slotOffset:], uint16(len(packet)))
	offset := slotOffset + 2
	copy(b.buf[offset:], packet)

	b.step = (b.step + 1) % (b.maxSteps + 1)

	return b.buf[offset : offset+len(packet)]
}

func (b *Bucket) get(sequenceNumber uint16) []byte {
	position, ok := b.position(sequenceNumber)
	if !ok {
		return nil
	}

	offset := position * maxPktSize
	if offset >= len(b.buf) {
		return nil
	}

	if readSequenceNumber(b.buf, offset) != sequenceNumber {
		return nil
	}

	size := readPacketSize(b.buf, offset)
	return b.buf[offset+2 : offset+2+size]
}

func (b *Bucket) set(sequenceNumber uint16, pkt []byte) ([]byte, error) {
	position, ok := b.position(sequenceNumber)
	if !ok {
		return nil, errPacketTooOld
	}

	offset := position * maxPktSize
	if offset >= len(b.buf) {
		return nil, errPacketTooOld
	}

	if readSequenceNumber(b.buf, offset) == sequenceNumber {
		return nil, errRTXPacket
	}

	binary.BigEndian.PutUint16(b.buf[offset:], uint16(len(pkt)))
	copy(b.buf[offset+2:], pkt)
	return b.buf[offset+2 : offset+2+len(pkt)], nil
}

// position はシーケンス番号からスロット位置を算出する
func (b *Bucket) position(sequenceNumber uint16) (int, bool) {
	back := int(b.headSequenceNumber - sequenceNumber + 1)

	position := b.step - back
	if position < 0 {
		if -position > b.maxSteps+1 {
			return 0, false
		}
		position = b.maxSteps + position + 1
	}

	return position, true
}
