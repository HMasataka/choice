# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Choice is a WebRTC Selective Forwarding Unit (SFU) server written in Go. It handles real-time media routing between multiple peers using the pion/webrtc library.

## Common Commands

```bash
# Run the server
go run cmd/server/main.go

# Build
go build -o choice ./cmd/server

# Run tests
go test ./...

# Run a specific test
go test -v ./pkg/sfu -run TestAudioObserver
```

## Architecture

### Core Components

**SFU** (`pkg/sfu/sfu.go`) - Top-level entry point. Manages sessions and WebRTC transport configuration.

**Session** (`pkg/sfu/session.go`) - Groups peers sharing media. Handles publish/subscribe logic and datachannel fanout. Each session has an AudioObserver for active speaker detection.

**Peer** (`pkg/sfu/peer.go`) - Represents a connected client. Each peer has:

- **Publisher** - Receives upstream media from the client
- **Subscriber** - Sends downstream media to the client

**Router** (`pkg/sfu/router.go`) - Routes RTP packets from Receivers to DownTracks. Handles TWCC feedback and RTCP routing.

**Receiver** (`pkg/sfu/receiver.go`) - Wraps an incoming WebRTC track. Manages simulcast layers and distributes packets to DownTracks.

**DownTrack** (`pkg/sfu/downtrack.go`) - Outbound track to a subscriber. Handles layer switching, packet sequencing, and RTCP reports.

### Signaling Flow

1. Client connects via WebSocket to `/ws` endpoint
2. JSON-RPC 2.0 messages handled by `internal/handler/handler.go`
3. Methods: `join`, `offer`, `answer`, `candidate`
4. Each peer uses two PeerConnections (Publisher PC for sending, Subscriber PC for receiving)

### Media Flow

```
Client A (Publisher)
    ↓ (RTP)
Publisher.OnTrack → Router.AddReceiver → Receiver
    ↓
Session.Publish → Router.AddDownTracks
    ↓
DownTrack → Subscriber PC → Client B
```

### Configuration

Config loaded from `config.toml`. Key sections:

- `[sfu]` - Ballast memory, stats
- `[router]` - Bandwidth limits, simulcast settings, audio level detection
- `[webrtc]` - ICE ports, STUN/TURN servers, SDP semantics
- `[turn]` - Embedded TURN server

### Key Packages

- `pkg/buffer` - RTP/RTCP packet buffering, NACK handling
- `pkg/relay` - SFU-to-SFU relay (cascading)
- `pkg/twcc` - Transport-Wide Congestion Control
- `pkg/sdpdebug` - SDP logging for debugging
