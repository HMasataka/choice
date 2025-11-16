package buffer

import (
	"encoding/binary"
	"math"
)

const maxPktSize = 1500

// RTPヘッダーからシーケンス番号を取得します。
// RTPヘッダーフォーマット: [0-1]:V/P/X/CC, [2-3]:M/PT, [4-5]:Sequence Number
// サイズフィールド(2バイト)の後、RTPヘッダーのSNフィールドを確認
func readSequenceNumber(buf []byte, offset int) uint16 {
	return binary.BigEndian.Uint16(buf[offset+4 : offset+6])
}

func readPacketSize(buf []byte, offset int) int {
	return int(binary.BigEndian.Uint16(buf[offset : offset+2]))
}

type Bucket struct {
	buf []byte
	src *[]byte

	init               bool
	step               int
	headSequenceNumber uint16
	maxSteps           int
}

// NewBucket はバッファポインタからBucketを作成します。
// buf: 事前に確保されたバイトスライスへのポインタ
// 戻り値: 初期化されたBucket
func NewBucket(buf *[]byte) *Bucket {
	return &Bucket{
		// プールに返却するための元のポインタを保持
		src: buf,
		// 実際のバイトスライスを保持
		buf: *buf,
		// バッファに収まる最大パケット数を計算
		maxSteps: int(math.Floor(float64(len(*buf))/float64(maxPktSize))) - 1,
	}
}

// AddPacket はパケットをバッファに追加します。
// pkt: 追加するRTPパケットのバイト列
// sn: パケットのシーケンス番号
// latest: trueの場合、このパケットは最新のパケットとして扱われる
// 戻り値: バッファ内のパケットデータへのスライス、エラー
func (b *Bucket) AddPacket(pkt []byte, sn uint16, latest bool) ([]byte, error) {
	// 初回パケット到着時の初期化
	if !b.init {
		// 最初のパケットのSN-1をheadSNとして設定
		// 例: 最初のパケットがSN=100の場合、headSN=99とする
		// これにより、push()時にstepが正しく進む
		b.headSequenceNumber = sn - 1
		b.init = true
	}

	// 古いパケット(順序が乱れたパケット)の場合
	if !latest {
		// 適切な位置を計算して設定
		return b.set(sn, pkt)
	}

	// 最新パケットの場合
	// 前回のheadSNとの差分を計算
	diff := sn - b.headSequenceNumber
	b.headSequenceNumber = sn

	// 差分の間にあるパケット(欠落)をスキップするためstepを進める
	for i := uint16(1); i < diff; i++ {
		b.step++
		if b.step >= b.maxSteps {
			b.step = 0
		}
	}

	return b.push(pkt), nil
}

// GetPacket は指定されたシーケンス番号のパケットを取得します。
// buf: パケットをコピーする先のバッファ
// sn: 取得したいパケットのシーケンス番号
// 戻り値: コピーしたバイト数、エラー
func (b *Bucket) GetPacket(buf []byte, sn uint16) (int, error) {
	packet := b.get(sn)
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

// push は最新パケットをバッファの現在位置に追加します。
// pkt: 追加するパケットデータ
// 戻り値: バッファ内のパケットデータへのスライス
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
// sn: 取得したいパケットのシーケンス番号
// 戻り値: パケットデータへのスライス(見つからない場合はnil)
func (b *Bucket) get(sn uint16) []byte {
	// headSNからの相対位置を計算
	pos := b.step - int(b.headSequenceNumber-sn+1)
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

	if readSequenceNumber(b.buf, offset) != sn {
		// シーケンス番号が一致しない（パケットが存在しないか、上書きされた）
		return nil
	}

	size := readPacketSize(b.buf, offset)
	return b.buf[offset+2 : offset+2+size]
}

// set は古いパケット(順序が乱れたパケット)を適切な位置に設定します。
// sn: パケットのシーケンス番号
// pkt: パケットデータ
// 戻り値: バッファ内のパケットデータへのスライス、エラー
func (b *Bucket) set(sn uint16, pkt []byte) ([]byte, error) {
	// パケットが古すぎる場合(バッファウィンドウ外)
	if b.headSequenceNumber-sn >= uint16(b.maxSteps+1) {
		return nil, errPacketTooOld
	}
	// headSNからの相対位置を計算
	// get()と同じロジックで位置を算出
	pos := b.step - int(b.headSequenceNumber-sn+1)
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
	if readSequenceNumber(b.buf, offset) == sn {
		return nil, errRTXPacket
	}

	// パケットを書き込み
	binary.BigEndian.PutUint16(b.buf[offset:], uint16(len(pkt)))
	copy(b.buf[offset+2:], pkt)
	return b.buf[offset+2 : offset+2+len(pkt)], nil
}
