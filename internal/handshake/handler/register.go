package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/HMasataka/choice/payload/handshake"
	"github.com/rs/xid"
)

type RegisterResponseHandler struct {
}

func NewRegisterHandler() *RegisterResponseHandler {
	return &RegisterResponseHandler{}
}

func (h *RegisterResponseHandler) Handle(ctx context.Context, msg *handshake.Message) (*handshake.Message, error) {
	clientID := xid.New().String()

	response := &handshake.RegisterResponse{
		ClientID: clientID,
	}

	b, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}

	return &handshake.Message{
		ID:        xid.New().String(),
		Type:      handshake.MessageTypeRegisterResponse,
		Timestamp: time.Now(),
		Data:      b,
	}, nil
}

func (h *RegisterResponseHandler) CanHandle(messageType handshake.MessageType) bool {
	return messageType == handshake.MessageTypeRegisterResponse
}
