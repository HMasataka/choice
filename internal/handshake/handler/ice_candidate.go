package handler

import (
	"context"
	"encoding/json"
	"log"

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
		log.Printf("Failed to unmarshal ICE candidate: %v", err)
		return nil, err
	}

	log.Printf("Received ICE candidate from %s: %s", candidateMsg.SenderID, candidateMsg.Candidate.Candidate)

	if err := h.pc.AddICECandidate(candidateMsg.Candidate); err != nil {
		log.Printf("Failed to add ICE candidate: %v", err)
		return nil, err
	}

	log.Println("Successfully added ICE candidate")
	return nil, nil
}

func (h *CandidateHandler) CanHandle(messageType handshake.MessageType) bool {
	return messageType == handshake.MessageTypeICECandidate
}
