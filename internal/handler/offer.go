package handler

import (
	"context"
	"encoding/json"

	"log/slog"

	"github.com/HMasataka/choice/payload/handshake"
	"github.com/sourcegraph/jsonrpc2"
)

func (h *Handler) Offer(ctx context.Context, conn *jsonrpc2.Conn, request *jsonrpc2.Request) {
	var args handshake.OfferRequest
	if err := json.Unmarshal(*request.Params, &args); err != nil {
		err := &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams, Message: "Invalid params"}
		if replyErr := conn.ReplyWithError(ctx, request.ID, err); replyErr != nil {
			slog.Error("failed to send error offer", "error", replyErr)
		}
		return
	}

	pub := h.peer.Publisher()
	if pub == nil {
		slog.Warn("publisher not found for offer", slog.String("session_id", args.SessionID), slog.String("user_id", args.UserID))
		return
	}

	answer, err := pub.Answer(args.Offer)
	if err != nil {
		err := &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams, Message: err.Error()}
		if replyErr := conn.ReplyWithError(ctx, request.ID, err); replyErr != nil {
			slog.Error("failed to send error offer", "error", replyErr)
		}
		return
	}

	reply := handshake.OfferResponse{Answer: &answer}
	if err := conn.Reply(ctx, request.ID, reply); err != nil {
		slog.Error("failed to send offer response", "error", err)
		return
	}
}
