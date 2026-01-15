package protocol

import (
	"encoding/json"
)

// Version is the JSON-RPC version.
const Version = "2.0"

// Message represents a generic JSON-RPC 2.0 message.
// It can be a Request, Response, or Notification.
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *string         `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// IsRequest returns true if the message is a request (has id and method).
func (m *Message) IsRequest() bool {
	return m.ID != nil && m.Method != ""
}

// IsResponse returns true if the message is a response (has id and either result or error).
func (m *Message) IsResponse() bool {
	return m.ID != nil && m.Method == "" && (m.Result != nil || m.Error != nil)
}

// IsNotification returns true if the message is a notification (has method but no id).
func (m *Message) IsNotification() bool {
	return m.ID == nil && m.Method != ""
}

// IsError returns true if the message is an error response.
func (m *Message) IsError() bool {
	return m.Error != nil
}

// ParseMessage parses a JSON message into a Message struct.
func ParseMessage(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, NewParseError(err.Error())
	}

	// Validate JSON-RPC version
	if msg.JSONRPC != Version {
		return nil, NewInvalidRequestError("invalid JSON-RPC version")
	}
	if msg.ID != nil && !isValidUUID(*msg.ID) {
		return nil, NewInvalidRequestError("invalid request id format")
	}
	if msg.Result != nil && msg.Error != nil {
		return nil, NewInvalidRequestError("response cannot have both result and error")
	}
	if msg.ID != nil && msg.Method == "" && msg.Result == nil && msg.Error == nil {
		return nil, NewInvalidRequestError("response must have result or error")
	}

	return &msg, nil
}

// MarshalMessage marshals a Message to JSON.
func MarshalMessage(msg *Message) ([]byte, error) {
	return json.Marshal(msg)
}

// TrackKind represents the type of media track.
type TrackKind string

const (
	TrackKindVideo TrackKind = "video"
	TrackKindAudio TrackKind = "audio"
)

// SimulcastLayer represents a simulcast quality layer.
type SimulcastLayer string

const (
	SimulcastLayerHigh   SimulcastLayer = "h" // 1280x720
	SimulcastLayerMedium SimulcastLayer = "m" // 640x360
	SimulcastLayerLow    SimulcastLayer = "l" // 320x180
)

// ParticipantState represents the state of a participant.
type ParticipantState string

const (
	ParticipantStateJoining     ParticipantState = "joining"
	ParticipantStateJoined      ParticipantState = "joined"
	ParticipantStatePublishing  ParticipantState = "publishing"
	ParticipantStateSubscribing ParticipantState = "subscribing"
	ParticipantStateLeaving     ParticipantState = "leaving"
	ParticipantStateLeft        ParticipantState = "left"
)

// LeaveReason represents the reason for a participant leaving.
type LeaveReason string

const (
	LeaveReasonLeave   LeaveReason = "leave"
	LeaveReasonTimeout LeaveReason = "timeout"
	LeaveReasonKicked  LeaveReason = "kicked"
)

// LeftReason represents the reason for being removed from a room.
type LeftReason string

const (
	LeftReasonVoluntary LeftReason = "voluntary"
	LeftReasonKicked    LeftReason = "kicked"
	LeftReasonTimeout   LeftReason = "timeout"
)

// ReconnectReason represents the reason for reconnection.
type ReconnectReason string

const (
	ReconnectReasonICEDisconnected ReconnectReason = "ice_disconnected"
	ReconnectReasonServerRestart   ReconnectReason = "server_restart"
)

// OfferReason represents the reason for renegotiation.
type OfferReason string

const (
	OfferReasonTrackAdded       OfferReason = "track_added"
	OfferReasonTrackRemoved     OfferReason = "track_removed"
	OfferReasonSimulcastChanged OfferReason = "simulcast_changed"
	OfferReasonCodecChanged     OfferReason = "codec_changed"
	OfferReasonICERestart       OfferReason = "ice_restart"
)

// LayerChangeReason represents the reason for layer change.
type LayerChangeReason string

const (
	LayerChangeReasonBandwidth   LayerChangeReason = "bandwidth"
	LayerChangeReasonUnavailable LayerChangeReason = "unavailable"
)

// ConnectionQuality represents the quality level of a connection.
type ConnectionQuality string

const (
	ConnectionQualityExcellent ConnectionQuality = "excellent"
	ConnectionQualityGood      ConnectionQuality = "good"
	ConnectionQualityFair      ConnectionQuality = "fair"
	ConnectionQualityPoor      ConnectionQuality = "poor"
)

// ServerState represents the state of the server/room.
type ServerState string

const (
	ServerStateActive      ServerState = "active"
	ServerStateDegraded    ServerState = "degraded"
	ServerStateMaintenance ServerState = "maintenance"
	ServerStateShuttingDown ServerState = "shutting_down"
)

// IceServer represents a STUN/TURN server configuration.
type IceServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// TrackInfo represents information about a media track.
type TrackInfo struct {
	TrackID   string                 `json:"trackId"`
	Kind      TrackKind              `json:"kind"`
	Simulcast bool                   `json:"simulcast,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ParticipantInfo represents information about a participant.
type ParticipantInfo struct {
	ID       string                 `json:"id"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Tracks   []TrackInfo            `json:"tracks,omitempty"`
}
