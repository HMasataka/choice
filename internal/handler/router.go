package handler

import (
	"context"

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

	router.Register(handshake.MessageTypeRegisterResponse, NewRegisterHandler())
	router.Register(handshake.MessageTypeUnregisterResponse, NewUnregisterHandler())
	router.Register(handshake.MessageTypeSDP, NewSessionDescriptionHandler(pc))
	router.Register(handshake.MessageTypeICECandidate, NewCandidateHandler(pc))

	return router
}
