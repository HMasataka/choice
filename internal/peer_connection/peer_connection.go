package peerconnection

import (
	"context"
	"encoding/json"
	"log"

	"github.com/HMasataka/choice/internal/handshake"
	payload "github.com/HMasataka/choice/payload/handshake"
	pkgwebrtc "github.com/HMasataka/choice/pkg/webrtc"

	"github.com/pion/webrtc/v4"
)

func NewPeerConnection(ctx context.Context, sender handshake.Sender) (*pkgwebrtc.PeerConnection, error) {
	pc, err := pkgwebrtc.NewPeerConnection(ctx, "server", pkgwebrtc.DefaultPeerConnectionOptions())
	if err != nil {
		return nil, err
	}

	pc.SetOnICECandidate(func(candidate *webrtc.ICECandidate) error {
		if candidate == nil {
			return nil
		}

		log.Printf("Generated ICE candidate: %s", candidate.String())

		msg, err := payload.NewICECandidateMessage("server", candidate.ToJSON())
		if err != nil {
			log.Printf("Failed to create ICE candidate message: %v", err)
			return err
		}

		msgBytes, err := json.Marshal(msg)
		if err != nil {
			log.Printf("Failed to marshal ICE candidate: %v", err)
			return err
		}

		if err := sender.Send(ctx, msgBytes); err != nil {
			log.Printf("Failed to send ICE candidate: %v", err)
			return err
		}

		log.Println("ICE candidate sent successfully")
		return nil
	})

	pc.SetOnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("Connection state changed: %s", state.String())
	})

	pc.SetOnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("ICE connection state changed: %s", state.String())
	})

	pc.SetOnICEGatheringStateChange(func(state webrtc.ICEGatheringState) {
		log.Printf("ICE gathering state changed: %s", state.String())
	})

	return pc, nil
}
