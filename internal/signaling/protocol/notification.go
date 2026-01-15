package protocol

import (
	"encoding/json"
)

// Notification method names.
const (
	NotifyParticipantJoined      = "participantJoined"
	NotifyParticipantLeft        = "participantLeft"
	NotifyParticipantReconnected = "participantReconnected"
	NotifyTrackPublished         = "trackPublished"
	NotifyTrackUnpublished       = "trackUnpublished"
	NotifyJoined                 = "joined"
	NotifyLeft                   = "left"
	NotifyOffer                  = "offer"
	NotifyCandidate              = "candidate"
	NotifyAnswer                 = "answer"
	NotifyLayerChanged           = "layerChanged"
	NotifyError                  = "error"
	NotifyReconnect              = "reconnect"
	NotifyRecordingStarted       = "recordingStarted"
	NotifyRecordingStopped       = "recordingStopped"
	NotifyTrackSubscribed        = "trackSubscribed"
	NotifyTrackSubscriptionFail  = "trackSubscriptionFailed"
	NotifyConnectionQuality      = "connectionQualityChanged"
	NotifyServerState            = "serverStateChanged"
)

// Notification represents a JSON-RPC 2.0 notification (no id field).
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// NewNotification creates a new JSON-RPC notification.
func NewNotification(method string, params interface{}) (*Notification, error) {
	var rawParams json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		rawParams = data
	}

	return &Notification{
		JSONRPC: Version,
		Method:  method,
		Params:  rawParams,
	}, nil
}

// UnmarshalParams unmarshals the params into the given struct.
func (n *Notification) UnmarshalParams(v interface{}) error {
	if n.Params == nil {
		return nil
	}
	return json.Unmarshal(n.Params, v)
}

// Marshal marshals the notification to JSON.
func (n *Notification) Marshal() ([]byte, error) {
	return json.Marshal(n)
}

// ParticipantJoinedParams represents parameters for participantJoined notification.
type ParticipantJoinedParams struct {
	ParticipantID string                 `json:"participantId"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// ParticipantLeftParams represents parameters for participantLeft notification.
type ParticipantLeftParams struct {
	ParticipantID string      `json:"participantId"`
	Reason        LeaveReason `json:"reason"`
}

// ParticipantReconnectedParams represents parameters for participantReconnected notification.
type ParticipantReconnectedParams struct {
	ParticipantID string                 `json:"participantId"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// TrackPublishedParams represents parameters for trackPublished notification.
type TrackPublishedParams struct {
	PublisherID string                 `json:"publisherId"`
	TrackID     string                 `json:"trackId"`
	Kind        TrackKind              `json:"kind"`
	Simulcast   bool                   `json:"simulcast,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// TrackUnpublishedParams represents parameters for trackUnpublished notification.
type TrackUnpublishedParams struct {
	PublisherID string `json:"publisherId"`
	TrackID     string `json:"trackId"`
}

// JoinedParams represents parameters for joined notification (to the joiner).
type JoinedParams struct {
	ParticipantID string `json:"participantId"`
	RoomID        string `json:"roomId"`
}

// LeftParams represents parameters for left notification (to the leaver).
type LeftParams struct {
	Reason LeftReason `json:"reason"`
}

// OfferNotificationParams represents parameters for offer notification (from server).
type OfferNotificationParams struct {
	SDP    string      `json:"sdp"`
	Reason OfferReason `json:"reason"`
}

// CandidateNotificationParams represents parameters for candidate notification (from server).
type CandidateNotificationParams struct {
	Candidate     string `json:"candidate"`
	SDPMid        string `json:"sdpMid,omitempty"`
	SDPMLineIndex *int   `json:"sdpMLineIndex,omitempty"`
}

// AnswerNotificationParams represents parameters for answer notification (from server).
type AnswerNotificationParams struct {
	SDP string `json:"sdp"`
}

// LayerChangedParams represents parameters for layerChanged notification.
type LayerChangedParams struct {
	TrackID        string            `json:"trackId"`
	RequestedLayer SimulcastLayer    `json:"requestedLayer"`
	ActualLayer    SimulcastLayer    `json:"actualLayer"`
	Reason         LayerChangeReason `json:"reason"`
}

// ErrorNotificationParams represents parameters for error notification.
type ErrorNotificationParams struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Fatal   bool   `json:"fatal"`
}

// ReconnectParams represents parameters for reconnect notification.
type ReconnectParams struct {
	Reason       ReconnectReason `json:"reason"`
	RetryAfterMs int             `json:"retryAfterMs"`
}

// RecordingStartedParams represents parameters for recordingStarted notification.
type RecordingStartedParams struct {
	RecordingID string `json:"recordingId"`
	StartedBy   string `json:"startedBy"`
}

// RecordingStoppedParams represents parameters for recordingStopped notification.
type RecordingStoppedParams struct {
	RecordingID string `json:"recordingId"`
	StoppedBy   string `json:"stoppedBy"`
}

// TrackSubscribedParams represents parameters for trackSubscribed notification.
type TrackSubscribedParams struct {
	SubscriptionID string `json:"subscriptionId"`
	PublisherID    string `json:"publisherId"`
	TrackID        string `json:"trackId"`
	Kind           TrackKind `json:"kind"`
}

// TrackSubscriptionFailedParams represents parameters for trackSubscriptionFailed notification.
type TrackSubscriptionFailedParams struct {
	PublisherID string `json:"publisherId"`
	TrackID     string `json:"trackId"`
	ErrorCode   int    `json:"errorCode"`
	ErrorMsg    string `json:"errorMessage"`
}

// ConnectionQualityChangedParams represents parameters for connectionQualityChanged notification.
type ConnectionQualityChangedParams struct {
	ParticipantID string            `json:"participantId"`
	Quality       ConnectionQuality `json:"quality"`
	Score         float64           `json:"score"`
}

// ServerStateChangedParams represents parameters for serverStateChanged notification.
type ServerStateChangedParams struct {
	RoomID  string      `json:"roomId"`
	State   ServerState `json:"state"`
	Message string      `json:"message,omitempty"`
}

// NewParticipantJoinedNotification creates a participantJoined notification.
func NewParticipantJoinedNotification(participantID string, metadata map[string]interface{}) (*Notification, error) {
	return NewNotification(NotifyParticipantJoined, &ParticipantJoinedParams{
		ParticipantID: participantID,
		Metadata:      metadata,
	})
}

// NewParticipantLeftNotification creates a participantLeft notification.
func NewParticipantLeftNotification(participantID string, reason LeaveReason) (*Notification, error) {
	return NewNotification(NotifyParticipantLeft, &ParticipantLeftParams{
		ParticipantID: participantID,
		Reason:        reason,
	})
}

// NewParticipantReconnectedNotification creates a participantReconnected notification.
func NewParticipantReconnectedNotification(participantID string, metadata map[string]interface{}) (*Notification, error) {
	return NewNotification(NotifyParticipantReconnected, &ParticipantReconnectedParams{
		ParticipantID: participantID,
		Metadata:      metadata,
	})
}

// NewTrackPublishedNotification creates a trackPublished notification.
func NewTrackPublishedNotification(publisherID, trackID string, kind TrackKind, simulcast bool, metadata map[string]interface{}) (*Notification, error) {
	return NewNotification(NotifyTrackPublished, &TrackPublishedParams{
		PublisherID: publisherID,
		TrackID:     trackID,
		Kind:        kind,
		Simulcast:   simulcast,
		Metadata:    metadata,
	})
}

// NewTrackUnpublishedNotification creates a trackUnpublished notification.
func NewTrackUnpublishedNotification(publisherID, trackID string) (*Notification, error) {
	return NewNotification(NotifyTrackUnpublished, &TrackUnpublishedParams{
		PublisherID: publisherID,
		TrackID:     trackID,
	})
}

// NewJoinedNotification creates a joined notification.
func NewJoinedNotification(participantID, roomID string) (*Notification, error) {
	return NewNotification(NotifyJoined, &JoinedParams{
		ParticipantID: participantID,
		RoomID:        roomID,
	})
}

// NewLeftNotification creates a left notification.
func NewLeftNotification(reason LeftReason) (*Notification, error) {
	return NewNotification(NotifyLeft, &LeftParams{
		Reason: reason,
	})
}

// NewOfferNotification creates an offer notification.
func NewOfferNotification(sdp string, reason OfferReason) (*Notification, error) {
	return NewNotification(NotifyOffer, &OfferNotificationParams{
		SDP:    sdp,
		Reason: reason,
	})
}

// NewCandidateNotification creates a candidate notification.
func NewCandidateNotification(candidate, sdpMid string, sdpMLineIndex *int) (*Notification, error) {
	return NewNotification(NotifyCandidate, &CandidateNotificationParams{
		Candidate:     candidate,
		SDPMid:        sdpMid,
		SDPMLineIndex: sdpMLineIndex,
	})
}

// NewAnswerNotification creates an answer notification.
func NewAnswerNotification(sdp string) (*Notification, error) {
	return NewNotification(NotifyAnswer, &AnswerNotificationParams{
		SDP: sdp,
	})
}

// NewLayerChangedNotification creates a layerChanged notification.
func NewLayerChangedNotification(trackID string, requestedLayer, actualLayer SimulcastLayer, reason LayerChangeReason) (*Notification, error) {
	return NewNotification(NotifyLayerChanged, &LayerChangedParams{
		TrackID:        trackID,
		RequestedLayer: requestedLayer,
		ActualLayer:    actualLayer,
		Reason:         reason,
	})
}

// NewErrorNotification creates an error notification.
func NewErrorNotification(code int, message string, fatal bool) (*Notification, error) {
	return NewNotification(NotifyError, &ErrorNotificationParams{
		Code:    code,
		Message: message,
		Fatal:   fatal,
	})
}

// NewReconnectNotification creates a reconnect notification.
func NewReconnectNotification(reason ReconnectReason, retryAfterMs int) (*Notification, error) {
	return NewNotification(NotifyReconnect, &ReconnectParams{
		Reason:       reason,
		RetryAfterMs: retryAfterMs,
	})
}

// NewRecordingStartedNotification creates a recordingStarted notification.
func NewRecordingStartedNotification(recordingID, startedBy string) (*Notification, error) {
	return NewNotification(NotifyRecordingStarted, &RecordingStartedParams{
		RecordingID: recordingID,
		StartedBy:   startedBy,
	})
}

// NewRecordingStoppedNotification creates a recordingStopped notification.
func NewRecordingStoppedNotification(recordingID, stoppedBy string) (*Notification, error) {
	return NewNotification(NotifyRecordingStopped, &RecordingStoppedParams{
		RecordingID: recordingID,
		StoppedBy:   stoppedBy,
	})
}

// NewTrackSubscribedNotification creates a trackSubscribed notification.
func NewTrackSubscribedNotification(subscriptionID, publisherID, trackID string, kind TrackKind) (*Notification, error) {
	return NewNotification(NotifyTrackSubscribed, &TrackSubscribedParams{
		SubscriptionID: subscriptionID,
		PublisherID:    publisherID,
		TrackID:        trackID,
		Kind:           kind,
	})
}

// NewTrackSubscriptionFailedNotification creates a trackSubscriptionFailed notification.
func NewTrackSubscriptionFailedNotification(publisherID, trackID string, errorCode int, errorMsg string) (*Notification, error) {
	return NewNotification(NotifyTrackSubscriptionFail, &TrackSubscriptionFailedParams{
		PublisherID: publisherID,
		TrackID:     trackID,
		ErrorCode:   errorCode,
		ErrorMsg:    errorMsg,
	})
}

// NewConnectionQualityChangedNotification creates a connectionQualityChanged notification.
func NewConnectionQualityChangedNotification(participantID string, quality ConnectionQuality, score float64) (*Notification, error) {
	return NewNotification(NotifyConnectionQuality, &ConnectionQualityChangedParams{
		ParticipantID: participantID,
		Quality:       quality,
		Score:         score,
	})
}

// NewServerStateChangedNotification creates a serverStateChanged notification.
func NewServerStateChangedNotification(roomID string, state ServerState, message string) (*Notification, error) {
	return NewNotification(NotifyServerState, &ServerStateChangedParams{
		RoomID:  roomID,
		State:   state,
		Message: message,
	})
}

// ParseNotification parses a JSON-RPC message as a notification.
func ParseNotification(data []byte) (*Notification, error) {
	var notif Notification
	if err := json.Unmarshal(data, &notif); err != nil {
		return nil, NewParseError(err.Error())
	}

	// Validate JSON-RPC version
	if notif.JSONRPC != Version {
		return nil, NewInvalidRequestError("invalid JSON-RPC version")
	}

	// Validate required fields
	if notif.Method == "" {
		return nil, NewInvalidRequestError("missing method")
	}

	return &notif, nil
}
