package main

import (
	"log"
	"net/http"

	"github.com/HMasataka/choice/payload/handshake"
	"github.com/gorilla/rpc/v2"
	"github.com/gorilla/rpc/v2/json2"
)

type SignalingServer struct{}

func (h *SignalingServer) Join(r *http.Request, args *handshake.JoinRequest, reply *handshake.JoinResponse) error {
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

	signaling := &SignalingServer{}
	server.RegisterService(signaling, "")

	mux := http.NewServeMux()
	mux.Handle("/", server)

	if err := http.ListenAndServe(":8081", mux); err != nil {
		log.Fatal(err)
	}
}
