package handshake

import (
	"context"
	"errors"

	"github.com/HMasataka/choice/payload/handshake"
)

type Handler interface {
	Handle(ctx context.Context, msg *handshake.Message) (*handshake.Message, error)
	CanHandle(messageType handshake.MessageType) bool
}

type HandlerFunc func(ctx context.Context, msg *handshake.Message) (*handshake.Message, error)

type HandlerRegistry interface {
	Register(messageType handshake.MessageType, handler Handler)

	Get(messageType handshake.MessageType) (Handler, bool)

	Handle(ctx context.Context, msg *handshake.Message) (*handshake.Message, error)
}

type DefaultHandlerRegistry struct {
	handlers map[handshake.MessageType]Handler
}

func NewHandlerRegistry() *DefaultHandlerRegistry {
	return &DefaultHandlerRegistry{
		handlers: make(map[handshake.MessageType]Handler),
	}
}

func (r *DefaultHandlerRegistry) Register(messageType handshake.MessageType, handler Handler) {
	r.handlers[messageType] = handler
}

func (r *DefaultHandlerRegistry) Get(messageType handshake.MessageType) (Handler, bool) {
	handler, ok := r.handlers[messageType]
	return handler, ok
}

func (r *DefaultHandlerRegistry) Handle(ctx context.Context, msg *handshake.Message) (*handshake.Message, error) {
	if msg == nil {
		return nil, errors.New("message is nil")
	}

	handler, ok := r.Get(msg.Type)
	if !ok {
		return nil, errors.New("no handler found for message type: " + string(msg.Type))
	}

	if handler == nil {
		return nil, errors.New("handler is nil for message type: " + string(msg.Type))
	}

	return handler.Handle(ctx, msg)
}
