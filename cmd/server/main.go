package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/HMasataka/choice/pkg/sfu"
	"github.com/pion/webrtc/v4"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

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
		slog.Info("SFU server starting on", "Addr", *addr)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	slog.Info("shutting down server...")
	server.Close()
}
