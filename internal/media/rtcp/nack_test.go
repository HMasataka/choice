package rtcp

import (
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

func TestNewNACKHandler(t *testing.T) {
	t.Run("with nil config uses defaults", func(t *testing.T) {
		h := NewNACKHandler(nil)
		if h == nil {
			t.Fatal("expected non-nil handler")
		}
		if h.config.MaxBufferSize != 500 {
			t.Errorf("expected 500 max buffer size, got %d", h.config.MaxBufferSize)
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &NACKConfig{
			MaxBufferSize:     1000,
			MaxRetries:        5,
			RetransmitTimeout: 20 * time.Millisecond,
			RTXEnabled:        false,
		}
		h := NewNACKHandler(cfg)
		if h.config.MaxBufferSize != 1000 {
			t.Errorf("expected 1000 max buffer size, got %d", h.config.MaxBufferSize)
		}
		if h.config.RTXEnabled {
			t.Error("expected RTX to be disabled")
		}
	})
}

func TestNACKHandler_BufferPacket(t *testing.T) {
	h := NewNACKHandler(nil)

	packet := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 100,
			SSRC:           12345,
		},
		Payload: []byte{1, 2, 3, 4, 5},
	}
	raw := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	h.BufferPacket(12345, packet, raw)

	buffered := h.GetBufferedPacket(12345, 100)
	if buffered == nil {
		t.Fatal("expected buffered packet")
	}
	if buffered.Packet.SequenceNumber != 100 {
		t.Errorf("expected seq 100, got %d", buffered.Packet.SequenceNumber)
	}
}

func TestNACKHandler_HandleNACK(t *testing.T) {
	h := NewNACKHandler(nil)

	var retransmitSSRC uint32
	var retransmitPacket *rtp.Packet

	h.SetOnRetransmit(func(ssrc uint32, packet *rtp.Packet, raw []byte) {
		retransmitSSRC = ssrc
		retransmitPacket = packet
	})

	// Buffer a packet
	packet := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 100,
			SSRC:           12345,
		},
		Payload: []byte{1, 2, 3, 4, 5},
	}
	h.BufferPacket(12345, packet, nil)

	// Send NACK
	nack := &rtcp.TransportLayerNack{
		SenderSSRC: 67890,
		MediaSSRC:  12345,
		Nacks: []rtcp.NackPair{
			{PacketID: 100, LostPackets: 0},
		},
	}
	h.HandleNACK(nack)

	if retransmitSSRC != 12345 {
		t.Errorf("expected retransmit for SSRC 12345, got %d", retransmitSSRC)
	}
	if retransmitPacket == nil || retransmitPacket.SequenceNumber != 100 {
		t.Error("expected retransmit packet with seq 100")
	}
}

func TestNACKHandler_MissingPacket(t *testing.T) {
	h := NewNACKHandler(nil)

	// Send NACK for non-existent packet
	nack := &rtcp.TransportLayerNack{
		SenderSSRC: 67890,
		MediaSSRC:  12345,
		Nacks: []rtcp.NackPair{
			{PacketID: 100, LostPackets: 0},
		},
	}
	h.HandleNACK(nack)

	stats := h.GetStats()
	if stats.TotalPacketsMissing != 1 {
		t.Errorf("expected 1 missing packet, got %d", stats.TotalPacketsMissing)
	}
}

func TestNACKHandler_RetryLimit(t *testing.T) {
	cfg := &NACKConfig{
		MaxBufferSize:     500,
		MaxRetries:        2,
		RetransmitTimeout: 1 * time.Millisecond,
	}
	h := NewNACKHandler(cfg)

	var retransmitCount int
	h.SetOnRetransmit(func(ssrc uint32, packet *rtp.Packet, raw []byte) {
		retransmitCount++
	})

	// Buffer a packet
	packet := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 100,
			SSRC:           12345,
		},
	}
	h.BufferPacket(12345, packet, nil)

	// Send multiple NACKs
	nack := &rtcp.TransportLayerNack{
		MediaSSRC: 12345,
		Nacks:     []rtcp.NackPair{{PacketID: 100, LostPackets: 0}},
	}

	// First two should trigger retransmit
	h.HandleNACK(nack)
	time.Sleep(2 * time.Millisecond)
	h.HandleNACK(nack)
	time.Sleep(2 * time.Millisecond)

	// Third should be ignored (max retries reached)
	h.HandleNACK(nack)

	if retransmitCount != 2 {
		t.Errorf("expected 2 retransmits (max retries), got %d", retransmitCount)
	}
}

func TestNACKHandler_GenerateNACK(t *testing.T) {
	h := NewNACKHandler(nil)

	t.Run("empty sequence numbers", func(t *testing.T) {
		nack := h.GenerateNACK(12345, 67890, []uint16{})
		if nack != nil {
			t.Error("expected nil NACK for empty sequence numbers")
		}
	})

	t.Run("single sequence number", func(t *testing.T) {
		nack := h.GenerateNACK(12345, 67890, []uint16{100})
		if nack == nil {
			t.Fatal("expected non-nil NACK")
		}
		if nack.SenderSSRC != 12345 {
			t.Errorf("expected sender SSRC 12345, got %d", nack.SenderSSRC)
		}
		if nack.MediaSSRC != 67890 {
			t.Errorf("expected media SSRC 67890, got %d", nack.MediaSSRC)
		}
		if len(nack.Nacks) != 1 {
			t.Errorf("expected 1 NACK pair, got %d", len(nack.Nacks))
		}
	})

	t.Run("consecutive sequence numbers", func(t *testing.T) {
		nack := h.GenerateNACK(12345, 67890, []uint16{100, 101, 102})
		if nack == nil {
			t.Fatal("expected non-nil NACK")
		}
		// Should be packed into single NACK pair with bitmask
		if len(nack.Nacks) != 1 {
			t.Errorf("expected 1 NACK pair for consecutive seqs, got %d", len(nack.Nacks))
		}
	})
}

func TestNACKHandler_CreateRTXPacket(t *testing.T) {
	h := NewNACKHandler(nil)

	original := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Padding:        false,
			Extension:      false,
			Marker:         true,
			PayloadType:    96,
			SequenceNumber: 100,
			Timestamp:      123456,
			SSRC:           12345,
		},
		Payload: []byte{1, 2, 3, 4, 5},
	}

	rtx := h.CreateRTXPacket(original, 12346, 97, 200)

	if rtx.SSRC != 12346 {
		t.Errorf("expected RTX SSRC 12346, got %d", rtx.SSRC)
	}
	if rtx.PayloadType != 97 {
		t.Errorf("expected RTX PT 97, got %d", rtx.PayloadType)
	}
	if rtx.SequenceNumber != 200 {
		t.Errorf("expected RTX seq 200, got %d", rtx.SequenceNumber)
	}
	if rtx.Timestamp != original.Timestamp {
		t.Errorf("expected same timestamp, got %d", rtx.Timestamp)
	}

	// Payload should be: original seq (2 bytes) + original payload
	if len(rtx.Payload) != 7 {
		t.Errorf("expected RTX payload length 7, got %d", len(rtx.Payload))
	}
	// First two bytes should be original sequence number
	origSeq := uint16(rtx.Payload[0])<<8 | uint16(rtx.Payload[1])
	if origSeq != 100 {
		t.Errorf("expected original seq 100 in RTX payload, got %d", origSeq)
	}
}

func TestNACKHandler_ExtractOriginalFromRTX(t *testing.T) {
	h := NewNACKHandler(nil)

	// Create RTX packet
	rtxPayload := []byte{0, 100, 1, 2, 3, 4, 5} // seq 100 + payload
	rtx := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    97,
			SequenceNumber: 200,
			Timestamp:      123456,
			SSRC:           12346,
		},
		Payload: rtxPayload,
	}

	original := h.ExtractOriginalFromRTX(rtx, 12345, 96)

	if original == nil {
		t.Fatal("expected non-nil original packet")
	}
	if original.SequenceNumber != 100 {
		t.Errorf("expected original seq 100, got %d", original.SequenceNumber)
	}
	if original.SSRC != 12345 {
		t.Errorf("expected original SSRC 12345, got %d", original.SSRC)
	}
	if original.PayloadType != 96 {
		t.Errorf("expected original PT 96, got %d", original.PayloadType)
	}
	if len(original.Payload) != 5 {
		t.Errorf("expected original payload length 5, got %d", len(original.Payload))
	}
}

func TestNACKHandler_ExtractOriginalFromRTX_TooShort(t *testing.T) {
	h := NewNACKHandler(nil)

	rtx := &rtp.Packet{
		Payload: []byte{0}, // Too short - needs at least 2 bytes for seq
	}

	original := h.ExtractOriginalFromRTX(rtx, 12345, 96)
	if original != nil {
		t.Error("expected nil for too short RTX payload")
	}
}

func TestNACKHandler_Stats(t *testing.T) {
	h := NewNACKHandler(nil)

	// Buffer a packet
	packet := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 100,
			SSRC:           12345,
		},
	}
	h.BufferPacket(12345, packet, nil)

	// Send NACK
	nack := &rtcp.TransportLayerNack{
		MediaSSRC: 12345,
		Nacks:     []rtcp.NackPair{{PacketID: 100, LostPackets: 0}},
	}
	h.HandleNACK(nack)

	stats := h.GetStats()
	if stats.TotalNACKsReceived != 1 {
		t.Errorf("expected 1 NACK received, got %d", stats.TotalNACKsReceived)
	}
	if stats.TotalPacketsRequested != 1 {
		t.Errorf("expected 1 packet requested, got %d", stats.TotalPacketsRequested)
	}
	if stats.TotalPacketsRetransmitted != 1 {
		t.Errorf("expected 1 packet retransmitted, got %d", stats.TotalPacketsRetransmitted)
	}
}

func TestNACKHandler_RTXEnabled(t *testing.T) {
	cfg := &NACKConfig{
		RTXEnabled:     true,
		RTXPayloadType: 97,
	}
	h := NewNACKHandler(cfg)

	if !h.IsRTXEnabled() {
		t.Error("expected RTX to be enabled")
	}
	if h.GetRTXPayloadType() != 97 {
		t.Errorf("expected RTX PT 97, got %d", h.GetRTXPayloadType())
	}
}

func TestDefaultNACKConfig(t *testing.T) {
	cfg := DefaultNACKConfig()

	if cfg.MaxBufferSize != 500 {
		t.Errorf("expected 500 max buffer size, got %d", cfg.MaxBufferSize)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("expected 3 max retries, got %d", cfg.MaxRetries)
	}
	if cfg.RetransmitTimeout != 10*time.Millisecond {
		t.Errorf("expected 10ms retransmit timeout, got %v", cfg.RetransmitTimeout)
	}
	if !cfg.RTXEnabled {
		t.Error("expected RTX to be enabled by default")
	}
}

func TestSequenceNumbersToNACKPairs(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		pairs := sequenceNumbersToNACKPairs([]uint16{})
		if pairs != nil {
			t.Error("expected nil for empty input")
		}
	})

	t.Run("single", func(t *testing.T) {
		pairs := sequenceNumbersToNACKPairs([]uint16{100})
		if len(pairs) != 1 {
			t.Errorf("expected 1 pair, got %d", len(pairs))
		}
		if pairs[0].PacketID != 100 {
			t.Errorf("expected PacketID 100, got %d", pairs[0].PacketID)
		}
	})

	t.Run("consecutive within 16", func(t *testing.T) {
		pairs := sequenceNumbersToNACKPairs([]uint16{100, 101, 102})
		if len(pairs) != 1 {
			t.Errorf("expected 1 pair for consecutive seqs, got %d", len(pairs))
		}
	})
}
