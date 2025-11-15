package sfu

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/bep/debounce"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

// subscriberはDownTrackから受信したメディアをクライアントに送信するための抽象化された構造体です。
// subscriberはクライアントと1対1の関係にあります。

type Subscriber interface {
	GetUserID() string
	GetPeerConnection() *webrtc.PeerConnection
	AddDatachannel(peer Peer, dc *Datachannel) error
	DataChannel(label string) *webrtc.DataChannel
	OnNegotiationNeeded(f func())
	CreateOffer() (webrtc.SessionDescription, error)
	OnICECandidate(f func(c *webrtc.ICECandidate))
	AddICECandidate(candidate webrtc.ICECandidateInit) error
	AddDownTrack(streamID string, downTrack DownTrack)
	RemoveDownTrack(streamID string, downTrack DownTrack)
	AddDataChannel(label string) (*webrtc.DataChannel, error)
	SetRemoteDescription(desc webrtc.SessionDescription) error
	RegisterDatachannel(label string, dc *webrtc.DataChannel)
	GetDatachannel(label string) *webrtc.DataChannel
	DownTracks() []DownTrack
	GetDownTracks(streamID string) []DownTrack
	Negotiate()
	Close() error
	IsAutoSubscribe() bool
	GetMediaEngine() *webrtc.MediaEngine
	SendStreamDownTracksReports(streamID string)
}

type subscriber struct {
	sync.RWMutex

	userID      string
	mediaEngine *webrtc.MediaEngine
	pc          *webrtc.PeerConnection

	tracks     map[string][]DownTrack
	channels   map[string]*webrtc.DataChannel
	candidates []webrtc.ICECandidateInit

	negotiate func()
	closeOnce sync.Once

	isAutoSubscribe bool
}

func NewSubscriber(isAutoSubscribe bool) *subscriber {
	return &subscriber{
		isAutoSubscribe: isAutoSubscribe,
	}
}

func (s *subscriber) GetPeerConnection() *webrtc.PeerConnection {
	return s.pc
}

func (s *subscriber) GetUserID() string {
	return s.userID
}

func (s *subscriber) AddDatachannel(peer Peer, dc *Datachannel) error {
	ndc, err := s.pc.CreateDataChannel(dc.Label, &webrtc.DataChannelInit{})
	if err != nil {
		return err
	}

	middlewares := newDataChannelChain(dc.middlewares)
	p := middlewares.Process(ProcessFunc(func(ctx context.Context, args ProcessArgs) {
		if dc.onMessage != nil {
			dc.onMessage(ctx, args)
		}
	}))

	ndc.OnMessage(func(msg webrtc.DataChannelMessage) {
		p.Process(context.Background(), ProcessArgs{
			Peer:        peer,
			Message:     msg,
			DataChannel: ndc,
		})
	})

	s.channels[dc.Label] = ndc

	return nil
}

func (s *subscriber) DataChannel(label string) *webrtc.DataChannel {
	s.RLock()
	defer s.RUnlock()

	return s.channels[label]

}

func (s *subscriber) OnNegotiationNeeded(f func()) {
	debounced := debounce.New(250 * time.Millisecond)
	s.negotiate = func() {
		debounced(f)
	}
}

func (s *subscriber) CreateOffer() (webrtc.SessionDescription, error) {
	offer, err := s.pc.CreateOffer(nil)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}

	err = s.pc.SetLocalDescription(offer)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}

	return offer, nil
}

// OnICECandidate handler
func (s *subscriber) OnICECandidate(f func(c *webrtc.ICECandidate)) {
	s.pc.OnICECandidate(f)
}

// AddICECandidate to peer connection
func (s *subscriber) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	if s.pc.RemoteDescription() != nil {
		return s.pc.AddICECandidate(candidate)
	}

	s.candidates = append(s.candidates, candidate)

	return nil
}

func (s *subscriber) AddDownTrack(streamID string, downTrack DownTrack) {
	s.Lock()
	defer s.Unlock()

	if dt, ok := s.tracks[streamID]; ok {
		dt = append(dt, downTrack)
		s.tracks[streamID] = dt
	} else {
		s.tracks[streamID] = []DownTrack{downTrack}
	}
}

func (s *subscriber) RemoveDownTrack(streamID string, downTrack DownTrack) {
	s.Lock()
	defer s.Unlock()

	if dts, ok := s.tracks[streamID]; ok {
		idx := -1
		for i, dt := range dts {
			if dt == downTrack {
				idx = i
				break
			}
		}
		if idx >= 0 {
			dts[idx] = dts[len(dts)-1]
			dts[len(dts)-1] = nil
			dts = dts[:len(dts)-1]
			s.tracks[streamID] = dts
		}
	}
}

func (s *subscriber) AddDataChannel(label string) (*webrtc.DataChannel, error) {
	s.Lock()
	defer s.Unlock()

	if s.channels[label] != nil {
		return s.channels[label], nil
	}

	dc, err := s.pc.CreateDataChannel(label, &webrtc.DataChannelInit{})
	if err != nil {
		return nil, errCreatingDataChannel
	}

	s.channels[label] = dc

	return dc, nil
}

// SetRemoteDescription sets the SessionDescription of the remote peer
func (s *subscriber) SetRemoteDescription(desc webrtc.SessionDescription) error {
	if err := s.pc.SetRemoteDescription(desc); err != nil {
		return err
	}

	for _, c := range s.candidates {
		if err := s.pc.AddICECandidate(c); err != nil {
			// TODO log
		}
	}
	s.candidates = nil

	return nil
}

func (s *subscriber) RegisterDatachannel(label string, dc *webrtc.DataChannel) {
	s.Lock()
	defer s.Unlock()

	s.channels[label] = dc
}

func (s *subscriber) GetDatachannel(label string) *webrtc.DataChannel {
	return s.DataChannel(label)
}

func (s *subscriber) DownTracks() []DownTrack {
	s.RLock()
	defer s.RUnlock()

	var downTracks []DownTrack

	for _, tracks := range s.tracks {
		downTracks = append(downTracks, tracks...)
	}

	return downTracks
}

func (s *subscriber) GetDownTracks(streamID string) []DownTrack {
	s.RLock()
	defer s.RUnlock()

	return s.tracks[streamID]
}

func (s *subscriber) Close() error {
	return s.pc.Close()
}

func (s *subscriber) downTracksReports() {
	for {
		time.Sleep(5 * time.Second)

		if s.pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
			return
		}

		sourceDescriptions, r := s.buildTracksReports()

		i := 0
		j := 0
		for i < len(sourceDescriptions) {
			i = (j + 1) * 15

			if i >= len(sourceDescriptions) {
				i = len(sourceDescriptions)
			}

			nsd := sourceDescriptions[j*15 : i]

			r = append(r, &rtcp.SourceDescription{Chunks: nsd})

			j++

			if err := s.pc.WriteRTCP(r); err != nil {
				if err == io.EOF || err == io.ErrClosedPipe {
					return
				}
			}

			r = r[:0]
		}
	}
}

func (s *subscriber) buildTracksReports() ([]rtcp.SourceDescriptionChunk, []rtcp.Packet) {
	s.RLock()
	defer s.RUnlock()

	var sd []rtcp.SourceDescriptionChunk
	var r []rtcp.Packet

	for _, dts := range s.tracks {
		for _, dt := range dts {
			if !dt.Bound() {
				continue
			}

			if sr := dt.CreateSenderReport(); sr != nil {
				r = append(r, sr)
			}

			sd = append(sd, dt.CreateSourceDescriptionChunks()...)
		}
	}

	return sd, r
}

func (s *subscriber) sendStreamDownTracksReports(streamID string) {
	var r []rtcp.Packet
	var sd []rtcp.SourceDescriptionChunk

	s.RLock()
	dts := s.tracks[streamID]
	for _, dt := range dts {
		if !dt.Bound() {
			continue
		}
		sd = append(sd, dt.CreateSourceDescriptionChunks()...)
	}
	s.RUnlock()
	if len(sd) == 0 {
		return
	}
	r = append(r, &rtcp.SourceDescription{Chunks: sd})
	go func() {
		r := r
		i := 0
		for {
			if err := s.pc.WriteRTCP(r); err != nil {
				// TODO log
			}
			if i > 5 {
				return
			}
			i++
			time.Sleep(20 * time.Millisecond)
		}
	}()
}

func (s *subscriber) Negotiate() {
	if s.negotiate == nil {
		return
	}

	s.negotiate()
}

func (s *subscriber) SendStreamDownTracksReports(streamID string) {
}

func (s *subscriber) IsAutoSubscribe() bool {
	return s.isAutoSubscribe
}

func (s *subscriber) GetMediaEngine() *webrtc.MediaEngine {
	return s.mediaEngine
}
