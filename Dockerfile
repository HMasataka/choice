# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)" \
    -o /app/bin/sfu ./cmd/sfu

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 sfu && \
    adduser -u 1000 -G sfu -s /bin/sh -D sfu

# Copy binary from builder
COPY --from=builder /app/bin/sfu /app/sfu

# Copy default config
COPY --from=builder /app/configs/config.yaml /app/configs/config.yaml

# Create directories for recordings and logs
RUN mkdir -p /app/recordings /app/logs && \
    chown -R sfu:sfu /app

# Switch to non-root user
USER sfu

# Expose ports
# HTTP/REST API
EXPOSE 8080
# WebRTC UDP port range (default: 10000-20000, configurable via config.yaml)
# Note: Docker only exposes a subset. For production, use host networking
# or configure specific ports based on expected concurrent connections.
EXPOSE 10000-10100/udp

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8080/health || exit 1

# Default command
ENTRYPOINT ["/app/sfu"]
CMD ["--config", "/app/configs/config.yaml"]
