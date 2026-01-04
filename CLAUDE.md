# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Run Commands

```bash
# Run the server
go run cmd/server/main.go

# Build the binary
go build -o choice ./cmd/server

# Run tests
go test ./...

# Run a single test
go test -run TestName ./pkg/sfu

# Install dependencies
go mod tidy
```

## Architecture

Choice is a WebRTC SFU (Selective Forwarding Unit) for multi-party video conferencing written in Go.

### Core Components

- **SFU** (`pkg/sfu/sfu.go`): Main entry point. Manages sessions and creates WebRTC peer connections via pion/webrtc.
- **Session** (`pkg/sfu/session.go`): Represents a room. Contains peers (participants) and routers (media sources).
- **Peer** (`pkg/sfu/peer.go`): A connected client. Combines a Publisher (upstream) and Subscriber (downstream).
- **Router** (`pkg/sfu/router.go`): Routes media from one publisher to multiple subscribers via Forwarder.
- **Forwarder** (`pkg/sfu/forwarder.go`): Distributes RTP packets to multiple DownTracks with layer selection (simulcast).
- **DownTrack** (`pkg/sfu/downtrack.go`): Sends RTP to a subscriber with sequence number rewriting.
- **Receiver** (`pkg/sfu/receiver.go`): Receives RTP packets from remote tracks and forwards to router.

### Media Flow

```
Publisher → Receiver → Router → Forwarder → DownTrack → Subscriber
```

### Signaling

JSON-RPC 2.0 over WebSocket (`/ws` endpoint). Methods: `join`, `leave`, `subscribe`, `candidate`, `answer`, `setLayer`, `getLayer`.

Server notifications: `offer`, `candidate`, `trackAdded`.

### Simulcast

Supports three layers: `high`, `mid`, `low`. Layer switching via `setLayer` signaling method.
