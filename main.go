package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/HMasataka/choice/internal/handshake"
	payload "github.com/HMasataka/choice/payload/handshake"
	webrtcinternal "github.com/HMasataka/choice/pkg/webrtc"
	ws "github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

var upgrader = ws.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	ctx := context.Background()

	pc, err := webrtcinternal.NewPeerConnection(ctx, "server", webrtcinternal.DefaultPeerConnectionOptions())
	if err != nil {
		log.Printf("Failed to create peer connection: %v", err)
		conn.Close()
		return
	}

	pc.SetOnICECandidate(func(candidate *webrtc.ICECandidate) error {
		if candidate == nil {
			return nil
		}

		msg, err := payload.NewICECandidateMessage("server", candidate.ToJSON())
		if err != nil {
			return err
		}

		msgBytes, err := json.Marshal(msg)
		if err != nil {
			return err
		}

		return conn.WriteMessage(ws.TextMessage, msgBytes)
	})

	pc.SetOnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("Connection state changed: %s", state.String())
	})

	router := handshake.NewHandshakeRouter(pc)
	sender := handshake.NewWebSocketSender(ctx, conn, handshake.DefaultSenderOptions())
	connection := handshake.NewConnection(ctx, conn, sender, router, handshake.DefaultConnectionOptions())

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		log.Printf("Failed to create offer: %v", err)
		conn.Close()
		pc.Close()
		return
	}

	msg, err := payload.NewSDPMessage("server", offer)
	if err != nil {
		log.Printf("Failed to create SDP message: %v", err)
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal message: %v", err)
		conn.Close()
		pc.Close()
		return
	}

	if err := connection.Send(ctx, msgBytes); err != nil {
		log.Printf("Failed to send offer: %v", err)
		conn.Close()
		pc.Close()
		return
	}

	log.Println("WebRTC offer sent to client")

	connection.Start(ctx)

	pc.Close()
}

func main() {
	http.HandleFunc("/ws", handleWebSocket)

	addr := ":8080"
	log.Printf("Starting WebSocket server on %s", addr)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
