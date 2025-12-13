package handler

import (
	"net/http"

	"github.com/gorilla/websocket"
)

func NewUpgrader() websocket.Upgrader {
	return websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
}
