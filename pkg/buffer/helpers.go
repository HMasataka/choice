package buffer

import (
	"encoding/binary"
	"errors"
	"log/slog"
)

var (
	errShortPacket = errors.New("packet is not large enough")
	errNilPacket   = errors.New("invalid nil packet")
)

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
	if len(payload) < 1 {
		return errShortPacket
	}

	r := &vp8Reader{payload: payload, len: len(payload)}
	S := payload[0]&0x10 > 0
	hasExtension := payload[0]&0x80 > 0

	if !hasExtension {
		return p.parseSimple(r, S)
	}
	return p.parseExtended(r, S)
}

type vp8Reader struct {
	payload []byte
	len     int
	idx     int
}

func (r *vp8Reader) advance() error {
	r.idx++
	if r.len < r.idx+1 {
		return errShortPacket
	}
	return nil
}

func (r *vp8Reader) current() byte {
	return r.payload[r.idx]
}

func (p *VP8) parseSimple(r *vp8Reader, S bool) error {
	if err := r.advance(); err != nil {
		return err
	}
	p.IsKeyFrame = r.current()&0x01 == 0 && S
	return nil
}

func (p *VP8) parseExtended(r *vp8Reader, S bool) error {
	if err := r.advance(); err != nil {
		return err
	}

	p.TemporalSupported = r.current()&0x20 > 0
	K := r.current()&0x10 > 0
	L := r.current()&0x40 > 0
	hasPictureID := r.current()&0x80 > 0

	if hasPictureID {
		if err := p.parsePictureID(r); err != nil {
			return err
		}
	}
	if L {
		if err := p.parseTL0PICIDX(r); err != nil {
			return err
		}
	}
	if p.TemporalSupported || K {
		if err := p.parseTID(r); err != nil {
			return err
		}
	}

	if r.idx >= r.len {
		return errShortPacket
	}
	if err := r.advance(); err != nil {
		return err
	}
	p.IsKeyFrame = r.current()&0x01 == 0 && S
	return nil
}

func (p *VP8) parsePictureID(r *vp8Reader) error {
	if err := r.advance(); err != nil {
		return err
	}
	p.PicIDIdx = r.idx
	pid := r.current() & 0x7f

	if r.current()&0x80 > 0 {
		if err := r.advance(); err != nil {
			return err
		}
		p.MBit = true
		p.PictureID = binary.BigEndian.Uint16([]byte{pid, r.current()})
	} else {
		p.PictureID = uint16(pid)
	}
	return nil
}

func (p *VP8) parseTL0PICIDX(r *vp8Reader) error {
	if err := r.advance(); err != nil {
		return err
	}
	p.TlzIdx = r.idx
	if r.idx >= r.len {
		return errShortPacket
	}
	p.TL0PICIDX = r.current()
	return nil
}

func (p *VP8) parseTID(r *vp8Reader) error {
	if err := r.advance(); err != nil {
		return err
	}
	p.TID = (r.current() & 0xc0) >> 6
	return nil
}

// isH264Keyframe はH.264ペイロードがキーフレームかを判定する
// Credit: https://github.com/jech/galene
func isH264Keyframe(payload []byte) bool {
	if len(payload) < 1 {
		return false
	}

	nalu := payload[0] & 0x1F
	switch {
	case nalu == 0:
		return false
	case nalu <= 23:
		return nalu == 5
	case nalu >= 24 && nalu <= 27:
		return isH264KeyframeInSTAP(payload, nalu)
	case nalu == 28 || nalu == 29:
		return isH264KeyframeInFU(payload)
	default:
		return false
	}
}

// isH264KeyframeInSTAP はSTAP-A/B, MTAP16/24内のキーフレームを検出する
func isH264KeyframeInSTAP(payload []byte, nalu byte) bool {
	i := 1
	if nalu >= 25 {
		i += 2
	}

	for i < len(payload) {
		length, ok := readSTAPLength(payload, i)
		if !ok {
			return false
		}
		i += 2

		if i+int(length) > len(payload) {
			return false
		}

		if isKeyframeNALU(payload, i, nalu, length) {
			return true
		}
		i += int(length)
	}
	return false
}

func readSTAPLength(payload []byte, i int) (uint16, bool) {
	if i+2 > len(payload) {
		return 0, false
	}
	return uint16(payload[i])<<8 | uint16(payload[i+1]), true
}

func isKeyframeNALU(payload []byte, i int, nalu byte, length uint16) bool {
	offset := getSTAPOffset(nalu)
	if offset >= int(length) {
		return false
	}

	n := payload[i+offset] & 0x1F
	if n == 7 {
		return true
	}
	if n >= 24 {
		slog.Debug("unexpected high NALU type in STAP processing", "nalu_type", n)
	}
	return false
}

func getSTAPOffset(nalu byte) int {
	switch nalu {
	case 26:
		return 3
	case 27:
		return 4
	default:
		return 0
	}
}

// isH264KeyframeInFU はFU-A/B内のキーフレームを検出する
func isH264KeyframeInFU(payload []byte) bool {
	if len(payload) < 2 {
		return false
	}
	isStart := payload[1]&0x80 != 0
	if !isStart {
		return false
	}
	return payload[1]&0x1F == 7
}
