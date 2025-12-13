package handler

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/HMasataka/choice/payload/handshake"
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

    sub := h.peer.Subscriber()
    if sub == nil {
        err := &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidRequest, Message: "subscriber not ready"}
        if replyErr := conn.ReplyWithError(ctx, request.ID, err); replyErr != nil {
            slog.Error("failed to send error answer", "error", replyErr)
        }
        return
    }

    if err := sub.SetRemoteDescription(args.Answer); err != nil {
        err := &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams, Message: err.Error()}
        if replyErr := conn.ReplyWithError(ctx, request.ID, err); replyErr != nil {
            slog.Error("failed to send error answer", "error", replyErr)
        }
        return
    }
}
