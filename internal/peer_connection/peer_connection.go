package peerconnection

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/HMasataka/choice/internal/handshake"
	payload "github.com/HMasataka/choice/payload/handshake"
	pkgwebrtc "github.com/HMasataka/choice/pkg/webrtc"

	"github.com/pion/webrtc/v4"
)

func NewPeerConnection(ctx context.Context, sender handshake.Sender) (*pkgwebrtc.PeerConnection, error) {
	// Create media engine with Opus codec support
	mediaEngine, err := pkgwebrtc.CreateOpusMediaEngine()
	if err != nil {
		log.Printf("Failed to create Opus media engine: %v", err)
		return nil, err
	}

	log.Printf("Created Opus media engine successfully")

	// Create peer connection with Opus support
	options := pkgwebrtc.DefaultPeerConnectionOptions()
	pc, err := pkgwebrtc.NewPeerConnection(ctx, "server", options, mediaEngine)
	if err != nil {
		return nil, err
	}

	log.Printf("Created peer connection with media engine")

	// Add audio transceiver to enable audio in SDP offer
	if err := addAudioTransceiver(pc); err != nil {
		log.Printf("Failed to add audio transceiver: %v", err)
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

// addAudioTransceiver adds an audio transceiver to enable audio in SDP offer
func addAudioTransceiver(pc *pkgwebrtc.PeerConnection) error {
	// Add audio transceiver with sendrecv direction
	// This ensures the SDP offer includes audio media lines for bidirectional audio
	transceiver, err := pc.AddTransceiverFromKind(
		webrtc.RTPCodecTypeAudio,
		webrtc.RTPTransceiverDirectionSendrecv,
	)
	if err != nil {
		return fmt.Errorf("failed to add audio transceiver: %w", err)
	}

	log.Printf("Added audio transceiver: mid=%s, direction=%s",
		transceiver.Mid(), transceiver.Direction().String())

	return nil
}
