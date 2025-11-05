package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/HMasataka/choice/internal/datachannel"
	"github.com/HMasataka/choice/internal/handshake"
	peerconnection "github.com/HMasataka/choice/internal/peer_connection"
	payload "github.com/HMasataka/choice/payload/handshake"
	pkgwebrtc "github.com/HMasataka/choice/pkg/webrtc"
	ws "github.com/gorilla/websocket"
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

	sender := handshake.NewWebSocketSender(ctx, conn, handshake.DefaultSenderOptions())

	pc, err := peerconnection.NewPeerConnection(ctx, sender)
	if err != nil {
		log.Printf("Failed to create peer connection: %v", err)
		conn.Close()
		return
	}

	router := handshake.NewHandshakeRouter(pc)
	connection := handshake.NewConnection(ctx, conn, sender, router, handshake.DefaultConnectionOptions())

	if _, err := datachannel.NewDataChannel(pc); err != nil {
		log.Printf("Failed to create data channel: %v", err)
		conn.Close()
		pc.Close()
		return
	}

	if err := sendOffer(ctx, pc, sender); err != nil {
		log.Printf("Failed to marshal message: %v", err)
		conn.Close()
		pc.Close()
		return
	}

	log.Println("WebRTC offer sent to client")

	sender.Start(ctx)
	connection.Start(ctx)

	pc.Close()
}

func sendOffer(ctx context.Context, pc *pkgwebrtc.PeerConnection, sender handshake.Sender) error {
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		log.Printf("Failed to create offer: %v", err)
		return err
	}

	msg, err := payload.NewSDPMessage("server", offer)
	if err != nil {
		log.Printf("Failed to create SDP message: %v", err)
		return err
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal message: %v", err)
		return err
	}

	if err := sender.Send(ctx, msgBytes); err != nil {
		log.Printf("Failed to send offer: %v", err)
		return err
	}

	return nil
}

func main() {
	http.HandleFunc("/ws", handleWebSocket)

	addr := ":8080"
	log.Printf("Starting WebSocket server on %s", addr)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
