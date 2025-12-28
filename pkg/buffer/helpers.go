package buffer

import (
	"encoding/binary"
	"errors"
	"log/slog"
	"sync/atomic"
)

var (
	errShortPacket = errors.New("packet is not large enough")
	errNilPacket   = errors.New("invalid nil packet")
)

type atomicBool int32

func (a *atomicBool) set(value bool) {
	var i int32
	if value {
		i = 1
	}
	atomic.StoreInt32((*int32)(a), i)
}

func (a *atomicBool) get() bool {
	return atomic.LoadInt32((*int32)(a)) != 0
}

// VP8 はVP8 RTPペイロード記述子をパースする（RFC 7741）
type VP8 struct {
	TemporalSupported bool
	PictureID         uint16
	PicIDIdx          int
	MBit              bool
	TL0PICIDX         uint8
	TlzIdx            int
	TID               uint8
	IsKeyFrame        bool
}

func (p *VP8) Unmarshal(payload []byte) error {
	if payload == nil {
		return errNilPacket
	}

	payloadLen := len(payload)

	if payloadLen < 1 {
		return errShortPacket
	}

	idx := 0
	S := payload[idx]&0x10 > 0
	if payload[idx]&0x80 > 0 {
		idx++
		if payloadLen < idx+1 {
			return errShortPacket
		}
		p.TemporalSupported = payload[idx]&0x20 > 0
		K := payload[idx]&0x10 > 0
		L := payload[idx]&0x40 > 0
		if payload[idx]&0x80 > 0 {
			idx++
			if payloadLen < idx+1 {
				return errShortPacket
			}
			p.PicIDIdx = idx
			pid := payload[idx] & 0x7f
			if payload[idx]&0x80 > 0 {
				idx++
				if payloadLen < idx+1 {
					return errShortPacket
				}
				p.MBit = true
				p.PictureID = binary.BigEndian.Uint16([]byte{pid, payload[idx]})
			} else {
				p.PictureID = uint16(pid)
			}
		}
		if L {
			idx++
			if payloadLen < idx+1 {
				return errShortPacket
			}
			p.TlzIdx = idx

			if int(idx) >= payloadLen {
				return errShortPacket
			}
			p.TL0PICIDX = payload[idx]
		}
		if p.TemporalSupported || K {
			idx++
			if payloadLen < idx+1 {
				return errShortPacket
			}
			p.TID = (payload[idx] & 0xc0) >> 6
		}
		if idx >= payloadLen {
			return errShortPacket
		}
		idx++
		if payloadLen < idx+1 {
			return errShortPacket
		}
		p.IsKeyFrame = payload[idx]&0x01 == 0 && S
	} else {
		idx++
		if payloadLen < idx+1 {
			return errShortPacket
		}
		p.IsKeyFrame = payload[idx]&0x01 == 0 && S
	}
	return nil
}

// isH264Keyframe はH.264ペイロードがキーフレームかを判定する
// Credit: https://github.com/jech/galene
func isH264Keyframe(payload []byte) bool {
	if len(payload) < 1 {
		return false
	}
	nalu := payload[0] & 0x1F
	if nalu == 0 {
		return false
	} else if nalu <= 23 {
		return nalu == 5
	} else if nalu == 24 || nalu == 25 || nalu == 26 || nalu == 27 {
		// STAP-A, STAP-B, MTAP16 or MTAP24
		i := 1
		if nalu == 25 || nalu == 26 || nalu == 27 {
			i += 2
		}
		for i < len(payload) {
			if i+2 > len(payload) {
				return false
			}
			length := uint16(payload[i])<<8 |
				uint16(payload[i+1])
			i += 2
			if i+int(length) > len(payload) {
				return false
			}
			offset := 0
			if nalu == 26 {
				offset = 3
			} else if nalu == 27 {
				offset = 4
			}
			if offset >= int(length) {
				return false
			}
			n := payload[i+offset] & 0x1F
			if n == 7 {
				return true
			} else if n >= 24 {
				slog.Debug("unexpected high NALU type in STAP processing", "nalu_type", n)
			}
			i += int(length)
		}
		if i == len(payload) {
			return false
		}
		return false
	} else if nalu == 28 || nalu == 29 {
		// FU-A or FU-B
		if len(payload) < 2 {
			return false
		}
		if (payload[1] & 0x80) == 0 {
			return false
		}
		return payload[1]&0x1F == 7
	}

	return false
}
