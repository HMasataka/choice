package handler

import (
	"context"
	"log"

	"github.com/HMasataka/choice/pkg/sfu"
	"github.com/sourcegraph/jsonrpc2"
)

func NewHandler(peer sfu.Peer) *Handler {
	return &Handler{
		peer: peer,
	}
}

type Handler struct {
	peer sfu.Peer
}

func (h *Handler) Handle(ctx context.Context, conn *jsonrpc2.Conn, request *jsonrpc2.Request) {
	switch request.Method {
	case "join":
		h.JoinHandle(ctx, conn, request)
	case "offer":
		h.Offer(ctx, conn, request)
	case "answer":
		h.Answer(ctx, conn, request)
	case "candidate":
		h.Candidate(ctx, conn, request)
	default:
		log.Printf("unknown method: %s", request.Method)
	}
}
