package handler

import (
	"context"
	"encoding/json"

	"log/slog"

	"github.com/HMasataka/choice/payload/handshake"
	"github.com/HMasataka/choice/pkg/sfu"
	"github.com/HMasataka/logging"
	"github.com/sourcegraph/jsonrpc2"
)

func (h *Handler) JoinHandle(ctx context.Context, conn *jsonrpc2.Conn, request *jsonrpc2.Request) {
	// TODO: JoinConfig の設定
	var joinConfig sfu.JoinConfig

	var args handshake.JoinRequest
	if err := json.Unmarshal(*request.Params, &args); err != nil {
		err := &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams, Message: "Invalid params"}
		if replyErr := conn.ReplyWithError(ctx, request.ID, err); replyErr != nil {
			slog.Error("failed to send error reply", "error", replyErr)
		}
		return
	}

	if err := h.peer.Join(ctx, args.SessionID, args.UserID, joinConfig); err != nil {
		jsonErr := &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams, Message: err.Error()}
		if replyErr := conn.ReplyWithError(ctx, request.ID, jsonErr); replyErr != nil {
			slog.Error("join error occurred", "error", replyErr)
		}
		return
	}

	pub := h.peer.Publisher()
	if pub == nil {
		return
	}

	answer, err := pub.Answer(args.Offer)
	if err != nil {
		jsonErr := &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams, Message: err.Error()}
		if replyErr := conn.ReplyWithError(ctx, request.ID, jsonErr); replyErr != nil {
			slog.Error("failed to send answer", "error", replyErr)
		}
		return
	}

	response := handshake.JoinResponse{Answer: &answer}
	if err := conn.Reply(ctx, request.ID, response); err != nil {
		slog.Error("failed to send join response", "error", err)
		return
	}

	if logging.HasLoggingContext(ctx) {
		slog.InfoContext(ctx, "peer joined", slog.String("session_id", args.SessionID), slog.String("user_id", args.UserID))
	}
}
