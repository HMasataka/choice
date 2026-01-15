package room

import "errors"

var (
	// ErrRoomNotFound is returned when a room is not found.
	ErrRoomNotFound = errors.New("room not found")

	// ErrRoomFull is returned when a room has reached its maximum number of participants.
	ErrRoomFull = errors.New("room is full")

	// ErrRoomLocked is returned when attempting to join a locked room.
	ErrRoomLocked = errors.New("room is locked")

	// ErrRoomClosed is returned when attempting to perform an operation on a closed room.
	ErrRoomClosed = errors.New("room is closed")

	// ErrRoomNotLocked is returned when attempting to unlock a room that is not locked.
	ErrRoomNotLocked = errors.New("room is not locked")

	// ErrParticipantNotFound is returned when a participant is not found.
	ErrParticipantNotFound = errors.New("participant not found")

	// ErrParticipantAlreadyJoined is returned when a participant has already joined the room.
	ErrParticipantAlreadyJoined = errors.New("participant has already joined")

	// ErrMaxTracksReached is returned when the maximum number of tracks for a room has been reached.
	ErrMaxTracksReached = errors.New("maximum tracks reached")
)
