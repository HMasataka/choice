package main

import (
	"log"
	"net/http"
)

func main() {
	fs := http.FileServer(http.Dir("."))
	http.Handle("/", fs)

	addr := ":8000"
	log.Printf("Starting client HTTP server on %s", addr)
	log.Printf("Open http://localhost%s/client.html in your browser", addr)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
