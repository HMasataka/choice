package server

import (
	"context"
	"fmt"

	pion "github.com/pion/webrtc/v4"

	"github.com/HMasataka/choice/internal/media"
	"github.com/HMasataka/choice/internal/signaling"
	"github.com/HMasataka/choice/internal/signaling/protocol"
	"github.com/HMasataka/choice/internal/webrtc"
	"github.com/HMasataka/choice/pkg/config"
	"github.com/HMasataka/choice/pkg/logger"
)

// WebRTCComponents holds all initialized WebRTC-related components.
type WebRTCComponents struct {
	Service       *webrtc.Service
	EventsBridge  *signaling.WebRTCEventsBridge
	MediaRouter   media.MediaRouter
	Handler       *signaling.Handler
	Handlers      *signaling.Handlers
	Dispatcher    *signaling.Dispatcher
}

// InitializeWebRTC initializes all WebRTC and signaling components.
func InitializeWebRTC(cfg *config.Config, log *logger.Logger) (*WebRTCComponents, error) {
	// 1. Create MediaEngine with codec configuration
	mediaEngine, err := createMediaEngine(cfg.Media)
	if err != nil {
		return nil, fmt.Errorf("failed to create media engine: %w", err)
	}

	// 2. Create PeerConfig from config
	peerConfig := createPeerConfig(cfg.WebRTC)

	// 3. Create Notifier for signaling notifications
	notifier := signaling.NewNotifier()

	// 4. Create MediaRouter (Phase 1.7.2)
	mediaRouter := media.NewMediaRouter()

	// 5. Create WebRTCEventsBridge
	eventsBridge := signaling.NewWebRTCEventsBridge(notifier, mediaRouter, *log)

	// 6. Create WebRTC Service
	rtcService := webrtc.NewService(peerConfig, mediaEngine, eventsBridge)

	// 7. Create MediaService (Task 1.7.3)
	// Wrap rtcService to satisfy media.WebRTCService interface
	rtcServiceAdapter := &webrtcServiceAdapter{service: rtcService}
	mediaServiceImpl := media.NewService(mediaRouter, rtcServiceAdapter)
	// Wrap mediaService to satisfy signaling.MediaService interface
	mediaService := &mediaServiceAdapter{service: mediaServiceImpl}

	// 8. Create Dispatcher
	dispatcher := signaling.NewDispatcher(signaling.DefaultDispatcherConfig())

	// 9. Create Handlers configuration
	handlersConfig := createHandlersConfig(cfg.WebRTC)

	// 10. Create Handlers (method handlers)
	// Note: RoomService is nil for Phase 1 (will be implemented in Phase 2)
	handlers := signaling.NewHandlers(
		dispatcher,
		nil,          // roomService (Phase 2)
		rtcService,   // rtcService
		mediaService, // mediaService (Task 1.7.3)
		eventsBridge, // eventsBridge for ICE candidate routing
		handlersConfig,
	)

	// 11. Create DispatcherConnectionHandler and set up callbacks
	connHandler := signaling.NewDispatcherConnectionHandler(dispatcher)

	// Set OnDisconnect callback to clean up participant state
	connHandler.SetOnDisconnect(func(conn *signaling.Connection, err error) {
		handlers.OnConnectionClosed(conn)
		// Unregister participant from EventsBridge
		if participantID, ok := conn.GetData("participant_id"); ok {
			if pid, ok := participantID.(string); ok {
				eventsBridge.UnregisterParticipant(pid)
			}
		}
	})

	// 12. Create WebSocket Handler
	wsHandlerConfig := createWebSocketHandlerConfig(cfg.Server.WebSocket)
	wsHandler := signaling.NewHandler(wsHandlerConfig, connHandler)

	return &WebRTCComponents{
		Service:      rtcService,
		EventsBridge: eventsBridge,
		MediaRouter:  mediaRouter,
		Handler:      wsHandler,
		Handlers:     handlers,
		Dispatcher:   dispatcher,
	}, nil
}

// createMediaEngine creates and configures a MediaEngine with codecs.
func createMediaEngine(cfg config.MediaConfig) (*pion.MediaEngine, error) {
	me := &pion.MediaEngine{}

	// Register video codecs
	// VP8 (mandatory)
	if err := me.RegisterCodec(pion.RTPCodecParameters{
		RTPCodecCapability: pion.RTPCodecCapability{
			MimeType:     pion.MimeTypeVP8,
			ClockRate:    90000,
			Channels:     0,
			SDPFmtpLine:  "",
			RTCPFeedback: createRTCPFeedback(),
		},
		PayloadType: 96,
	}, pion.RTPCodecTypeVideo); err != nil {
		return nil, fmt.Errorf("failed to register VP8: %w", err)
	}

	// VP9 (recommended)
	if err := me.RegisterCodec(pion.RTPCodecParameters{
		RTPCodecCapability: pion.RTPCodecCapability{
			MimeType:     pion.MimeTypeVP9,
			ClockRate:    90000,
			Channels:     0,
			SDPFmtpLine:  "",
			RTCPFeedback: createRTCPFeedback(),
		},
		PayloadType: 98,
	}, pion.RTPCodecTypeVideo); err != nil {
		return nil, fmt.Errorf("failed to register VP9: %w", err)
	}

	// H.264 (compatibility)
	if err := me.RegisterCodec(pion.RTPCodecParameters{
		RTPCodecCapability: pion.RTPCodecCapability{
			MimeType:     pion.MimeTypeH264,
			ClockRate:    90000,
			Channels:     0,
			SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
			RTCPFeedback: createRTCPFeedback(),
		},
		PayloadType: 102,
	}, pion.RTPCodecTypeVideo); err != nil {
		return nil, fmt.Errorf("failed to register H.264: %w", err)
	}

	// Register audio codecs
	// Opus (mandatory)
	if err := me.RegisterCodec(pion.RTPCodecParameters{
		RTPCodecCapability: pion.RTPCodecCapability{
			MimeType:     pion.MimeTypeOpus,
			ClockRate:    48000,
			Channels:     2,
			SDPFmtpLine:  "minptime=10;useinbandfec=1;stereo=1",
			RTCPFeedback: []pion.RTCPFeedback{},
		},
		PayloadType: 111,
	}, pion.RTPCodecTypeAudio); err != nil {
		return nil, fmt.Errorf("failed to register Opus: %w", err)
	}

	// Register RTP header extensions
	if err := me.RegisterHeaderExtension(pion.RTPHeaderExtensionCapability{
		URI: "urn:ietf:params:rtp-hdrext:sdes:mid",
	}, pion.RTPCodecTypeVideo); err != nil {
		return nil, fmt.Errorf("failed to register MID extension for video: %w", err)
	}

	if err := me.RegisterHeaderExtension(pion.RTPHeaderExtensionCapability{
		URI: "urn:ietf:params:rtp-hdrext:sdes:mid",
	}, pion.RTPCodecTypeAudio); err != nil {
		return nil, fmt.Errorf("failed to register MID extension for audio: %w", err)
	}

	if err := me.RegisterHeaderExtension(pion.RTPHeaderExtensionCapability{
		URI: "urn:ietf:params:rtp-hdrext:sdes:rtp-stream-id",
	}, pion.RTPCodecTypeVideo); err != nil {
		return nil, fmt.Errorf("failed to register RID extension: %w", err)
	}

	if err := me.RegisterHeaderExtension(pion.RTPHeaderExtensionCapability{
		URI: "http://www.webrtc.org/experiments/rtp-hdrext/abs-send-time",
	}, pion.RTPCodecTypeVideo); err != nil {
		return nil, fmt.Errorf("failed to register abs-send-time extension: %w", err)
	}

	if err := me.RegisterHeaderExtension(pion.RTPHeaderExtensionCapability{
		URI: "http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01",
	}, pion.RTPCodecTypeVideo); err != nil {
		return nil, fmt.Errorf("failed to register transport-cc extension: %w", err)
	}

	return me, nil
}

// createRTCPFeedback creates RTCP feedback configuration for video codecs.
func createRTCPFeedback() []pion.RTCPFeedback {
	return []pion.RTCPFeedback{
		{Type: "nack", Parameter: ""},
		{Type: "nack", Parameter: "pli"},
		{Type: "ccm", Parameter: "fir"},
		{Type: "goog-remb", Parameter: ""},
		{Type: "transport-cc", Parameter: ""},
	}
}

// createPeerConfig creates a PeerConfig from the application config.
func createPeerConfig(cfg config.WebRTCConfig) webrtc.PeerConfig {
	// Convert ICE servers
	var iceServers []pion.ICEServer
	for _, server := range cfg.ICEServer {
		iceServer := pion.ICEServer{
			URLs: server.URLs,
		}
		if server.Username != "" {
			iceServer.Username = server.Username
		}
		if server.Credential != "" {
			iceServer.Credential = server.Credential
		}
		iceServers = append(iceServers, iceServer)
	}

	// Build NAT 1:1 IPs
	var nat1To1IPs []string
	if cfg.PublicIP != "" {
		nat1To1IPs = []string{cfg.PublicIP}
	}

	peerCfg := webrtc.DefaultPeerConfig()
	peerCfg.ICEServers = iceServers
	peerCfg.ICELite = cfg.ICELite
	peerCfg.NAT1To1IPs = nat1To1IPs

	return peerCfg
}

// createHandlersConfig creates a HandlersConfig from the application config.
func createHandlersConfig(cfg config.WebRTCConfig) signaling.HandlersConfig {
	// Convert ICE servers to protocol format
	var iceServers []protocol.IceServer
	for _, server := range cfg.ICEServer {
		iceServer := protocol.IceServer{
			URLs:       server.URLs,
			Username:   server.Username,
			Credential: server.Credential,
		}
		iceServers = append(iceServers, iceServer)
	}

	return signaling.HandlersConfig{
		IceServers: iceServers,
	}
}

// createWebSocketHandlerConfig creates a WebSocket handler config from the application config.
func createWebSocketHandlerConfig(cfg config.WebSocketConfig) signaling.HandlerConfig {
	return signaling.HandlerConfig{
		ReadBufferSize:   cfg.ReadBufferSize,
		WriteBufferSize:  cfg.WriteBufferSize,
		HandshakeTimeout: cfg.HandshakeTimeout,
		PingPeriod:       cfg.PingInterval, // Map PingInterval to PingPeriod
	}
}

// webrtcServiceAdapter adapts webrtc.Service to media.WebRTCService interface.
type webrtcServiceAdapter struct {
	service *webrtc.Service
}

// GetPeer returns the peer for the given participant ID.
// Returns nil if no peer exists.
// Note: *webrtc.Peer implements media.WebRTCPeer interface.
func (a *webrtcServiceAdapter) GetPeer(participantID string) media.WebRTCPeer {
	return a.service.GetPeer(participantID)
}

// mediaServiceAdapter adapts media.Service to signaling.MediaService interface.
type mediaServiceAdapter struct {
	service *media.Service
}

// Publish adapts media.Service.Publish to signaling.MediaService.Publish.
func (a *mediaServiceAdapter) Publish(ctx context.Context, participantID string, kind protocol.TrackKind, simulcast bool, metadata map[string]interface{}, label string) (*signaling.PublishResponse, error) {
	resp, err := a.service.Publish(ctx, participantID, kind, simulcast, metadata, label)
	if err != nil {
		return nil, err
	}
	return &signaling.PublishResponse{
		TrackID: resp.TrackID,
		Mid:     resp.Mid,
	}, nil
}

// Unpublish adapts media.Service.Unpublish to signaling.MediaService.Unpublish.
func (a *mediaServiceAdapter) Unpublish(ctx context.Context, participantID string, trackID string) error {
	return a.service.Unpublish(ctx, participantID, trackID)
}

// Subscribe adapts media.Service.Subscribe to signaling.MediaService.Subscribe.
func (a *mediaServiceAdapter) Subscribe(ctx context.Context, participantID string, publisherID string, trackID string, preferredLayer protocol.SimulcastLayer) (*signaling.SubscribeResponse, error) {
	resp, err := a.service.Subscribe(ctx, participantID, publisherID, trackID, preferredLayer)
	if err != nil {
		return nil, err
	}
	return &signaling.SubscribeResponse{
		SubscriptionID: resp.SubscriptionID,
		TrackID:        resp.TrackID,
		PublisherID:    resp.PublisherID,
	}, nil
}

// Unsubscribe adapts media.Service.Unsubscribe to signaling.MediaService.Unsubscribe.
func (a *mediaServiceAdapter) Unsubscribe(ctx context.Context, participantID string, subscriptionID string) error {
	return a.service.Unsubscribe(ctx, participantID, subscriptionID)
}

// SetPreferredLayer adapts media.Service.SetPreferredLayer to signaling.MediaService.SetPreferredLayer.
func (a *mediaServiceAdapter) SetPreferredLayer(ctx context.Context, participantID string, trackID string, layer protocol.SimulcastLayer) error {
	return a.service.SetPreferredLayer(ctx, participantID, trackID, layer)
}
