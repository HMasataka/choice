package main

import (
	"log"
	"net/http"
	"os"

	"log/slog"

	"github.com/HMasataka/choice/internal/handler"
	"github.com/HMasataka/choice/pkg/sfu"
	"github.com/HMasataka/logging"
	"github.com/gorilla/websocket"
	"github.com/pelletier/go-toml/v2"
	"github.com/sourcegraph/jsonrpc2"
)

func loadConfig() (sfu.Config, error) {
	path := "config.toml"
	b, err := os.ReadFile(path)
	if err != nil {
		return sfu.Config{}, err
	}

	var cfg sfu.Config

	if err := toml.Unmarshal(b, &cfg); err != nil {
		return sfu.Config{}, err
	}

	return cfg, nil
}

func main() {
	logger := slog.New(logging.NewHandler(slog.NewJSONHandler(os.Stdout, nil)))
	slog.SetDefault(logger)

	cfg, err := loadConfig()
	if err != nil {
		slog.Error("failed to load config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	slog.Info("config loaded", slog.Any("config", cfg))

	s := sfu.NewSFU(cfg)
	peer := sfu.NewPeer(s)

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		upgrader := handler.NewUpgrader()
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("Failed to upgrade to WebSocket: %v", err)
			return
		}
		defer ws.Close()

		ctx := r.Context()
		conn := jsonrpc2.NewConn(
			ctx,
			jsonrpc2.NewBufferedStream(
				&websocketReadWriteCloser{ws: ws},
				jsonrpc2.VSCodeObjectCodec{},
			),
			handler.NewHandler(peer),
		)
		<-conn.DisconnectNotify()
	})

	slog.Info("Starting signaling server on :8081")

	if err := http.ListenAndServe(":8081", nil); err != nil {
		log.Fatal(err)
	}

}

type websocketReadWriteCloser struct {
	ws *websocket.Conn
}

func (w *websocketReadWriteCloser) Read(p []byte) (n int, err error) {
	_, data, err := w.ws.ReadMessage()
	if err != nil {
		return 0, err
	}
	copy(p, data)
	return len(data), nil
}

func (w *websocketReadWriteCloser) Write(p []byte) (n int, err error) {
	if err := w.ws.WriteMessage(websocket.TextMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *websocketReadWriteCloser) Close() error {
	return w.ws.Close()
}
