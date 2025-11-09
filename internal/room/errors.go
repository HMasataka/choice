package room

import "errors"

var (
	// Client errors
	ErrClientDisconnected  = errors.New("client is disconnected")
	ErrDataChannelNotReady = errors.New("data channel is not ready")
	ErrClientNotFound      = errors.New("client not found")
	ErrClientAlreadyExists = errors.New("client already exists")

	// Room errors
	ErrRoomNotFound      = errors.New("room not found")
	ErrRoomAlreadyExists = errors.New("room already exists")
	ErrRoomIsFull        = errors.New("room is full")
	ErrClientNotInRoom   = errors.New("client is not in the room")

	// Manager errors
	ErrInvalidRoomID   = errors.New("invalid room ID")
	ErrInvalidClientID = errors.New("invalid client ID")
)
