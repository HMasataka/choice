package protocol

import (
	"fmt"
)

// Standard JSON-RPC 2.0 error codes.
const (
	// Parse error - Invalid JSON was received.
	CodeParseError = -32700
	// Invalid Request - The JSON sent is not a valid Request object.
	CodeInvalidRequest = -32600
	// Method not found - The method does not exist.
	CodeMethodNotFound = -32601
	// Invalid params - Invalid method parameter(s).
	CodeInvalidParams = -32602
	// Internal error - Internal JSON-RPC error.
	CodeInternalError = -32603
)

// Application-specific error codes (1001-1009).
const (
	// Room not found.
	CodeRoomNotFound = 1001
	// Room is full.
	CodeRoomFull = 1002
	// Unauthorized access.
	CodeUnauthorized = 1003
	// Already joined a room.
	CodeAlreadyJoined = 1004
	// Not in a room.
	CodeNotInRoom = 1005
	// Track not found.
	CodeTrackNotFound = 1006
	// Invalid SDP.
	CodeInvalidSDP = 1007
	// ICE connection failure.
	CodeICEFailure = 1008
	// Session expired.
	CodeSessionExpired = 1009
)

// Error represents a JSON-RPC 2.0 error object.
type Error struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Data != nil {
		return fmt.Sprintf("JSON-RPC error %d: %s (data: %v)", e.Code, e.Message, e.Data)
	}
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// NewError creates a new JSON-RPC error.
func NewError(code int, message string, data map[string]interface{}) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Data:    data,
	}
}

// NewParseError creates a parse error (-32700).
func NewParseError(details string) *Error {
	return &Error{
		Code:    CodeParseError,
		Message: "Parse error",
		Data:    map[string]interface{}{"details": details},
	}
}

// NewInvalidRequestError creates an invalid request error (-32600).
func NewInvalidRequestError(details string) *Error {
	return &Error{
		Code:    CodeInvalidRequest,
		Message: "Invalid Request",
		Data:    map[string]interface{}{"details": details},
	}
}

// NewMethodNotFoundError creates a method not found error (-32601).
func NewMethodNotFoundError(method string) *Error {
	return &Error{
		Code:    CodeMethodNotFound,
		Message: "Method not found",
		Data:    map[string]interface{}{"method": method},
	}
}

// NewInvalidParamsError creates an invalid params error (-32602).
func NewInvalidParamsError(details string) *Error {
	return &Error{
		Code:    CodeInvalidParams,
		Message: "Invalid params",
		Data:    map[string]interface{}{"details": details},
	}
}

// NewInternalError creates an internal error (-32603).
func NewInternalError(details string) *Error {
	return &Error{
		Code:    CodeInternalError,
		Message: "Internal error",
		Data:    map[string]interface{}{"details": details},
	}
}

// NewRoomNotFoundError creates a room not found error (1001).
func NewRoomNotFoundError(roomID string) *Error {
	return &Error{
		Code:    CodeRoomNotFound,
		Message: "Room not found",
		Data:    map[string]interface{}{"roomId": roomID},
	}
}

// NewRoomFullError creates a room full error (1002).
func NewRoomFullError(roomID string) *Error {
	return &Error{
		Code:    CodeRoomFull,
		Message: "Room full",
		Data:    map[string]interface{}{"roomId": roomID},
	}
}

// NewUnauthorizedError creates an unauthorized error (1003).
func NewUnauthorizedError(reason string) *Error {
	return &Error{
		Code:    CodeUnauthorized,
		Message: "Unauthorized",
		Data:    map[string]interface{}{"reason": reason},
	}
}

// NewAlreadyJoinedError creates an already joined error (1004).
func NewAlreadyJoinedError(roomID string) *Error {
	return &Error{
		Code:    CodeAlreadyJoined,
		Message: "Already joined",
		Data:    map[string]interface{}{"roomId": roomID},
	}
}

// NewNotInRoomError creates a not in room error (1005).
func NewNotInRoomError() *Error {
	return &Error{
		Code:    CodeNotInRoom,
		Message: "Not in room",
	}
}

// NewTrackNotFoundError creates a track not found error (1006).
func NewTrackNotFoundError(trackID string) *Error {
	return &Error{
		Code:    CodeTrackNotFound,
		Message: "Track not found",
		Data:    map[string]interface{}{"trackId": trackID},
	}
}

// NewInvalidSDPError creates an invalid SDP error (1007).
func NewInvalidSDPError(details string) *Error {
	return &Error{
		Code:    CodeInvalidSDP,
		Message: "Invalid SDP",
		Data:    map[string]interface{}{"details": details},
	}
}

// NewICEFailureError creates an ICE failure error (1008).
func NewICEFailureError(details string) *Error {
	return &Error{
		Code:    CodeICEFailure,
		Message: "ICE failure",
		Data:    map[string]interface{}{"details": details},
	}
}

// NewSessionExpiredError creates a session expired error (1009).
func NewSessionExpiredError(sessionID string) *Error {
	return &Error{
		Code:    CodeSessionExpired,
		Message: "Session expired",
		Data:    map[string]interface{}{"sessionId": sessionID},
	}
}

// IsStandardError returns true if the error code is a standard JSON-RPC error.
func IsStandardError(code int) bool {
	return code >= -32768 && code <= -32000
}

// IsApplicationError returns true if the error code is an application-specific error.
func IsApplicationError(code int) bool {
	return code >= 1001 && code <= 1009
}
