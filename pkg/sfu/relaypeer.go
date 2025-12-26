package sfu

import (
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/HMasataka/choice/pkg/relay"
	"github.com/pion/rtcp"
	"github.com/pion/transport/v3/packetio"
	"github.com/pion/webrtc/v4"
)

type RelayPeer struct {
	mu sync.RWMutex

	peer         *relay.Peer
	session      Session
	router       Router
	config       *WebRTCTransportConfig
	tracks       []PublisherTrack
	relayPeers   []*relay.Peer
	dataChannels []*webrtc.DataChannel
}

func NewRelayPeer(peer *relay.Peer, session Session, config *WebRTCTransportConfig) *RelayPeer {
	r := NewRouter(peer.ID(), session, config)

	r.SetRTCPWriter(peer.WriteRTCP)

	rp := &RelayPeer{
		peer:    peer,
		router:  r,
		config:  config,
		session: session,
	}

	peer.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver, meta *relay.TrackMeta) {
		if recv, pub := r.AddReceiver(receiver, track, meta.TrackID, meta.StreamID); pub {
			recv.SetTrackMeta(meta.TrackID, meta.StreamID)
			session.Publish(r, recv)
			rp.mu.Lock()
			rp.tracks = append(rp.tracks, PublisherTrack{track, recv, true})
			for _, lrp := range rp.relayPeers {
				if err := rp.createRelayTrack(track, recv, lrp); err != nil {
					slog.Error("create relay track failed", "error", err, "track_id", track.ID(), "stream_id", track.StreamID())
				}
			}
			rp.mu.Unlock()
		} else {
			rp.mu.Lock()
			rp.tracks = append(rp.tracks, PublisherTrack{track, recv, false})
			rp.mu.Unlock()
		}
	})

	return rp
}

func (r *RelayPeer) GetRouter() Router {
	return r.router
}

func (r *RelayPeer) ID() string {
	return r.peer.ID()
}

func (r *RelayPeer) Relay(signalFn func(meta relay.PeerMeta, signal []byte) ([]byte, error)) (*relay.Peer, error) {
	rp, err := relay.NewPeer(
		relay.PeerMeta{
			PeerID:    r.peer.ID(),
			SessionID: r.session.ID(),
		},
		&relay.PeerConfig{
			SettingEngine: r.config.Setting,
			ICEServers:    r.config.Configuration.ICEServers,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("relay: %w", err)
	}

	rp.OnReady(func() {
		r.mu.Lock()
		for _, tp := range r.tracks {
			if !tp.clientRelay {
				// simulcast will just relay client track for now
				continue
			}
			if err = r.createRelayTrack(tp.Track, tp.Receiver, rp); err != nil {
				slog.Error("create relay track on ready failed", "error", err, "track_id", tp.Track.ID(), "stream_id", tp.Track.StreamID())
			}
		}
		r.relayPeers = append(r.relayPeers, rp)
		r.mu.Unlock()
		go r.relayReports(rp)
	})

	rp.OnDataChannel(func(channel *webrtc.DataChannel) {
		r.mu.Lock()
		r.dataChannels = append(r.dataChannels, channel)
		r.mu.Unlock()
		r.session.AddDatachannel("", channel)
	})

	if err = rp.Offer(signalFn); err != nil {
		return nil, fmt.Errorf("relay: %w", err)
	}

	return rp, nil
}

func (r *RelayPeer) DataChannel(label string) *webrtc.DataChannel {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, dc := range r.dataChannels {
		if dc.Label() == label {
			return dc
		}
	}

	return nil
}

func (r *RelayPeer) createRelayTrack(track *webrtc.TrackRemote, receiver Receiver, rp *relay.Peer) error {
	codec := track.Codec()
	downTrack, err := NewDownTrack(
		webrtc.RTPCodecCapability{
			MimeType:    codec.MimeType,
			ClockRate:   codec.ClockRate,
			Channels:    codec.Channels,
			SDPFmtpLine: codec.SDPFmtpLine,
			RTCPFeedback: []webrtc.RTCPFeedback{
				{Type: "nack", Parameter: ""},
				{Type: "nack", Parameter: "pli"},
			},
		},
		receiver,
		r.config.BufferFactory,
		r.ID(),
		r.config.RouterConfig.MaxPacketTrack,
	)
	if err != nil {
		return err
	}

	sdr, err := rp.AddTrack(receiver.(*WebRTCReceiver).receiver, track, downTrack)
	if err != nil {
		return fmt.Errorf("relay: %w", err)
	}

	r.config.BufferFactory.GetOrNew(packetio.RTCPBufferPacket,
		uint32(sdr.GetParameters().Encodings[0].SSRC)).(*buffer.RTCPReader).OnPacket(func(bytes []byte) {
		packets, err := rtcp.Unmarshal(bytes)
		if err != nil {
			return
		}
		var rtcpPackets []rtcp.Packet
		for _, packet := range packets {
			switch packetType := packet.(type) {
			case *rtcp.PictureLossIndication:
				rtcpPackets = append(rtcpPackets, &rtcp.PictureLossIndication{
					SenderSSRC: packetType.MediaSSRC,
					MediaSSRC:  uint32(track.SSRC()),
				})
			}
		}

		if len(rtcpPackets) > 0 {
			if err := r.peer.WriteRTCP(rtcpPackets); err != nil {
				slog.Error("relay write RTCP failed", "error", err)
			}
		}
	})

	downTrack.OnCloseHandler(func() {
		if err = sdr.Stop(); err != nil {
			slog.Error("stop sender failed", "error", err)
		}
	})

	receiver.AddDownTrack(downTrack, true)

	return nil
}

func (r *RelayPeer) relayReports(rp *relay.Peer) {
	for {
		time.Sleep(5 * time.Second)

		var packets []rtcp.Packet
		for _, t := range rp.LocalTracks() {
			if dt, ok := t.(*downTrack); ok {
				if !dt.Bound() {
					continue
				}
				if sr := dt.CreateSenderReport(); sr != nil {
					packets = append(packets, sr)
				}
			}
		}

		if len(packets) == 0 {
			continue
		}

		if err := rp.WriteRTCP(packets); err != nil {
			if err == io.EOF || err == io.ErrClosedPipe {
				return
			}
		}
	}
}
