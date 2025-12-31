package sfu

import (
	"log"
	"sync"
	"sync/atomic"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// DownTrack represents an outgoing track to a subscriber.
// It receives RTP packets from a Receiver and writes them to a local track.
type DownTrack struct {
	subscriber *Subscriber
	receiver   *Receiver
	track      *webrtc.TrackLocalStaticRTP
	sender     *webrtc.RTPSender
	sequencer  *rtpSequencer
	closed     atomic.Bool
	mu         sync.RWMutex
}

// NewDownTrack creates a new downtrack for a subscriber.
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
		sequencer:  newRTPSequencer(),
	}

	go dt.readRTCP()

	// Request keyframe immediately for video tracks
	if receiver.Kind() == webrtc.RTPCodecTypeVideo {
		go dt.RequestKeyframe()
	}

	return dt, nil
}

// readRTCP reads RTCP packets from the sender (required to keep the connection alive).
func (d *DownTrack) readRTCP() {
	for {
		if d.closed.Load() {
			return
		}
		if _, _, err := d.sender.ReadRTCP(); err != nil {
			return
		}
	}
}

// RequestKeyframe sends a PLI (Picture Loss Indication) to request a keyframe.
func (d *DownTrack) RequestKeyframe() {
	if d.receiver == nil {
		return
	}

	log.Printf("[DownTrack] Requesting keyframe for track %s", d.receiver.TrackID())
	d.receiver.SendPLI()
}

// WriteRTP writes an RTP packet to the track with sequence number rewriting.
func (d *DownTrack) WriteRTP(packet *rtp.Packet) error {
	if d.closed.Load() {
		return nil
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	ssrc := uint32(d.sender.GetParameters().Encodings[0].SSRC)
	rewritten := d.sequencer.Rewrite(packet, ssrc)

	return d.track.WriteRTP(rewritten)
}

// Track returns the local track.
func (d *DownTrack) Track() *webrtc.TrackLocalStaticRTP {
	return d.track
}

// Receiver returns the receiver this downtrack is connected to.
func (d *DownTrack) Receiver() *Receiver {
	return d.receiver
}

// Close closes the downtrack.
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

// rtpSequencer rewrites RTP sequence numbers and timestamps for seamless playback.
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

	// Handle SSRC change (track switch)
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
