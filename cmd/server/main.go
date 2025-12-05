package main

import (
	"os"
	"sync"

	"log/slog"
	"net/http"

	"github.com/HMasataka/choice/payload/handshake"
	"github.com/HMasataka/choice/pkg/sfu"
	"github.com/HMasataka/logging"
	"github.com/gorilla/rpc/v2"
	"github.com/gorilla/rpc/v2/json2"
	"github.com/pelletier/go-toml/v2"
)

func NewSignalingServer(s *sfu.SFU) *SignalingServer {
	return &SignalingServer{
		sfu:   s,
		peers: make(map[string]map[string]sfu.Peer),
	}
}

type SignalingServer struct {
	sfu   *sfu.SFU
	mu    sync.RWMutex
	peers map[string]map[string]sfu.Peer
}

func (h *SignalingServer) Join(r *http.Request, args *handshake.JoinRequest, reply *handshake.JoinResponse) error {
	ctx := r.Context()

	// TODO: JoinConfig の設定
	var joinConfig sfu.JoinConfig

	peer := h.getOrCreatePeer(args.SessionID, args.UserID)
	if err := peer.Join(ctx, args.SessionID, args.UserID, joinConfig); err != nil {
		return err
	}

	pub := peer.Publisher()
	if pub == nil {
		return nil
	}

	answer, err := pub.Answer(args.Offer)
	if err != nil {
		return err
	}

	*reply = handshake.JoinResponse{Answer: &answer}

	if logging.HasLoggingContext(ctx) {
		slog.InfoContext(ctx, "peer joined", slog.String("session_id", args.SessionID), slog.String("user_id", args.UserID))
	}

	return nil
}

func (h *SignalingServer) Offer(r *http.Request, args *handshake.OfferRequest, reply *handshake.OfferResponse) error {
	return nil
}

func (h *SignalingServer) Answer(r *http.Request, args *handshake.AnswerRequest, reply *handshake.AnswerResponse) error {
	peer := h.getPeer(args.SessionID, args.UserID)
	if peer == nil {
		slog.Warn("peer not found for answer", slog.String("session_id", args.SessionID), slog.String("user_id", args.UserID))
		return nil
	}

	if err := peer.Subscriber().SetRemoteDescription(args.Answer); err != nil {
		return err
	}

	return nil
}

func (h *SignalingServer) Candidate(r *http.Request, args *handshake.CandidateRequest, reply *handshake.CandidateResponse) error {
	peer := h.getPeer(args.SessionID, args.UserID)
	if peer == nil {
		slog.Warn("peer not found for candidate", slog.String("session_id", args.SessionID), slog.String("user_id", args.UserID))
		return nil
	}

	switch args.ConnectionType {
	case handshake.ConnectionTypePublisher:
		if err := peer.Publisher().AddICECandidate(args.Candidate); err != nil {
			return err
		}
	case handshake.ConnectionTypeSubscriber:
		if err := peer.Subscriber().AddICECandidate(args.Candidate); err != nil {
			return err
		}
	}

	return nil
}

func main() {
	logger := slog.New(logging.NewHandler(slog.NewJSONHandler(os.Stdout, nil)))
	slog.SetDefault(logger)

	cfg, err := loadConfig()
	if err != nil {
		slog.Error("failed to load config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	slog.Info("config loaded", slog.Any("config", cfg))

	server := rpc.NewServer()
	server.RegisterCodec(json2.NewCodec(), "application/json")

	sfu := sfu.NewSFU(cfg)

	signaling := NewSignalingServer(sfu)
	if err := server.RegisterService(signaling, ""); err != nil {
		slog.Error("failed to register signaling service", slog.String("error", err.Error()))
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("/", server)

	slog.Info("Starting signaling server on :8081")

	if err := http.ListenAndServe(":8081", mux); err != nil {
		slog.Error("failed to start server", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func (h *SignalingServer) getPeer(sessionID, userID string) sfu.Peer {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.peers == nil {
		return nil
	}

	if users, ok := h.peers[sessionID]; ok {
		if p, ok := users[userID]; ok {
			return p
		}
	}

	return nil
}

func (h *SignalingServer) getOrCreatePeer(sessionID, userID string) sfu.Peer {
	if p := h.getPeer(sessionID, userID); p != nil {
		return p
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.peers[sessionID]; !ok {
		h.peers[sessionID] = make(map[string]sfu.Peer)
	}

	p := sfu.NewPeer(h.sfu)
	h.peers[sessionID][userID] = p

	return p
}

func loadConfig() (sfu.Config, error) {
	path := "config.toml"
	b, err := os.ReadFile(path)
	if err != nil {
		return sfu.Config{}, err
	}

	var cfg sfu.Config

	if err := toml.Unmarshal(b, &cfg); err != nil {
		return sfu.Config{}, err
	}

	return cfg, nil
}
