package sfu

import (
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/HMasataka/choice/pkg/twcc"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

// RouterはReceiverから受信したメディアを適切なDowntrackにルーティングするための抽象化された構造体です。
type Router interface {
	UserID() string
	AddReceiver(receiver *webrtc.RTPReceiver, track *webrtc.TrackRemote, trackID, streamID string) (Receiver, bool)
	AddDownTracks(s Subscriber, r Receiver) error
	SetRTCPWriter(func([]rtcp.Packet) error)
	AddDownTrack(s Subscriber, r Receiver) (DownTrack, error)
	Stop()
	GetReceiver() map[string]Receiver
	OnAddReceiverTrack(f func(receiver Receiver))
	OnDelReceiverTrack(f func(receiver Receiver))
}

var _ Router = (*router)(nil)

type router struct {
	mu sync.RWMutex

	twcc *twcc.Responder

	userID    string
	rtcpCh    chan []rtcp.Packet
	stopCh    chan struct{}
	receivers map[string]Receiver

	bufferFactory *buffer.Factory

	config RouterConfig

	session Session

	onAddTrack atomic.Value // func(Receiver)
	onDelTrack atomic.Value // func(Receiver)

	writeRTCP func([]rtcp.Packet) error
}

func NewRouter(userID string, session Session, cfg *WebRTCTransportConfig) *router {
	ch := make(chan []rtcp.Packet, 10)

	return &router{
		userID:        userID,
		rtcpCh:        ch,
		stopCh:        make(chan struct{}),
		receivers:     make(map[string]Receiver),
		bufferFactory: cfg.BufferFactory,
		config:        cfg.RouterConfig,
		session:       session,
	}
}

type RouterConfig struct {
	MaxBandwidth        uint64          `toml:"maxbandwidth"`
	MaxPacketTrack      int             `toml:"maxpackettrack"`
	AudioLevelInterval  int             `toml:"audiolevelinterval"`
	AudioLevelThreshold uint8           `toml:"audiolevelthreshold"`
	AudioLevelFilter    int             `toml:"audiolevelfilter"`
	Simulcast           SimulcastConfig `toml:"simulcast"`
	AllowSelfSubscribe  bool            `toml:"selfsubscribe"`
}

func (r *router) GetReceiver() map[string]Receiver {
	return r.receivers
}

func (r *router) OnAddReceiverTrack(f func(receiver Receiver)) {
	r.onAddTrack.Store(f)
}

func (r *router) OnDelReceiverTrack(f func(receiver Receiver)) {
	r.onDelTrack.Store(f)
}

func (r *router) UserID() string {
	return r.userID
}

func (r *router) Stop() {
	r.stopCh <- struct{}{}
}

func (r *router) AddReceiver(receiver *webrtc.RTPReceiver, track *webrtc.TrackRemote, trackID, streamID string) (Receiver, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	buff := r.setupBuffer(track)

	switch track.Kind() {
	case webrtc.RTPCodecTypeAudio:
		buff.OnAudioLevel(func(level uint8) {
			r.session.AudioObserver().observe(streamID, level)
		})
		r.session.AudioObserver().addStream(streamID)

	case webrtc.RTPCodecTypeVideo:
		if r.twcc == nil {
			r.twcc = twcc.NewTransportWideCCResponder(uint32(track.SSRC()))
			r.twcc.OnFeedback(func(p rtcp.RawPacket) {
				r.rtcpCh <- []rtcp.Packet{&p}
			})
		}
		buff.OnTransportWideCC(func(sn uint16, timeNS int64, marker bool) {
			r.twcc.Push(sn, timeNS, marker)
		})
	}

	recv, publish := r.getReceiver(receiver, track, trackID)
	recv.AddUpTrack(track, buff, r.config.Simulcast.BestQualityFirst)

	buff.Bind(receiver.GetParameters(), buffer.Options{
		MaxBitRate: r.config.MaxBandwidth * 1000,
	})

	return recv, publish
}

func (r *router) setupBuffer(track *webrtc.TrackRemote) *buffer.Buffer {
	buff, rtcpReader := r.bufferFactory.GetBufferPair(uint32(track.SSRC()))

	buff.OnFeedback(func(fb []rtcp.Packet) {
		r.rtcpCh <- fb
	})

	rtcpReader.OnPacket(func(bytes []byte) {
		packets, err := rtcp.Unmarshal(bytes)
		if err != nil {
			return
		}

		for _, packet := range packets {
			switch packetType := packet.(type) {
			case *rtcp.SourceDescription:
			case *rtcp.SenderReport:
				buff.SetSenderReportData(packetType.RTPTime, packetType.NTPTime)
			}
		}
	})

	return buff
}

func (r *router) getReceiver(receiver *webrtc.RTPReceiver, track *webrtc.TrackRemote, trackID string) (recv Receiver, publish bool) {
	recv, ok := r.receivers[trackID]
	if ok {
		return recv, false
	}

	recv = NewWebRTCReceiver(receiver, track, r.userID)

	r.receivers[trackID] = recv

	recv.SetRTCPCh(r.rtcpCh)

	recv.OnCloseHandler(func() {
		if recv.Kind() == webrtc.RTPCodecTypeAudio {
			r.session.AudioObserver().removeStream(track.StreamID())
		}

		r.deleteReceiver(trackID, uint32(track.SSRC()))
	})

	if handler, ok := r.onAddTrack.Load().(func(Receiver)); ok && handler != nil {
		handler(recv)
	}

	return recv, true
}

func (r *router) deleteReceiver(track string, ssrc uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if handler, ok := r.onDelTrack.Load().(func(Receiver)); ok && handler != nil {
		handler(r.receivers[track])
	}

	delete(r.receivers, track)
}

func (r *router) AddDownTracks(s Subscriber, receiver Receiver) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !s.IsAutoSubscribe() {
		return nil
	}

	if receiver != nil {
		if _, err := r.AddDownTrack(s, receiver); err != nil {
			return err
		}

		s.Negotiate()

		return nil
	}

	if len(r.receivers) > 0 {
		for _, rcv := range r.receivers {
			if _, err := r.AddDownTrack(s, rcv); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *router) SetRTCPWriter(fn func(packet []rtcp.Packet) error) {
	r.writeRTCP = fn
	go r.sendRTCP()
}

func (r *router) sendRTCP() {
	for {
		select {
		case packets := <-r.rtcpCh:
			if err := r.writeRTCP(packets); err != nil {
				slog.Error("failed to write RTCP packets", "error", err, "packets", len(packets), "user_id", r.userID)
			}
		case <-r.stopCh:
			return
		}
	}
}

func (r *router) AddDownTrack(subscriber Subscriber, receiver Receiver) (DownTrack, error) {
	for _, dt := range subscriber.GetDownTracks(receiver.StreamID()) {
		if dt.ID() == receiver.TrackID() {
			return dt, nil
		}
	}

	codec := receiver.Codec()

	// MediaEngineには既にgetSubscriberMediaEngine()で必要なCodecが登録されている
	// RegisterCodecは既に登録されているCodecに対してエラーを返す可能性があるため
	// エラーが発生しても無視する（既に登録されている場合は問題ない）
	if err := subscriber.GetMediaEngine().RegisterCodec(codec, receiver.Kind()); err != nil {
		// Codecが既に登録されている場合はエラーを無視
		// それ以外のエラーの場合はログに記録
		slog.Debug("codec registration skipped (may already be registered)", "codec", codec.MimeType, "error", err)
	}

	downTrack, err := r.newDownTrack(codec, subscriber, receiver)
	if err != nil {
		return nil, err
	}

	subscriber.AddDownTrack(receiver.StreamID(), downTrack)
	receiver.AddDownTrack(downTrack, r.config.Simulcast.BestQualityFirst)

	return downTrack, nil
}

func (r *router) newDownTrack(codec webrtc.RTPCodecParameters, subscriber Subscriber, receiver Receiver) (DownTrack, error) {
	downTrack, err := NewDownTrack(
		webrtc.RTPCodecCapability{
			MimeType:    codec.MimeType,
			ClockRate:   codec.ClockRate,
			Channels:    codec.Channels,
			SDPFmtpLine: codec.SDPFmtpLine,
			RTCPFeedback: []webrtc.RTCPFeedback{
				{Type: "goog-remb", Parameter: ""},
				{Type: "nack", Parameter: ""},
				{Type: "nack", Parameter: "pli"},
			},
		},
		receiver,
		r.bufferFactory,
		subscriber.GetUserID(),
		r.config.MaxPacketTrack,
	)
	if err != nil {
		return nil, err
	}

	pc := subscriber.GetPeerConnection()

	if downTrack.transceiver, err = pc.AddTransceiverFromTrack(downTrack, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendonly,
	}); err != nil {
		return nil, err
	}

	downTrack.OnCloseHandler(func() {
		if pc.ConnectionState() != webrtc.PeerConnectionStateClosed {
			if err := pc.RemoveTrack(downTrack.transceiver.Sender()); err != nil {
				if err == webrtc.ErrConnectionClosed {
					return
				}
			} else {
				subscriber.RemoveDownTrack(receiver.StreamID(), downTrack)
				subscriber.Negotiate()
			}
		}
	})

	downTrack.OnBind(func() {
		go subscriber.SendStreamDownTracksReports(receiver.StreamID())
	})

	return downTrack, nil
}
