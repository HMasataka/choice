# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Choice is a WebRTC Selective Forwarding Unit (SFU) server written in Go 1.23. It implements multi-party video/audio conferencing with Simulcast, RTCP handling, session management, and quality control.

## Build and Development Commands

```bash
make build          # Build binary to bin/sfu
make run            # Build and run the SFU server
make test           # Run all tests with race detection
make test-coverage  # Generate coverage report (HTML)
make lint           # Run golangci-lint
make fmt            # Format code (gofmt + goimports)
make vet            # Run go vet
make check          # Run fmt, vet, lint, and test (comprehensive check)
make tidy           # Tidy go.mod
make tools          # Install dev tools (goimports, golangci-lint)
make clean          # Remove binaries and coverage files
```

### Running a single test
```bash
go test -v -race -run TestFunctionName ./internal/package/...
```

## Architecture

```
cmd/sfu/           # Application entrypoint
internal/
  server/          # HTTP server, REST API endpoints
  signaling/       # WebSocket handler, JSON-RPC 2.0 protocol
  room/            # Room and participant management
  media/           # Media routing, Simulcast, track management
  auth/            # JWT validation with JWKS, permissions
  store/           # Session storage (Redis or memory)
  webrtc/          # WebRTC peer connections, ICE
  recording/       # Media recording (optional)
pkg/
  config/          # Configuration management
  logger/          # Structured logging with PII masking
  metrics/         # Prometheus metrics
```

### Key Components
- **HTTP Server**: REST API for room/participant management, health checks
- **WebSocket Signaling**: JSON-RPC 2.0 over WebSocket for real-time signaling
- **Room Manager**: Room lifecycle, participant limits (500 max tracks/room)
- **Media Router**: Publisher/Subscriber model, SSRC mapping, sequence rewriting
- **Simulcast Controller**: Layer selection (h/m/l) based on bandwidth and packet loss
- **RTCP Processor**: TWCC, NACK, PLI, FIR handling
- **Session Store**: Redis-backed session persistence for 30-second reconnection window

### Core Dependencies
- `github.com/pion/webrtc/v4` - WebRTC implementation
- `github.com/gorilla/websocket` - WebSocket transport
- `github.com/golang-jwt/jwt/v5` - JWT handling
- `github.com/stretchr/testify` - Testing utilities

## Configuration

Configuration via `configs/config.yaml`. Key sections: server (HTTP/WebSocket), webrtc (ICE servers, port range), media (Simulcast layers), auth (JWT/JWKS), store (Redis/memory).

## Code Conventions

- **Import prefix**: `github.com/HMasataka/choice` (configured in .golangci.yml)
- **Error handling**: Explicit returns, custom error types with `Err` prefix
- **Concurrency**: `sync.RWMutex` for shared state, context propagation
- **Testing**: Table-driven tests with `testify`, 80%+ coverage target
- **Logging**: Structured logging with context fields

## Signaling Protocol

JSON-RPC 2.0 methods: `join`, `leave`, `publish`, `unpublish`, `subscribe`, `unsubscribe`, `setPreferredLayer`

Server notifications: `participantJoined`, `participantLeft`, `trackPublished`, `trackUnpublished`, `layerChanged`

## Documentation

- `docs/design.md` - Complete system design (Japanese)
- `docs/adr/` - Architecture Decision Records
- `docs/test-plan.md` - Test strategy and coverage targets
