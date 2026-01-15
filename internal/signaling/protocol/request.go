package protocol

import (
	"encoding/json"
)

// Method names for JSON-RPC requests.
const (
	MethodJoin              = "join"
	MethodLeave             = "leave"
	MethodPublish           = "publish"
	MethodUnpublish         = "unpublish"
	MethodSubscribe         = "subscribe"
	MethodUnsubscribe       = "unsubscribe"
	MethodSetPreferredLayer = "setPreferredLayer"
	MethodOffer             = "offer"
	MethodAnswer            = "answer"
	MethodCandidate         = "candidate"
)

// Request represents a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// NewRequest creates a new JSON-RPC request.
func NewRequest(id, method string, params interface{}) (*Request, error) {
	var rawParams json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		rawParams = data
	}

	return &Request{
		JSONRPC: Version,
		ID:      id,
		Method:  method,
		Params:  rawParams,
	}, nil
}

// UnmarshalParams unmarshals the params into the given struct.
func (r *Request) UnmarshalParams(v interface{}) error {
	if r.Params == nil {
		return nil
	}
	return json.Unmarshal(r.Params, v)
}

// JoinParams represents parameters for the join method.
type JoinParams struct {
	Token     string                 `json:"token"`
	SessionID string                 `json:"sessionId,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// LeaveParams represents parameters for the leave method.
type LeaveParams struct{}

// PublishParams represents parameters for the publish method.
type PublishParams struct {
	Kind      TrackKind              `json:"kind"`
	Simulcast bool                   `json:"simulcast,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Label     string                 `json:"label,omitempty"`
}

// UnpublishParams represents parameters for the unpublish method.
type UnpublishParams struct {
	TrackID string `json:"trackId"`
}

// SubscribeParams represents parameters for the subscribe method.
type SubscribeParams struct {
	PublisherID    string         `json:"publisherId"`
	TrackID        string         `json:"trackId"`
	PreferredLayer SimulcastLayer `json:"preferredLayer,omitempty"`
}

// UnsubscribeParams represents parameters for the unsubscribe method.
type UnsubscribeParams struct {
	SubscriptionID string `json:"subscriptionId"`
}

// SetPreferredLayerParams represents parameters for the setPreferredLayer method.
type SetPreferredLayerParams struct {
	TrackID string         `json:"trackId"`
	Layer   SimulcastLayer `json:"layer"`
}

// OfferParams represents parameters for the offer method.
type OfferParams struct {
	SDP string `json:"sdp"`
}

// AnswerParams represents parameters for the answer method.
type AnswerParams struct {
	SDP string `json:"sdp"`
}

// CandidateParams represents parameters for the candidate method.
type CandidateParams struct {
	Candidate     string `json:"candidate"`
	SDPMid        string `json:"sdpMid,omitempty"`
	SDPMLineIndex *int   `json:"sdpMLineIndex,omitempty"`
}

// ParseRequest parses a JSON-RPC message as a request.
func ParseRequest(data []byte) (*Request, error) {
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, NewParseError(err.Error())
	}

	// Validate JSON-RPC version
	if req.JSONRPC != Version {
		return nil, NewInvalidRequestError("invalid JSON-RPC version")
	}

	// Validate required fields
	if req.ID == "" {
		return nil, NewInvalidRequestError("missing request id")
	}
	if !isValidUUID(req.ID) {
		return nil, NewInvalidRequestError("invalid request id format")
	}
	if req.Method == "" {
		return nil, NewInvalidRequestError("missing method")
	}
	if !isValidRequestMethod(req.Method) {
		return nil, NewInvalidRequestError("invalid method")
	}

	return &req, nil
}

// ValidateJoinParams validates join parameters.
func ValidateJoinParams(params *JoinParams) *Error {
	if params.Token == "" {
		return NewInvalidParamsError("token is required")
	}
	return nil
}

// ValidatePublishParams validates publish parameters.
func ValidatePublishParams(params *PublishParams) *Error {
	if params.Kind != TrackKindVideo && params.Kind != TrackKindAudio {
		return NewInvalidParamsError("kind must be 'video' or 'audio'")
	}
	return nil
}

// ValidateUnpublishParams validates unpublish parameters.
func ValidateUnpublishParams(params *UnpublishParams) *Error {
	if params.TrackID == "" {
		return NewInvalidParamsError("trackId is required")
	}
	return nil
}

// ValidateSubscribeParams validates subscribe parameters.
func ValidateSubscribeParams(params *SubscribeParams) *Error {
	if params.PublisherID == "" {
		return NewInvalidParamsError("publisherId is required")
	}
	if params.TrackID == "" {
		return NewInvalidParamsError("trackId is required")
	}
	if params.PreferredLayer != "" &&
		params.PreferredLayer != SimulcastLayerHigh &&
		params.PreferredLayer != SimulcastLayerMedium &&
		params.PreferredLayer != SimulcastLayerLow {
		return NewInvalidParamsError("preferredLayer must be 'h', 'm', or 'l'")
	}
	return nil
}

// ValidateUnsubscribeParams validates unsubscribe parameters.
func ValidateUnsubscribeParams(params *UnsubscribeParams) *Error {
	if params.SubscriptionID == "" {
		return NewInvalidParamsError("subscriptionId is required")
	}
	return nil
}

// ValidateSetPreferredLayerParams validates setPreferredLayer parameters.
func ValidateSetPreferredLayerParams(params *SetPreferredLayerParams) *Error {
	if params.TrackID == "" {
		return NewInvalidParamsError("trackId is required")
	}
	if params.Layer != SimulcastLayerHigh && params.Layer != SimulcastLayerMedium && params.Layer != SimulcastLayerLow {
		return NewInvalidParamsError("layer must be 'h', 'm', or 'l'")
	}
	return nil
}

// ValidateOfferParams validates offer parameters.
func ValidateOfferParams(params *OfferParams) *Error {
	if params.SDP == "" {
		return NewInvalidParamsError("sdp is required")
	}
	return nil
}

// ValidateAnswerParams validates answer parameters.
func ValidateAnswerParams(params *AnswerParams) *Error {
	if params.SDP == "" {
		return NewInvalidParamsError("sdp is required")
	}
	return nil
}

// ValidateCandidateParams validates candidate parameters.
func ValidateCandidateParams(params *CandidateParams) *Error {
	if params.Candidate == "" {
		return NewInvalidParamsError("candidate is required")
	}
	if params.SDPMLineIndex != nil && *params.SDPMLineIndex < 0 {
		return NewInvalidParamsError("sdpMLineIndex must be >= 0")
	}
	return nil
}
