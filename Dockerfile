FROM golang:1.25 AS builder

WORKDIR /app

COPY . .

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server cmd/server/main.go

FROM gcr.io/distroless/base-debian10

WORKDIR /

COPY --from=builder /app/server /server

ENTRYPOINT ["/server"]
