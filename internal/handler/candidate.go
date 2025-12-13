package handler

import (
	"context"
	"encoding/json"

	"log/slog"

	"github.com/HMasataka/choice/payload/handshake"
	"github.com/sourcegraph/jsonrpc2"
)

func (h *Handler) Candidate(ctx context.Context, conn *jsonrpc2.Conn, request *jsonrpc2.Request) {
	var args handshake.CandidateRequest
	if err := json.Unmarshal(*request.Params, &args); err != nil {
		err := &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams, Message: "Invalid params"}
		if replyErr := conn.ReplyWithError(ctx, request.ID, err); replyErr != nil {
			slog.Error("failed to send error candidate", "error", replyErr)
		}
		return
	}

	switch args.ConnectionType {
	case handshake.ConnectionTypePublisher:
		pub := h.peer.Publisher()
		if pub == nil {
			err := &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidRequest, Message: "publisher not ready"}
			if replyErr := conn.ReplyWithError(ctx, request.ID, err); replyErr != nil {
				slog.Error("failed to send error candidate", "error", replyErr)
			}
			return
		}
		if err := pub.AddICECandidate(args.Candidate); err != nil {
			err := &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams, Message: err.Error()}
			if replyErr := conn.ReplyWithError(ctx, request.ID, err); replyErr != nil {
				slog.Error("failed to send error candidate", "error", replyErr)
			}
			return
		}
	case handshake.ConnectionTypeSubscriber:
		sub := h.peer.Subscriber()
		if sub == nil {
			err := &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidRequest, Message: "subscriber not ready"}
			if replyErr := conn.ReplyWithError(ctx, request.ID, err); replyErr != nil {
				slog.Error("failed to send error candidate", "error", replyErr)
			}
			return
		}
		if err := sub.AddICECandidate(args.Candidate); err != nil {
			err := &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams, Message: err.Error()}
			if replyErr := conn.ReplyWithError(ctx, request.ID, err); replyErr != nil {
				slog.Error("failed to send error candidate", "error", replyErr)
			}
			return
		}
	}
}
