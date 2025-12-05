package main

import (
	"os"

	"log/slog"
	"net/http"

	"github.com/HMasataka/choice/payload/handshake"
	"github.com/HMasataka/choice/pkg/sfu"
	"github.com/HMasataka/logging"
	"github.com/gorilla/rpc/v2"
	"github.com/gorilla/rpc/v2/json2"
	"github.com/pelletier/go-toml/v2"
)

func NewSignalingServer(sfu *sfu.SFU) *SignalingServer {
	return &SignalingServer{
		sfu: sfu,
	}
}

type SignalingServer struct {
	sfu *sfu.SFU
}

func (h *SignalingServer) Join(r *http.Request, args *handshake.JoinRequest, reply *handshake.JoinResponse) error {
	peer := sfu.NewPeer(h.sfu)
	var joinConfig sfu.JoinConfig

	// TODO : JoinConfigの設定

	ctx := r.Context()

	if err := peer.Join(ctx, args.SessionID, args.UserID, joinConfig); err != nil {
		return err
	}

	if logging.HasLoggingContext(ctx) {
		slog.InfoContext(ctx, "peer joined", slog.String("session_id", args.SessionID), slog.String("user_id", args.UserID))
	}

	return nil
}

func (h *SignalingServer) Offer(r *http.Request, args *handshake.OfferRequest, reply *handshake.OfferResponse) error {
	return nil
}

func (h *SignalingServer) Answer(r *http.Request, args *handshake.AnswerRequest, reply *handshake.AnswerResponse) error {
	return nil
}

func (h *SignalingServer) Candidate(r *http.Request, args *handshake.CandidateRequest, reply *handshake.CandidateResponse) error {
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
