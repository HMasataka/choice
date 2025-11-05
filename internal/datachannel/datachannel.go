package datachannel

import (
	"log"

	pkgwebrtc "github.com/HMasataka/choice/pkg/webrtc"
)

// NewDataChannel creates a new data channel wrapper
func NewDataChannel(pc *pkgwebrtc.PeerConnection) (*pkgwebrtc.DataChannel, error) {
	dc, err := pc.CreateDataChannel("chat", nil)
	if err != nil {
		log.Printf("Failed to create data channel: %v", err)
		return nil, err
	}

	d := pkgwebrtc.NewDataChannel(dc)

	d.OnOpen(func() {
		log.Println("Data channel opened")
		if err := dc.SendText("Hello from server!"); err != nil {
			log.Printf("Failed to send message: %v", err)
		}
	})

	d.OnMessage(func(msg []byte) {
		log.Printf("Received message from client: %s", string(msg))
		response := "Server received: " + string(msg)
		if err := dc.SendText(response); err != nil {
			log.Printf("Failed to send response: %v", err)
		}
	})

	d.OnClose(func() {
		log.Println("Data channel closed")
	})

	d.OnError(func(err error) {
		log.Printf("Data channel error: %v", err)
	})

	return d, nil
}
