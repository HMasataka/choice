package handler

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/HMasataka/choice/payload/handshake"
	"github.com/HMasataka/choice/pkg/sdpdebug"
	"github.com/sourcegraph/jsonrpc2"
)

func (h *Handler) Answer(ctx context.Context, conn *jsonrpc2.Conn, request *jsonrpc2.Request) {
	var args handshake.AnswerRequest
	if err := json.Unmarshal(*request.Params, &args); err != nil {
		err := &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams, Message: "Invalid params"}
		if replyErr := conn.ReplyWithError(ctx, request.ID, err); replyErr != nil {
			slog.Error("failed to send error answer", "error", replyErr)
		}
		return
	}

	// Wire client's answer to subscriber via peer so it can manage negotiation flags correctly.
	sub := h.peer.Subscriber()
	if sub == nil {
		err := &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidRequest, Message: "subscriber not ready"}
		if replyErr := conn.ReplyWithError(ctx, request.ID, err); replyErr != nil {
			slog.Error("failed to send error answer", "error", replyErr)
		}
		return
	}

	sdpdebug.SaveAndLogSDP("subscriber-remote-answer", args.Answer)

	if err := h.peer.SetRemoteDescription(args.Answer); err != nil {
		err := &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams, Message: err.Error()}
		if replyErr := conn.ReplyWithError(ctx, request.ID, err); replyErr != nil {
			slog.Error("failed to send error answer", "error", replyErr)
		}
		return
	}
}
