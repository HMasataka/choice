package sfu

import (
	"encoding/binary"
	"strings"
	"sync/atomic"

	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/pion/webrtc/v4"
)

func setVP8TemporalLayer(p *buffer.ExtPacket, d *downTrack) (buf []byte, picID uint16, tlz0Idx uint8, drop bool) {
	pkt, ok := p.Payload.(buffer.VP8)
	if !ok {
		return
	}

	layer := atomic.LoadInt32(&d.temporalLayer)
	currentLayer := uint16(layer)
	currentTargetLayer := uint16(layer >> 16)
	// Check if temporal getLayer is requested
	if currentTargetLayer != currentLayer {
		if pkt.TID <= uint8(currentTargetLayer) {
			atomic.StoreInt32(&d.temporalLayer, int32(currentTargetLayer)<<16|int32(currentTargetLayer))
		}
	} else if pkt.TID > uint8(currentLayer) {
		drop = true
		return
	}

	buf = *d.payload
	buf = buf[:len(p.Packet.Payload)]
	copy(buf, p.Packet.Payload)

	picID = pkt.PictureID - d.simulcast.refPicID + d.simulcast.pRefPicID + 1
	tlz0Idx = pkt.TL0PICIDX - d.simulcast.refTlZIdx + d.simulcast.pRefTlZIdx + 1

	if p.Head {
		d.simulcast.lPicID = picID
		d.simulcast.lTlZIdx = tlz0Idx
	}

	modifyVP8TemporalPayload(buf, pkt.PicIDIdx, pkt.TlzIdx, picID, tlz0Idx, pkt.MBit)

	return
}

func modifyVP8TemporalPayload(payload []byte, picIDIdx, tlz0Idx int, picID uint16, tlz0ID uint8, mBit bool) {
	pid := make([]byte, 2)
	binary.BigEndian.PutUint16(pid, picID)
	payload[picIDIdx] = pid[0]
	if mBit {
		payload[picIDIdx] |= 0x80
		payload[picIDIdx+1] = pid[1]
	}
	payload[tlz0Idx] = tlz0ID
}

// Do a fuzzy find for a codec in the list of codecs
// Used for lookup up a codec in an existing list to find a match
func codecParametersFuzzySearch(needle webrtc.RTPCodecParameters, haystack []webrtc.RTPCodecParameters) (webrtc.RTPCodecParameters, error) {
	// First attempt to match on MimeType + SDPFmtpLine
	for _, c := range haystack {
		if strings.EqualFold(c.MimeType, needle.MimeType) &&
			c.SDPFmtpLine == needle.SDPFmtpLine {
			return c, nil
		}
	}

	// Fallback to just MimeType
	for _, c := range haystack {
		if strings.EqualFold(c.MimeType, needle.MimeType) {
			return c, nil
		}
	}

	return webrtc.RTPCodecParameters{}, webrtc.ErrCodecNotFound
}
