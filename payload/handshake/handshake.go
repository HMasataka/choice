package handshake

import "github.com/pion/webrtc/v4"

type JoinRequest struct {
	SessionID string                    `json:"session_id"`
	UserID    string                    `json:"user_id"`
	Offer     webrtc.SessionDescription `json:"offer"`
}

type JoinResponse struct {
	Answer *webrtc.SessionDescription `json:"answer"`
}

type OfferRequest struct {
	Offer webrtc.SessionDescription `json:"offer"`
}

type OfferResponse struct {
	Answer *webrtc.SessionDescription `json:"answer"`
}

type AnswerRequest struct {
	SessionID string                    `json:"session_id"`
	UserID    string                    `json:"user_id"`
	Answer    webrtc.SessionDescription `json:"answer"`
}

type AnswerResponse struct{}

type ConnectionType string

const (
	ConnectionTypePublisher  ConnectionType = "publisher"
	ConnectionTypeSubscriber ConnectionType = "subscriber"
)

type CandidateRequest struct {
	SessionID      string                  `json:"session_id"`
	UserID         string                  `json:"user_id"`
	ConnectionType ConnectionType          `json:"connection_type"`
	Candidate      webrtc.ICECandidateInit `json:"candidate"`
}

type CandidateResponse struct{}
