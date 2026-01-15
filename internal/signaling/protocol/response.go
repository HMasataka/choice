package protocol

import (
	"encoding/json"
)

// Response represents a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// NewSuccessResponse creates a new successful JSON-RPC response.
func NewSuccessResponse(id string, result interface{}) (*Response, error) {
	var rawResult json.RawMessage
	if result != nil {
		data, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		rawResult = data
	} else {
		rawResult = []byte("{}")
	}

	return &Response{
		JSONRPC: Version,
		ID:      id,
		Result:  rawResult,
	}, nil
}

// NewErrorResponse creates a new error JSON-RPC response.
func NewErrorResponse(id string, err *Error) *Response {
	return &Response{
		JSONRPC: Version,
		ID:      id,
		Error:   err,
	}
}

// IsSuccess returns true if the response is successful.
func (r *Response) IsSuccess() bool {
	return r.Error == nil && r.Result != nil
}

// IsError returns true if the response is an error.
func (r *Response) IsError() bool {
	return r.Error != nil
}

// UnmarshalResult unmarshals the result into the given struct.
func (r *Response) UnmarshalResult(v interface{}) error {
	if r.Result == nil {
		return nil
	}
	return json.Unmarshal(r.Result, v)
}

// JoinResult represents the result of a successful join.
type JoinResult struct {
	SessionID     string            `json:"sessionId"`
	RoomID        string            `json:"roomId"`
	ParticipantID string            `json:"participantId"`
	Participants  []ParticipantInfo `json:"participants"`
	IceServers    []IceServer       `json:"iceServers"`
}

// LeaveResult represents the result of a successful leave.
type LeaveResult struct{}

// PublishResult represents the result of a successful publish.
type PublishResult struct {
	TrackID string `json:"trackId"`
	Mid     string `json:"mid"`
}

// UnpublishResult represents the result of a successful unpublish.
type UnpublishResult struct{}

// SubscribeResult represents the result of a successful subscribe.
type SubscribeResult struct {
	SubscriptionID string `json:"subscriptionId"`
	TrackID        string `json:"trackId"`
	PublisherID    string `json:"publisherId"`
}

// UnsubscribeResult represents the result of a successful unsubscribe.
type UnsubscribeResult struct{}

// SetPreferredLayerResult represents the result of a successful setPreferredLayer.
type SetPreferredLayerResult struct{}

// OfferResult represents the result of a successful offer.
type OfferResult struct {
	SDP string `json:"sdp"`
}

// AnswerResult represents the result of a successful answer.
type AnswerResult struct{}

// CandidateResult represents the result of a successful candidate.
type CandidateResult struct{}

// NewJoinResponse creates a new join response.
func NewJoinResponse(id string, result *JoinResult) (*Response, error) {
	return NewSuccessResponse(id, result)
}

// NewLeaveResponse creates a new leave response.
func NewLeaveResponse(id string) (*Response, error) {
	return NewSuccessResponse(id, &LeaveResult{})
}

// NewPublishResponse creates a new publish response.
func NewPublishResponse(id string, result *PublishResult) (*Response, error) {
	return NewSuccessResponse(id, result)
}

// NewUnpublishResponse creates a new unpublish response.
func NewUnpublishResponse(id string) (*Response, error) {
	return NewSuccessResponse(id, &UnpublishResult{})
}

// NewSubscribeResponse creates a new subscribe response.
func NewSubscribeResponse(id string, result *SubscribeResult) (*Response, error) {
	return NewSuccessResponse(id, result)
}

// NewUnsubscribeResponse creates a new unsubscribe response.
func NewUnsubscribeResponse(id string) (*Response, error) {
	return NewSuccessResponse(id, &UnsubscribeResult{})
}

// NewSetPreferredLayerResponse creates a new setPreferredLayer response.
func NewSetPreferredLayerResponse(id string) (*Response, error) {
	return NewSuccessResponse(id, &SetPreferredLayerResult{})
}

// NewOfferResponse creates a new offer response.
func NewOfferResponse(id string, sdp string) (*Response, error) {
	return NewSuccessResponse(id, &OfferResult{SDP: sdp})
}

// NewAnswerResponse creates a new answer response.
func NewAnswerResponse(id string) (*Response, error) {
	return NewSuccessResponse(id, &AnswerResult{})
}

// NewCandidateResponse creates a new candidate response.
func NewCandidateResponse(id string) (*Response, error) {
	return NewSuccessResponse(id, &CandidateResult{})
}

// Marshal marshals the response to JSON.
func (r *Response) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

// ParseResponse parses a JSON-RPC message as a response.
func ParseResponse(data []byte) (*Response, error) {
	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, NewParseError(err.Error())
	}

	// Validate JSON-RPC version
	if resp.JSONRPC != Version {
		return nil, NewInvalidRequestError("invalid JSON-RPC version")
	}

	// Validate required fields
	if resp.ID == "" {
		return nil, NewInvalidRequestError("missing response id")
	}
	if !isValidUUID(resp.ID) {
		return nil, NewInvalidRequestError("invalid response id format")
	}

	// Must have either result or error
	if resp.Result == nil && resp.Error == nil {
		return nil, NewInvalidRequestError("response must have result or error")
	}

	// Cannot have both result and error
	if resp.Result != nil && resp.Error != nil {
		return nil, NewInvalidRequestError("response cannot have both result and error")
	}

	return &resp, nil
}
