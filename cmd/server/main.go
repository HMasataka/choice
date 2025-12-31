package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/HMasataka/choice/pkg/sfu"
	"github.com/pion/webrtc/v4"
)

func main() {
	addr := flag.String("addr", ":8080", "server address")
	webDir := flag.String("web", "web", "web directory path")
	flag.Parse()

	config := sfu.Config{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	s := sfu.NewSFU(config)

	http.HandleFunc("/ws", s.HandleWebSocket)

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	http.Handle("/", http.FileServer(http.Dir(*webDir)))

	server := &http.Server{
		Addr: *addr,
	}

	go func() {
		log.Printf("SFU server starting on %s", *addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("shutting down server...")
	server.Close()
}
