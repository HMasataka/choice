package buffer

import (
	"io"
	"sync"

	"github.com/pion/transport/v3/packetio"
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
		videoPool: &sync.Pool{
			New: func() any {
				// trackingPackets * maxPktSize バイトのバッファを作成
				// 例: 500 * 1500 = 750KB
				b := make([]byte, trackingPackets*maxPktSize)
				return &b
			},
		},
		audioPool: &sync.Pool{
			New: func() any {
				// maxPktSize * 25 バイトのバッファを作成
				// 例: 1500 * 25 = 37.5KB
				// 音声はビデオより小さいバッファで十分
				b := make([]byte, maxPktSize*25)
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

	switch packetType {
	case packetio.RTCPBufferPacket:
		if reader, ok := f.rtcpReaders[ssrc]; ok {
			return reader
		}

		reader := NewRTCPReader(ssrc)

		f.rtcpReaders[ssrc] = reader
		reader.OnClose(func() {
			f.Lock()
			delete(f.rtcpReaders, ssrc)
			f.Unlock()
		})
		return reader
	case packetio.RTPBufferPacket:
		if reader, ok := f.rtpBuffers[ssrc]; ok {
			return reader
		}

		buffer := NewBuffer(ssrc, f.videoPool, f.audioPool)

		f.rtpBuffers[ssrc] = buffer
		buffer.OnClose(func() {
			f.Lock()
			delete(f.rtpBuffers, ssrc)
			f.Unlock()
		})
		return buffer
	}

	return nil
}

func (f *Factory) GetBufferPair(ssrc uint32) (*Buffer, *RTCPReader) {
	f.RLock()
	defer f.RUnlock()

	return f.rtpBuffers[ssrc], f.rtcpReaders[ssrc]
}

func (f *Factory) GetBuffer(ssrc uint32) *Buffer {
	f.RLock()
	defer f.RUnlock()

	return f.rtpBuffers[ssrc]
}

func (f *Factory) GetRTCPReader(ssrc uint32) *RTCPReader {
	f.RLock()
	defer f.RUnlock()

	return f.rtcpReaders[ssrc]
}
