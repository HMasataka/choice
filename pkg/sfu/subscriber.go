package sfu

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/HMasataka/choice/pkg/retry"
	"github.com/bep/debounce"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

const (
	APIChannelLabel = "choice"
	// sendRTCPRetryAttempts は、RTCP送信のリトライ回数の既定値です。
	sendRTCPRetryAttempts = 6
	// sdesChunkBatchSize は、SDES(Chunk) を一度に送信する際の最大数です。
	sdesChunkBatchSize = 15
	// downTrackReportInterval は、DownTrack レポート送信の間隔です。
	downTrackReportInterval = 5 * time.Second
)

// subscriberはDownTrackから受信したメディアをクライアントに送信するための抽象化された構造体です。
// subscriberはクライアントと1対1の関係にあります。
//
//go:generate mockgen -source subscriber.go -destination mock/subscriber.go
type Subscriber interface {
	GetUserID() string
	GetPeerConnection() *webrtc.PeerConnection
	AddDatachannel(ctx context.Context, peer Peer, dc *Datachannel) error
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
	mu sync.RWMutex

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

func NewSubscriber(userID string, isAutoSubscribe bool, cfg *WebRTCTransportConfig) *subscriber {
	s := &subscriber{
		userID:          userID,
		isAutoSubscribe: isAutoSubscribe,
		tracks:          make(map[string][]DownTrack),
		channels:        make(map[string]*webrtc.DataChannel),
	}

	me, err := getSubscriberMediaEngine()
	if err != nil {
		slog.Error("failed to create media engine for subscriber", "error", err)
		os.Exit(1)
	}
	s.mediaEngine = me

	api := webrtc.NewAPI(webrtc.WithMediaEngine(me), webrtc.WithSettingEngine(cfg.Setting))
	pc, err := api.NewPeerConnection(cfg.Configuration)
	if err != nil {
		slog.Error("failed to create peer connection for subscriber", "error", err)
		os.Exit(1)
	}
	pc.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		switch connectionState {
		case webrtc.ICEConnectionStateFailed:
			fallthrough
		case webrtc.ICEConnectionStateClosed:
			if err := s.Close(); err != nil {
				slog.Error("failed to close subscriber peer connection", "error", err)
			}
		}
	})
	s.pc = pc

	go s.downTracksReports()

	return s
}

func (s *subscriber) GetPeerConnection() *webrtc.PeerConnection {
	return s.pc
}

func (s *subscriber) GetUserID() string {
	return s.userID
}

func (s *subscriber) AddDatachannel(ctx context.Context, peer Peer, dc *Datachannel) error {
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
	s.mu.RLock()
	defer s.mu.RUnlock()

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
	s.mu.Lock()
	defer s.mu.Unlock()

	if dt, ok := s.tracks[streamID]; ok {
		dt = append(dt, downTrack)
		s.tracks[streamID] = dt
	} else {
		s.tracks[streamID] = []DownTrack{downTrack}
	}
}

func (s *subscriber) RemoveDownTrack(streamID string, downTrack DownTrack) {
	s.mu.Lock()
	defer s.mu.Unlock()

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
	s.mu.Lock()
	defer s.mu.Unlock()

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

// SetRemoteDescription は、リモートピアの SessionDescription を設定します。
func (s *subscriber) SetRemoteDescription(desc webrtc.SessionDescription) error {
	if err := s.pc.SetRemoteDescription(desc); err != nil {
		return err
	}

	for _, c := range s.candidates {
		if err := s.pc.AddICECandidate(c); err != nil {
			slog.Error("failed to add ICE candidate (deferred)", "error", err)
		}
	}
	s.candidates = nil

	return nil
}

func (s *subscriber) RegisterDatachannel(label string, dc *webrtc.DataChannel) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.channels[label] = dc
}

func (s *subscriber) GetDatachannel(label string) *webrtc.DataChannel {
	return s.DataChannel(label)
}

func (s *subscriber) DownTracks() []DownTrack {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var downTracks []DownTrack

	for _, tracks := range s.tracks {
		downTracks = append(downTracks, tracks...)
	}

	return downTracks
}

func (s *subscriber) GetDownTracks(streamID string) []DownTrack {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.tracks[streamID]
}

func (s *subscriber) Close() error {
	var err error

	s.closeOnce.Do(func() {
		err = s.pc.Close()
	})

	return err
}

func (s *subscriber) downTracksReports() {
	ticker := time.NewTicker(downTrackReportInterval)
	defer ticker.Stop()

	for {
		if s.pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
			return
		}

		<-ticker.C

		sd, reports := s.buildTracksReports()
		if len(sd) == 0 && len(reports) == 0 {
			continue
		}

		s.sendBatchedReports(reports, sd, sdesChunkBatchSize)
	}
}

// buildTracksReports は、全 DownTrack から SDES と SenderReport を収集して返します。
func (s *subscriber) buildTracksReports() ([]rtcp.SourceDescriptionChunk, []rtcp.Packet) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var (
		sd []rtcp.SourceDescriptionChunk
		r  []rtcp.Packet
	)

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

// sendBatchedReports は、最初の送信で SenderReport を含め、その後は SDES を batchSize 件ずつ送信します。
// EOF/ClosedPipe を検知した場合は早期に終了します。
func (s *subscriber) sendBatchedReports(reports []rtcp.Packet, sd []rtcp.SourceDescriptionChunk, batchSize int) {
	if batchSize <= 0 {
		batchSize = sdesChunkBatchSize
	}

	sentReports := false
	for off := 0; off < len(sd); off += batchSize {
		end := off + batchSize
		end = min(end, len(sd))
		nsd := sd[off:end]

		var packets []rtcp.Packet
		if !sentReports && len(reports) > 0 {
			packets = append(packets, reports...)
			sentReports = true
		}
		packets = append(packets, &rtcp.SourceDescription{Chunks: nsd})

		if err := s.pc.WriteRTCP(packets); err != nil {
			if !retry.ShouldRetry(err) {
				return
			}
		}
	}
}

func (s *subscriber) SendStreamDownTracksReports(streamID string) {
	sd := s.buildStreamSourceDescriptions(streamID)
	if len(sd) == 0 {
		return
	}

	packets := []rtcp.Packet{&rtcp.SourceDescription{Chunks: sd}}
	go s.sendRTCPWithRetry(streamID, packets, sendRTCPRetryAttempts, 20*time.Millisecond)
}

// buildStreamSourceDescriptions は、指定されたストリームに属するバインド済みの DownTrack から SDES (SourceDescription) チャンクを生成して返します。
func (s *subscriber) buildStreamSourceDescriptions(streamID string) []rtcp.SourceDescriptionChunk {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var sd []rtcp.SourceDescriptionChunk
	for _, dt := range s.tracks[streamID] {
		if !dt.Bound() {
			continue
		}
		sd = append(sd, dt.CreateSourceDescriptionChunks()...)
	}
	return sd
}

// rtcpRetryExecutor はRTCPリトライのExecutor実装
type rtcpRetryExecutor struct {
	sub      *subscriber
	streamID string
	packets  []rtcp.Packet
	cfg      retry.Config
}

// DetermineAction は接続状態に基づいて次のアクションを決定する
func (e *rtcpRetryExecutor) DetermineAction() retry.Action {
	switch e.sub.pc.ConnectionState() {
	case webrtc.PeerConnectionStateClosed, webrtc.PeerConnectionStateFailed:
		return retry.Abort
	case webrtc.PeerConnectionStateConnected:
		return retry.Execute
	default:
		return retry.Wait
	}
}

// Execute はRTCPパケットの送信を試みる
func (e *rtcpRetryExecutor) Execute(attempt int) bool {
	err := e.sub.pc.WriteRTCP(e.packets)
	if !retry.ShouldRetry(err) {
		return true
	}

	if attempt == e.cfg.Attempts-1 && err != nil {
		slog.Error("failed to write RTCP for stream downtracks", "error", err, "stream_id", e.streamID)
	}
	time.Sleep(retry.Backoff(attempt, e.cfg.BaseInterval, e.cfg.MaxBackoff))
	return false
}

// sendRTCPWithRetry は、一定間隔で指定回数だけ RTCP パケットを書き込みます。
func (s *subscriber) sendRTCPWithRetry(streamID string, packets []rtcp.Packet, attempts int, interval time.Duration) {
	if s == nil || s.pc == nil || len(packets) == 0 {
		return
	}

	cfg := retry.DefaultConfig()
	if attempts > 0 {
		cfg.Attempts = attempts
	}
	if interval > 0 {
		cfg.BaseInterval = interval
	}

	executor := &rtcpRetryExecutor{
		sub:      s,
		streamID: streamID,
		packets:  packets,
		cfg:      cfg,
	}
	retry.Run(cfg, executor)
}

func (s *subscriber) Negotiate() {
	if s.negotiate == nil {
		slog.Debug("Negotiate called but negotiate is nil", "user_id", s.userID)
		return
	}

	slog.Debug("Negotiate called", "user_id", s.userID)
	s.negotiate()
}

func (s *subscriber) IsAutoSubscribe() bool {
	return s.isAutoSubscribe
}

func (s *subscriber) GetMediaEngine() *webrtc.MediaEngine {
	return s.mediaEngine
}
