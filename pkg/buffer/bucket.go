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

type Bucket struct {
	buf []byte

	init               bool
	step               int
	headSequenceNumber uint16
	maxSteps           int
}

// NewBucket はバッファポインタからBucketを作成します。
func NewBucket(buf *[]byte) *Bucket {
	return &Bucket{
		buf: *buf,
		// バッファに収まる最大パケット数を計算
		maxSteps: int(math.Floor(float64(len(*buf))/float64(maxPktSize))) - 1,
	}
}

// AddPacket はパケットをバッファに追加します。
func (b *Bucket) AddPacket(pkt []byte, sequenceNumber uint16, latest bool) ([]byte, error) {
	// 初回パケット時の初期化
	if !b.init {
		b.headSequenceNumber = sequenceNumber - 1
		b.init = true
	}

	// 遅延パケットは適切な位置に格納
	if !latest {
		return b.set(sequenceNumber, pkt)
	}

	// 最新パケット: 欠落分だけstepを進める（diff-1）
	diff := sequenceNumber - b.headSequenceNumber
	if diff > 1 {
		b.advanceStep(diff - 1)
	}
	// ヘッドを更新してから現在のパケットを書き込む
	b.headSequenceNumber = sequenceNumber

	return b.push(pkt), nil
}

// advanceStep はリングバッファの step を n スロット分前進させます（ラップアラウンド対応）。
func (b *Bucket) advanceStep(n uint16) {
	if n == 0 {
		return
	}
	slots := b.maxSteps + 1
	b.step = (b.step + int(n)) % slots
}

// GetPacket は指定されたシーケンス番号のパケットを取得します。
func (b *Bucket) GetPacket(buf []byte, sequenceNumber uint16) (int, error) {
	packet := b.get(sequenceNumber)
	if packet == nil {
		return 0, errPacketNotFound
	}

	// 返すバイト数を設定
	n := len(packet)

	// 提供されたバッファの容量が不足している場合
	// cap()でバッファの容量を確認
	if cap(buf) < n {
		return 0, errBufferTooSmall
	}

	if len(buf) < n {
		buf = buf[:n]
	}

	copy(buf, packet)

	return n, nil
}

func (b *Bucket) push(pkt []byte) []byte {
	// パケットサイズを先頭2バイトに書き込み(big endian)
	binary.BigEndian.PutUint16(b.buf[b.step*maxPktSize:], uint16(len(pkt)))
	offset := b.step*maxPktSize + 2
	copy(b.buf[offset:], pkt)

	// 次のスロットに移動
	// リングバッファなので、stepは循環する
	b.step++
	if b.step > b.maxSteps {
		b.step = 0
	}

	// 書き込んだパケットデータへのスライスを返す
	// このスライスはバッファ内の実際のメモリを指すため、コピー不要
	return b.buf[offset : offset+len(pkt)]
}

// get は指定されたシーケンス番号のパケットをバッファから取得します。
// sequenceNumber: 取得したいパケットのシーケンス番号
// 戻り値: パケットデータへのスライス(見つからない場合はnil)
func (b *Bucket) get(sequenceNumber uint16) []byte {
	// headSNからの相対位置を計算
	pos := b.step - int(b.headSequenceNumber-sequenceNumber+1)
	// 位置が負の場合(リングバッファを巻き戻す)
	if pos < 0 {
		// 範囲外の場合(古すぎるパケット)
		if pos*-1 > b.maxSteps+1 {
			return nil
		}
		// リングバッファの後方から計算
		pos = b.maxSteps + pos + 1
	}

	offset := pos * maxPktSize
	if offset > len(b.buf) {
		return nil
	}

	if readSequenceNumber(b.buf, offset) != sequenceNumber {
		// シーケンス番号が一致しない（パケットが存在しないか、上書きされた）
		return nil
	}

	size := readPacketSize(b.buf, offset)
	return b.buf[offset+2 : offset+2+size]
}

// set は古いパケット(順序が乱れたパケット)を適切な位置に設定します。
func (b *Bucket) set(sequenceNumber uint16, pkt []byte) ([]byte, error) {
	// パケットが古すぎる場合(バッファウィンドウ外)
	if b.headSequenceNumber-sequenceNumber >= uint16(b.maxSteps+1) {
		return nil, errPacketTooOld
	}
	// headSNからの相対位置を計算
	// get()と同じロジックで位置を算出
	pos := b.step - int(b.headSequenceNumber-sequenceNumber+1)
	// 位置が負の場合(リングバッファを巻き戻す)
	if pos < 0 {
		// リングバッファの後方から計算
		pos = b.maxSteps + pos + 1
	}

	offset := pos * maxPktSize
	if offset > len(b.buf) || offset < 0 {
		// オフセットが範囲外（実装エラー）
		return nil, errPacketTooOld
	}

	// パケットが既に存在する場合は上書きしない(重複パケット検出)
	if readSequenceNumber(b.buf, offset) == sequenceNumber {
		return nil, errRTXPacket
	}

	// パケットを書き込み
	binary.BigEndian.PutUint16(b.buf[offset:], uint16(len(pkt)))
	copy(b.buf[offset+2:], pkt)
	return b.buf[offset+2 : offset+2+len(pkt)], nil
}
