package handler

import (
	"context"
	"encoding/json"
	"log"
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
		log.Printf("Failed to unmarshal SDP message: %v", err)
		return nil, err
	}

	log.Printf("Received SDP type: %s from %s", sdpMsg.SessionDescription.Type, sdpMsg.SenderID)

	if err := h.pc.SetRemoteDescription(sdpMsg.SessionDescription); err != nil {
		log.Printf("Failed to set remote description: %v", err)
		return nil, err
	}

	if sdpMsg.SessionDescription.Type == webrtc.SDPTypeAnswer {
		log.Println("Received answer, no response needed")
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
