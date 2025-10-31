package handler

import (
	"context"

	"github.com/HMasataka/choice/payload/handshake"
)

type RegisterResponseHandler struct {
}

func NewRegisterHandler() *RegisterResponseHandler {
	return &RegisterResponseHandler{}
}

func (h *RegisterResponseHandler) Handle(ctx context.Context, msg *handshake.Message) (*handshake.Message, error) {
	return nil, nil
}

func (h *RegisterResponseHandler) CanHandle(messageType handshake.MessageType) bool {
	return messageType == handshake.MessageTypeRegisterResponse
}

type UnregisterResponseHandler struct {
}

func NewUnregisterHandler() *UnregisterResponseHandler {
	return &UnregisterResponseHandler{}
}

func (h *UnregisterResponseHandler) Handle(ctx context.Context, msg *handshake.Message) (*handshake.Message, error) {
	return nil, nil
}

func (h *UnregisterResponseHandler) CanHandle(messageType handshake.MessageType) bool {
	return messageType == handshake.MessageTypeUnregisterResponse
}
