package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/HMasataka/choice/payload/handshake"
	"github.com/HMasataka/choice/pkg/sfu"
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

	if err := peer.Join(args.SessionID, args.UserID, joinConfig); err != nil {
		return err
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
	server := rpc.NewServer()
	server.RegisterCodec(json2.NewCodec(), "application/json")

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	fmt.Printf("Loaded config: %+v\n", cfg)

	sfu := sfu.NewSFU(cfg)
	signaling := NewSignalingServer(sfu)
	server.RegisterService(signaling, "")

	mux := http.NewServeMux()
	mux.Handle("/", server)

	fmt.Println("Starting signaling server on :8081")

	if err := http.ListenAndServe(":8081", mux); err != nil {
		log.Fatal(err)
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
