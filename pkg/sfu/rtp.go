package sfu

import (
	"github.com/pion/rtp"
)

// rtpSequencer rewrites RTP sequence numbers and timestamps for seamless layer switching.
type rtpSequencer struct {
	lastSeq   uint16
	seqOffset uint16
	lastTS    uint32
	tsOffset  uint32
	lastSSRC  uint32
	inited    bool
}

func newRTPSequencer() *rtpSequencer {
	return &rtpSequencer{}
}

// Rewrite adjusts the packet's sequence number, timestamp, and SSRC.
func (s *rtpSequencer) Rewrite(packet *rtp.Packet, ssrc uint32) *rtp.Packet {
	if !s.inited {
		s.lastSeq = packet.SequenceNumber
		s.lastTS = packet.Timestamp
		s.lastSSRC = ssrc
		s.inited = true
	}

	// Handle SSRC change (layer switch)
	if packet.SSRC != s.lastSSRC {
		s.seqOffset = s.lastSeq - packet.SequenceNumber + 1
		s.tsOffset = s.lastTS - packet.Timestamp + 1
		s.lastSSRC = packet.SSRC
	}

	newPacket := packet.Clone()
	newPacket.SequenceNumber = packet.SequenceNumber + s.seqOffset
	newPacket.Timestamp = packet.Timestamp + s.tsOffset
	newPacket.SSRC = ssrc

	s.lastSeq = newPacket.SequenceNumber
	s.lastTS = newPacket.Timestamp

	return newPacket
}

// IsKeyframe checks if an RTP packet contains a keyframe.
func IsKeyframe(payload []byte, codecType string) bool {
	if len(payload) == 0 {
		return false
	}

	switch codecType {
	case "video/VP8":
		return isVP8Keyframe(payload)
	case "video/VP9":
		return isVP9Keyframe(payload)
	case "video/H264":
		return isH264Keyframe(payload)
	default:
		return false
	}
}

// isVP8Keyframe checks if a VP8 payload is a keyframe.
func isVP8Keyframe(payload []byte) bool {
	if len(payload) < 1 {
		return false
	}
	// VP8 keyframe: S bit set and partition index = 0, P bit clear
	return (payload[0]&0x01) == 0 && (payload[0]&0x10) != 0
}

// isVP9Keyframe checks if a VP9 payload is a keyframe.
func isVP9Keyframe(payload []byte) bool {
	if len(payload) < 1 {
		return false
	}
	// VP9 keyframe: P bit clear
	return (payload[0] & 0x40) == 0
}

// isH264Keyframe checks if an H264 payload is a keyframe.
func isH264Keyframe(payload []byte) bool {
	if len(payload) < 1 {
		return false
	}
	// H264 keyframe: NAL type 5 (IDR) or 7 (SPS)
	nalType := payload[0] & 0x1F
	return nalType == 5 || nalType == 7
}
