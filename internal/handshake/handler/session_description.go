package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/HMasataka/choice/payload/handshake"
	pkgwebrtc "github.com/HMasataka/choice/pkg/webrtc"
	"github.com/pion/webrtc/v4"
	"github.com/rs/xid"
)

type SessionDescriptionHandler struct {
	pc *pkgwebrtc.PeerConnection
}

func NewSessionDescriptionHandler(pc *pkgwebrtc.PeerConnection) *SessionDescriptionHandler {
	return &SessionDescriptionHandler{
		pc: pc,
	}
}

func (h *SessionDescriptionHandler) Handle(ctx context.Context, msg *handshake.Message) (*handshake.Message, error) {
	var sdpMsg handshake.SDPMessage

	if err := json.Unmarshal(msg.Data, &sdpMsg); err != nil {
		return nil, err
	}

	if err := h.pc.SetRemoteDescription(sdpMsg.SessionDescription); err != nil {
		return nil, err
	}

	if sdpMsg.SessionDescription.Type == webrtc.SDPTypeAnswer {
		return nil, nil
	}

	answer, err := h.pc.CreateAnswer(nil)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(handshake.SDPMessage{
		SenderID:           "server",
		SessionDescription: answer,
	})
	if err != nil {
		return nil, err
	}

	response := &handshake.Message{
		ID:        xid.New().String(),
		Type:      handshake.MessageTypeSDP,
		Timestamp: time.Now(),
		Data:      data,
	}

	return response, nil
}

func (h *SessionDescriptionHandler) CanHandle(messageType handshake.MessageType) bool {
	return messageType == handshake.MessageTypeSDP
}
