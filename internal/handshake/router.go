package handshake

import (
	"context"

	"github.com/HMasataka/choice/internal/handshake/handler"
	"github.com/HMasataka/choice/payload/handshake"
	webrtcinternal "github.com/HMasataka/choice/pkg/webrtc"
)

type Router struct {
	handlerRegistry HandlerRegistry
}

func NewRouter() *Router {
	return &Router{
		handlerRegistry: NewHandlerRegistry(),
	}
}

func (r *Router) Register(messageType handshake.MessageType, handler Handler) {
	r.handlerRegistry.Register(messageType, handler)
}

func (r *Router) Handle(ctx context.Context, msg *handshake.Message) (*handshake.Message, error) {
	return r.handlerRegistry.Handle(ctx, msg)
}

func NewHandshakeRouter(pc *webrtcinternal.PeerConnection) *Router {
	router := NewRouter()

	router.Register(handshake.MessageTypeRegisterResponse, handler.NewRegisterHandler())
	router.Register(handshake.MessageTypeSDP, handler.NewSessionDescriptionHandler(pc))
	router.Register(handshake.MessageTypeICECandidate, handler.NewCandidateHandler(pc))

	return router
}
