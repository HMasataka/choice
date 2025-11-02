package handler

import (
	"context"
	"encoding/json"

	"github.com/HMasataka/choice/payload/handshake"
	webrtcinternal "github.com/HMasataka/choice/pkg/webrtc"
)

type CandidateHandler struct {
	pc *webrtcinternal.PeerConnection
}

func NewCandidateHandler(pc *webrtcinternal.PeerConnection) *CandidateHandler {
	return &CandidateHandler{
		pc: pc,
	}
}

func (h *CandidateHandler) Handle(ctx context.Context, msg *handshake.Message) (*handshake.Message, error) {
	var candidateMsg handshake.ICECandidateMessage

	if err := json.Unmarshal(msg.Data, &candidateMsg); err != nil {
		return nil, err
	}

	if err := h.pc.AddICECandidate(candidateMsg.Candidate); err != nil {
		return nil, err
	}

	return nil, nil
}

func (h *CandidateHandler) CanHandle(messageType handshake.MessageType) bool {
	return messageType == handshake.MessageTypeICECandidate
}
