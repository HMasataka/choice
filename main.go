package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/HMasataka/choice/internal/datachannel"
	"github.com/HMasataka/choice/internal/handshake"
	peerconnection "github.com/HMasataka/choice/internal/peer_connection"
	"github.com/HMasataka/choice/internal/room"
	payload "github.com/HMasataka/choice/payload/handshake"
	pkgwebrtc "github.com/HMasataka/choice/pkg/webrtc"
	ws "github.com/gorilla/websocket"
)

var (
	upgrader = ws.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	// Global room manager
	roomManager        *room.RoomManager
	roomChannelManager *datachannel.RoomChannelManager
)

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	ctx := context.Background()

	// Get room ID from query parameters
	roomID := r.URL.Query().Get("room")
	if roomID == "" {
		roomID = "default"
	}
	roomID = room.SanitizeRoomID(roomID)

	// Get client name from query parameters
	clientName := r.URL.Query().Get("name")
	if clientName == "" {
		clientName = "Anonymous"
	}

	// Generate unique client ID
	clientID := room.GenerateClientID()

	sender := handshake.NewWebSocketSender(ctx, conn, handshake.DefaultSenderOptions())

	pc, err := peerconnection.NewPeerConnection(ctx, sender)
	if err != nil {
		log.Printf("Failed to create peer connection: %v", err)
		conn.Close()
		return
	}

	// Create client
	client := room.NewClient(clientID, roomID, clientName, conn, pc)

	// Use room-aware router
	router := handshake.NewHandshakeRouterWithRoom(pc, roomManager)
	connection := handshake.NewConnection(ctx, conn, sender, router, handshake.DefaultConnectionOptions())

	// Create data channel
	dc, err := datachannel.NewDataChannel(pc)
	if err != nil {
		log.Printf("Failed to create data channel: %v", err)
		conn.Close()
		pc.Close()
		return
	}

	// Set data channel to client
	client.SetDataChannel(dc)

	// Add client to room
	if err := roomManager.AddClient(roomID, client); err != nil {
		log.Printf("Failed to add client to room: %v", err)
		conn.Close()
		pc.Close()
		return
	}

	// Set up room data channel messaging
	roomChannelManager.SetupRoomDataChannel(client)

	if err := sendOffer(ctx, pc, sender); err != nil {
		log.Printf("Failed to send offer: %v", err)
		roomManager.RemoveClient(clientID)
		conn.Close()
		pc.Close()
		return
	}

	log.Printf("WebRTC offer sent to client %s in room %s", clientID, roomID)

	// Set up connection close handler
	conn.SetCloseHandler(func(code int, text string) error {
		log.Printf("Client %s disconnected: %d %s", clientID, code, text)
		roomManager.RemoveClient(clientID)
		return nil
	})

	sender.Start(ctx)
	connection.Start(ctx)

	// The connection will be cleaned up by the close handler
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
	// Initialize room manager
	roomManager = room.NewRoomManager(10) // Default max 10 clients per room

	// Initialize room channel manager
	roomChannelManager = datachannel.NewRoomChannelManager(roomManager)

	// Set up HTTP routes
	http.HandleFunc("/ws", handleWebSocket)

	// Add room management API endpoints
	http.HandleFunc("/api/rooms", handleRoomsAPI)
	http.HandleFunc("/api/stats", handleStatsAPI)

	addr := ":8080"
	log.Printf("Starting WebSocket server on %s", addr)
	log.Printf("Room manager initialized with default max clients: %d", 10)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// handleRoomsAPI handles room list API requests
func handleRoomsAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	rooms := roomManager.GetRoomList()
	if err := json.NewEncoder(w).Encode(rooms); err != nil {
		http.Error(w, "Failed to encode rooms", http.StatusInternalServerError)
		return
	}
}

// handleStatsAPI handles statistics API requests
func handleStatsAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	stats := roomManager.GetStats()
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		http.Error(w, "Failed to encode stats", http.StatusInternalServerError)
		return
	}
}
