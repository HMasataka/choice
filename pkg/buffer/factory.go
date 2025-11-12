package buffer

import (
	"io"
	"sync"

	"github.com/pion/transport/packetio"
)

type Factory struct {
	sync.RWMutex
	videoPool   *sync.Pool
	audioPool   *sync.Pool
	rtpBuffers  map[uint32]*Buffer
	rtcpReaders map[uint32]*RTCPReader
}

func NewBufferFactory(trackingPackets int) *Factory {
	return &Factory{
		// ビデオ用バッファプールの初期化
		videoPool: &sync.Pool{
			// 新しいバッファが必要な時に呼ばれる関数
			New: func() interface{} {
				// trackingPackets * maxPktSize バイトのバッファを作成
				// 例: 500 * 1500 = 750KB
				b := make([]byte, trackingPackets*maxPktSize)
				// ポインタを返す（Bucketでsrcとして使用）
				return &b
			},
		},
		// 音声用バッファプールの初期化
		audioPool: &sync.Pool{
			// 新しいバッファが必要な時に呼ばれる関数
			New: func() interface{} {
				// maxPktSize * 25 バイトのバッファを作成
				// 例: 1500 * 25 = 37.5KB
				// 音声はビデオより小さいバッファで十分
				b := make([]byte, maxPktSize*25)
				// ポインタを返す（Bucketでsrcとして使用）
				return &b
			},
		},
		// SSRCをキーとするBufferのマップ
		rtpBuffers: make(map[uint32]*Buffer),
		// SSRCをキーとするRTCPReaderのマップ
		rtcpReaders: make(map[uint32]*RTCPReader),
	}
}

func (f *Factory) GetOrNew(packetType packetio.BufferPacketType, ssrc uint32) io.ReadWriteCloser {
	f.Lock()
	defer f.Unlock()
	// パケットタイプに応じて適切なバッファを取得または作成
	switch packetType {
	case packetio.RTCPBufferPacket:
		// RTCPパケット用のリーダー
		// 既に存在する場合はそれを返す
		if reader, ok := f.rtcpReaders[ssrc]; ok {
			return reader
		}
		// 新しいRTCPReaderを作成
		reader := NewRTCPReader(ssrc)
		// マップに登録
		f.rtcpReaders[ssrc] = reader
		// クローズ時にマップから削除するコールバックを設定
		reader.OnClose(func() {
			f.Lock()
			// マップからこのSSRCのエントリを削除
			delete(f.rtcpReaders, ssrc)
			f.Unlock()
		})
		return reader
	case packetio.RTPBufferPacket:
		// RTPパケット用のバッファ
		// 既に存在する場合はそれを返す
		if reader, ok := f.rtpBuffers[ssrc]; ok {
			return reader
		}
		// 新しいBufferを作成（プールへの参照を渡す）
		buffer := NewBuffer(ssrc, f.videoPool, f.audioPool)
		// マップに登録
		f.rtpBuffers[ssrc] = buffer
		// クローズ時にマップから削除するコールバックを設定
		buffer.OnClose(func() {
			f.Lock()
			// マップからこのSSRCのエントリを削除
			delete(f.rtpBuffers, ssrc)
			f.Unlock()
		})
		return buffer
	}
	// 未知のパケットタイプ
	return nil
}

func (f *Factory) GetBufferPair(ssrc uint32) (*Buffer, *RTCPReader) {
	// 読み取りロック（複数ゴルーチンからの並行読み取りを許可）
	f.RLock()
	defer f.RUnlock()
	// 指定されたSSRCのBufferとRTCPReaderを同時に取得
	// RTPとRTCPを一緒に処理する場合に便利
	return f.rtpBuffers[ssrc], f.rtcpReaders[ssrc]
}

func (f *Factory) GetBuffer(ssrc uint32) *Buffer {
	// 読み取りロック（複数ゴルーチンからの並行読み取りを許可）
	f.RLock()
	defer f.RUnlock()
	// 指定されたSSRCのBufferを取得
	// 存在しない場合はnilを返す
	return f.rtpBuffers[ssrc]
}

func (f *Factory) GetRTCPReader(ssrc uint32) *RTCPReader {
	// 読み取りロック（複数ゴルーチンからの並行読み取りを許可）
	f.RLock()
	defer f.RUnlock()
	// 指定されたSSRCのRTCPReaderを取得
	// 存在しない場合はnilを返す
	return f.rtcpReaders[ssrc]
}
