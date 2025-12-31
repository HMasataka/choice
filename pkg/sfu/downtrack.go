package sfu

import (
	"sync"
	"sync/atomic"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

type DownTrack struct {
	subscriber   *Subscriber
	receiver     *Receiver
	track        *webrtc.TrackLocalStaticRTP
	sender       *webrtc.RTPSender
	mu           sync.RWMutex
	closed       atomic.Bool
	sequencer    *Sequencer
	lastSSRC     uint32
	ssrcRewriter bool
}

type Sequencer struct {
	lastSeq    uint16
	seqOffset  uint16
	lastTS     uint32
	tsOffset   uint32
	lastSSRC   uint32
	ssrcOffset uint32
	inited     bool
}

func NewSequencer() *Sequencer {
	return &Sequencer{}
}

func (s *Sequencer) Rewrite(packet *rtp.Packet, ssrc uint32) *rtp.Packet {
	if !s.inited {
		s.lastSeq = packet.SequenceNumber
		s.lastTS = packet.Timestamp
		s.lastSSRC = ssrc
		s.inited = true
	}

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

func NewDownTrack(subscriber *Subscriber, receiver *Receiver) (*DownTrack, error) {
	codec := receiver.Codec()

	track, err := webrtc.NewTrackLocalStaticRTP(
		codec.RTPCodecCapability,
		receiver.TrackID(),
		receiver.StreamID(),
	)
	if err != nil {
		return nil, err
	}

	sender, err := subscriber.pc.AddTrack(track)
	if err != nil {
		return nil, err
	}

	dt := &DownTrack{
		subscriber: subscriber,
		receiver:   receiver,
		track:      track,
		sender:     sender,
		sequencer:  NewSequencer(),
	}

	go dt.handleRTCP()

	return dt, nil
}

func (d *DownTrack) handleRTCP() {
	for {
		if d.closed.Load() {
			return
		}

		packets, _, err := d.sender.ReadRTCP()
		if err != nil {
			return
		}

		for range packets {
		}
	}
}

func (d *DownTrack) WriteRTP(packet *rtp.Packet) error {
	if d.closed.Load() {
		return nil
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	rewrittenPacket := d.sequencer.Rewrite(packet, uint32(d.sender.GetParameters().Encodings[0].SSRC))

	return d.track.WriteRTP(rewrittenPacket)
}

func (d *DownTrack) Track() *webrtc.TrackLocalStaticRTP {
	return d.track
}

func (d *DownTrack) Receiver() *Receiver {
	return d.receiver
}

func (d *DownTrack) Close() error {
	if d.closed.Swap(true) {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.receiver != nil {
		d.receiver.RemoveDownTrack(d)
	}

	if d.subscriber != nil && d.sender != nil {
		d.subscriber.pc.RemoveTrack(d.sender)
	}

	return nil
}
