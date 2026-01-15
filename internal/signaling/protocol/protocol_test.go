package protocol

import (
	"encoding/json"
	"testing"
)

func TestParseMessage(t *testing.T) {
	validUUID := "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
	tests := []struct {
		name           string
		input          string
		wantErr        bool
		isRequest      bool
		isResponse     bool
		isNotification bool
	}{
		{
			name:      "valid request with UUID",
			input:     `{"jsonrpc":"2.0","id":"` + validUUID + `","method":"join","params":{"token":"abc"}}`,
			wantErr:   false,
			isRequest: true,
		},
		{
			name:       "valid success response with UUID",
			input:      `{"jsonrpc":"2.0","id":"` + validUUID + `","result":{"sessionId":"456"}}`,
			wantErr:    false,
			isResponse: true,
		},
		{
			name:       "valid error response with UUID",
			input:      `{"jsonrpc":"2.0","id":"` + validUUID + `","error":{"code":-32600,"message":"Invalid Request"}}`,
			wantErr:    false,
			isResponse: true,
		},
		{
			name:           "valid notification",
			input:          `{"jsonrpc":"2.0","method":"participantJoined","params":{"participantId":"p1"}}`,
			wantErr:        false,
			isNotification: true,
		},
		{
			name:    "invalid JSON",
			input:   `{invalid}`,
			wantErr: true,
		},
		{
			name:    "wrong version",
			input:   `{"jsonrpc":"1.0","id":"` + validUUID + `","method":"join"}`,
			wantErr: true,
		},
		{
			name:    "invalid UUID format",
			input:   `{"jsonrpc":"2.0","id":"not-a-uuid","method":"join"}`,
			wantErr: true,
		},
		{
			name:    "response with both result and error",
			input:   `{"jsonrpc":"2.0","id":"` + validUUID + `","result":{},"error":{"code":-32600,"message":"error"}}`,
			wantErr: true,
		},
		{
			name:    "response with neither result nor error",
			input:   `{"jsonrpc":"2.0","id":"` + validUUID + `"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseMessage([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if msg.IsRequest() != tt.isRequest {
				t.Errorf("IsRequest() = %v, want %v", msg.IsRequest(), tt.isRequest)
			}
			if msg.IsResponse() != tt.isResponse {
				t.Errorf("IsResponse() = %v, want %v", msg.IsResponse(), tt.isResponse)
			}
			if msg.IsNotification() != tt.isNotification {
				t.Errorf("IsNotification() = %v, want %v", msg.IsNotification(), tt.isNotification)
			}
		})
	}
}

func TestError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{
			name: "error without data",
			err:  &Error{Code: -32600, Message: "Invalid Request"},
			want: "JSON-RPC error -32600: Invalid Request",
		},
		{
			name: "error with data",
			err:  &Error{Code: -32600, Message: "Invalid Request", Data: map[string]interface{}{"details": "missing field"}},
			want: "JSON-RPC error -32600: Invalid Request (data: map[details:missing field])",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error.Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewRequest(t *testing.T) {
	validUUID := "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
	params := &JoinParams{Token: "test-token"}
	req, err := NewRequest(validUUID, MethodJoin, params)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	if req.JSONRPC != Version {
		t.Errorf("JSONRPC = %v, want %v", req.JSONRPC, Version)
	}
	if req.ID != validUUID {
		t.Errorf("ID = %v, want %v", req.ID, validUUID)
	}
	if req.Method != MethodJoin {
		t.Errorf("Method = %v, want %v", req.Method, MethodJoin)
	}

	var gotParams JoinParams
	if err := req.UnmarshalParams(&gotParams); err != nil {
		t.Fatalf("UnmarshalParams() error = %v", err)
	}
	if gotParams.Token != "test-token" {
		t.Errorf("Token = %v, want test-token", gotParams.Token)
	}
}

func TestParseRequest_Validation(t *testing.T) {
	validUUID := "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid request",
			input:   `{"jsonrpc":"2.0","id":"` + validUUID + `","method":"join","params":{"token":"abc"}}`,
			wantErr: false,
		},
		{
			name:    "invalid UUID",
			input:   `{"jsonrpc":"2.0","id":"invalid-uuid","method":"join"}`,
			wantErr: true,
		},
		{
			name:    "invalid method",
			input:   `{"jsonrpc":"2.0","id":"` + validUUID + `","method":"unknownMethod"}`,
			wantErr: true,
		},
		{
			name:    "missing method",
			input:   `{"jsonrpc":"2.0","id":"` + validUUID + `"}`,
			wantErr: true,
		},
		{
			name:    "missing id",
			input:   `{"jsonrpc":"2.0","method":"join"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRequest([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewSuccessResponse(t *testing.T) {
	validUUID := "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
	result := &JoinResult{
		SessionID:     "session-1",
		RoomID:        "room-1",
		ParticipantID: "participant-1",
		Participants:  []ParticipantInfo{},
		IceServers:    []IceServer{},
	}

	resp, err := NewSuccessResponse(validUUID, result)
	if err != nil {
		t.Fatalf("NewSuccessResponse() error = %v", err)
	}

	if resp.JSONRPC != Version {
		t.Errorf("JSONRPC = %v, want %v", resp.JSONRPC, Version)
	}
	if resp.ID != validUUID {
		t.Errorf("ID = %v, want %v", resp.ID, validUUID)
	}
	if resp.Error != nil {
		t.Errorf("Error = %v, want nil", resp.Error)
	}
	if !resp.IsSuccess() {
		t.Error("IsSuccess() = false, want true")
	}

	var gotResult JoinResult
	if err := resp.UnmarshalResult(&gotResult); err != nil {
		t.Fatalf("UnmarshalResult() error = %v", err)
	}
	if gotResult.SessionID != "session-1" {
		t.Errorf("SessionID = %v, want session-1", gotResult.SessionID)
	}
}

func TestNewErrorResponse(t *testing.T) {
	validUUID := "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
	err := NewRoomNotFoundError("room-123")
	resp := NewErrorResponse(validUUID, err)

	if resp.JSONRPC != Version {
		t.Errorf("JSONRPC = %v, want %v", resp.JSONRPC, Version)
	}
	if resp.ID != validUUID {
		t.Errorf("ID = %v, want %v", resp.ID, validUUID)
	}
	if resp.Result != nil {
		t.Errorf("Result = %v, want nil", resp.Result)
	}
	if !resp.IsError() {
		t.Error("IsError() = false, want true")
	}
	if resp.Error.Code != CodeRoomNotFound {
		t.Errorf("Error.Code = %v, want %v", resp.Error.Code, CodeRoomNotFound)
	}
}

func TestNewNotification(t *testing.T) {
	notif, err := NewParticipantJoinedNotification("p1", map[string]interface{}{"name": "Alice"})
	if err != nil {
		t.Fatalf("NewParticipantJoinedNotification() error = %v", err)
	}

	if notif.JSONRPC != Version {
		t.Errorf("JSONRPC = %v, want %v", notif.JSONRPC, Version)
	}
	if notif.Method != NotifyParticipantJoined {
		t.Errorf("Method = %v, want %v", notif.Method, NotifyParticipantJoined)
	}

	var params ParticipantJoinedParams
	if err := notif.UnmarshalParams(&params); err != nil {
		t.Fatalf("UnmarshalParams() error = %v", err)
	}
	if params.ParticipantID != "p1" {
		t.Errorf("ParticipantID = %v, want p1", params.ParticipantID)
	}
}

func TestValidateParams(t *testing.T) {
	tests := []struct {
		name    string
		validate func() *Error
		wantErr bool
	}{
		{
			name: "valid join params",
			validate: func() *Error {
				return ValidateJoinParams(&JoinParams{Token: "token"})
			},
			wantErr: false,
		},
		{
			name: "invalid join params - missing token",
			validate: func() *Error {
				return ValidateJoinParams(&JoinParams{})
			},
			wantErr: true,
		},
		{
			name: "valid publish params - video",
			validate: func() *Error {
				return ValidatePublishParams(&PublishParams{Kind: TrackKindVideo})
			},
			wantErr: false,
		},
		{
			name: "valid publish params - audio",
			validate: func() *Error {
				return ValidatePublishParams(&PublishParams{Kind: TrackKindAudio})
			},
			wantErr: false,
		},
		{
			name: "invalid publish params - wrong kind",
			validate: func() *Error {
				return ValidatePublishParams(&PublishParams{Kind: "invalid"})
			},
			wantErr: true,
		},
		{
			name: "valid subscribe params",
			validate: func() *Error {
				return ValidateSubscribeParams(&SubscribeParams{
					PublisherID: "pub-1",
					TrackID:     "track-1",
				})
			},
			wantErr: false,
		},
		{
			name: "invalid subscribe params - missing publisherId",
			validate: func() *Error {
				return ValidateSubscribeParams(&SubscribeParams{TrackID: "track-1"})
			},
			wantErr: true,
		},
		{
			name: "valid setPreferredLayer params",
			validate: func() *Error {
				return ValidateSetPreferredLayerParams(&SetPreferredLayerParams{
					TrackID: "track-1",
					Layer:   SimulcastLayerHigh,
				})
			},
			wantErr: false,
		},
		{
			name: "invalid setPreferredLayer params - wrong layer",
			validate: func() *Error {
				return ValidateSetPreferredLayerParams(&SetPreferredLayerParams{
					TrackID: "track-1",
					Layer:   "x",
				})
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestErrorCodes(t *testing.T) {
	tests := []struct {
		code             int
		isStandardError  bool
		isApplicationError bool
	}{
		{CodeParseError, true, false},
		{CodeInvalidRequest, true, false},
		{CodeMethodNotFound, true, false},
		{CodeInvalidParams, true, false},
		{CodeInternalError, true, false},
		{CodeRoomNotFound, false, true},
		{CodeRoomFull, false, true},
		{CodeUnauthorized, false, true},
		{CodeAlreadyJoined, false, true},
		{CodeNotInRoom, false, true},
		{CodeTrackNotFound, false, true},
		{CodeInvalidSDP, false, true},
		{CodeICEFailure, false, true},
		{CodeSessionExpired, false, true},
		{9999, false, false},
	}

	for _, tt := range tests {
		if IsStandardError(tt.code) != tt.isStandardError {
			t.Errorf("IsStandardError(%d) = %v, want %v", tt.code, IsStandardError(tt.code), tt.isStandardError)
		}
		if IsApplicationError(tt.code) != tt.isApplicationError {
			t.Errorf("IsApplicationError(%d) = %v, want %v", tt.code, IsApplicationError(tt.code), tt.isApplicationError)
		}
	}
}

func TestMarshalResponse(t *testing.T) {
	validUUID := "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
	resp, _ := NewSuccessResponse(validUUID, &LeaveResult{})
	data, err := resp.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got["jsonrpc"] != Version {
		t.Errorf("jsonrpc = %v, want %v", got["jsonrpc"], Version)
	}
	if got["id"] != validUUID {
		t.Errorf("id = %v, want %v", got["id"], validUUID)
	}
	if got["error"] != nil {
		t.Errorf("error = %v, want nil", got["error"])
	}
}

func TestValidateParams_Extended(t *testing.T) {
	tests := []struct {
		name     string
		validate func() *Error
		wantErr  bool
	}{
		{
			name: "valid subscribe params with preferredLayer",
			validate: func() *Error {
				return ValidateSubscribeParams(&SubscribeParams{
					PublisherID:    "pub-1",
					TrackID:        "track-1",
					PreferredLayer: SimulcastLayerHigh,
				})
			},
			wantErr: false,
		},
		{
			name: "invalid subscribe params - wrong preferredLayer",
			validate: func() *Error {
				return ValidateSubscribeParams(&SubscribeParams{
					PublisherID:    "pub-1",
					TrackID:        "track-1",
					PreferredLayer: "invalid",
				})
			},
			wantErr: true,
		},
		{
			name: "valid candidate params with sdpMLineIndex",
			validate: func() *Error {
				idx := 0
				return ValidateCandidateParams(&CandidateParams{
					Candidate:     "candidate",
					SDPMLineIndex: &idx,
				})
			},
			wantErr: false,
		},
		{
			name: "invalid candidate params - negative sdpMLineIndex",
			validate: func() *Error {
				idx := -1
				return ValidateCandidateParams(&CandidateParams{
					Candidate:     "candidate",
					SDPMLineIndex: &idx,
				})
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMarshalNotification(t *testing.T) {
	notif, _ := NewErrorNotification(1001, "Room not found", true)
	data, err := notif.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got["jsonrpc"] != Version {
		t.Errorf("jsonrpc = %v, want %v", got["jsonrpc"], Version)
	}
	if got["method"] != NotifyError {
		t.Errorf("method = %v, want %v", got["method"], NotifyError)
	}
	if got["id"] != nil {
		t.Errorf("id = %v, want nil (notification should not have id)", got["id"])
	}
}

func TestAllNotificationCreators(t *testing.T) {
	tests := []struct {
		name   string
		create func() (*Notification, error)
		method string
	}{
		{
			name: "ParticipantJoined",
			create: func() (*Notification, error) {
				return NewParticipantJoinedNotification("p1", nil)
			},
			method: NotifyParticipantJoined,
		},
		{
			name: "ParticipantLeft",
			create: func() (*Notification, error) {
				return NewParticipantLeftNotification("p1", LeaveReasonLeave)
			},
			method: NotifyParticipantLeft,
		},
		{
			name: "TrackPublished",
			create: func() (*Notification, error) {
				return NewTrackPublishedNotification("p1", "t1", TrackKindVideo, true, nil)
			},
			method: NotifyTrackPublished,
		},
		{
			name: "TrackUnpublished",
			create: func() (*Notification, error) {
				return NewTrackUnpublishedNotification("p1", "t1")
			},
			method: NotifyTrackUnpublished,
		},
		{
			name: "Joined",
			create: func() (*Notification, error) {
				return NewJoinedNotification("p1", "r1")
			},
			method: NotifyJoined,
		},
		{
			name: "Left",
			create: func() (*Notification, error) {
				return NewLeftNotification(LeftReasonVoluntary)
			},
			method: NotifyLeft,
		},
		{
			name: "Offer",
			create: func() (*Notification, error) {
				return NewOfferNotification("sdp", OfferReasonTrackAdded)
			},
			method: NotifyOffer,
		},
		{
			name: "Candidate",
			create: func() (*Notification, error) {
				return NewCandidateNotification("candidate", "mid", nil)
			},
			method: NotifyCandidate,
		},
		{
			name: "Answer",
			create: func() (*Notification, error) {
				return NewAnswerNotification("sdp")
			},
			method: NotifyAnswer,
		},
		{
			name: "LayerChanged",
			create: func() (*Notification, error) {
				return NewLayerChangedNotification("t1", SimulcastLayerHigh, SimulcastLayerMedium, LayerChangeReasonBandwidth)
			},
			method: NotifyLayerChanged,
		},
		{
			name: "Error",
			create: func() (*Notification, error) {
				return NewErrorNotification(1001, "error", false)
			},
			method: NotifyError,
		},
		{
			name: "Reconnect",
			create: func() (*Notification, error) {
				return NewReconnectNotification(ReconnectReasonICEDisconnected, 1000)
			},
			method: NotifyReconnect,
		},
		{
			name: "RecordingStarted",
			create: func() (*Notification, error) {
				return NewRecordingStartedNotification("rec1", "p1")
			},
			method: NotifyRecordingStarted,
		},
		{
			name: "RecordingStopped",
			create: func() (*Notification, error) {
				return NewRecordingStoppedNotification("rec1", "p1")
			},
			method: NotifyRecordingStopped,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notif, err := tt.create()
			if err != nil {
				t.Fatalf("create() error = %v", err)
			}
			if notif.Method != tt.method {
				t.Errorf("Method = %v, want %v", notif.Method, tt.method)
			}
			if notif.JSONRPC != Version {
				t.Errorf("JSONRPC = %v, want %v", notif.JSONRPC, Version)
			}
		})
	}
}
