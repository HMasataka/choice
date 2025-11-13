package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/HMasataka/choice/payload/handshake"
	"github.com/gorilla/rpc/v2/json2"
	"github.com/pion/webrtc/v4"
)

type Client struct {
	serverURL string
	client    *http.Client
}

func NewClient(serverURL string) *Client {
	return &Client{
		serverURL: serverURL,
		client:    &http.Client{},
	}
}

func (c *Client) call(method string, params any) (any, error) {
	body, err := json2.EncodeClientRequest(method, []any{params})
	if err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}

	resp, err := c.client.Post(
		c.serverURL,
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var result any
	if err := json2.DecodeClientResponse(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

func (c *Client) Join(sessionID, userID string, offer *webrtc.SessionDescription) (*handshake.JoinResponse, error) {
	req := handshake.JoinRequest{
		SessionID: sessionID,
		UserID:    userID,
		Offer:     *offer,
	}

	result, err := c.call("SignalingServer.Join", req)
	if err != nil {
		return nil, err
	}

	resultJSON, _ := json.Marshal(result)
	var resp handshake.JoinResponse
	if err := json.Unmarshal(resultJSON, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal join response: %w", err)
	}

	return &resp, nil
}

func (c *Client) Offer(offer *webrtc.SessionDescription) (*handshake.OfferResponse, error) {
	req := handshake.OfferRequest{
		Offer: *offer,
	}

	result, err := c.call("SignalingServer.Offer", req)
	if err != nil {
		return nil, err
	}

	resultJSON, _ := json.Marshal(result)
	var resp handshake.OfferResponse
	if err := json.Unmarshal(resultJSON, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal offer response: %w", err)
	}

	return &resp, nil
}

func (c *Client) Answer(answer *webrtc.SessionDescription) error {
	req := handshake.AnswerRequest{
		Answer: *answer,
	}

	_, err := c.call("SignalingServer.Answer", req)
	return err
}

func (c *Client) Candidate(connType handshake.ConnectionType, candidate webrtc.ICECandidateInit) error {
	req := handshake.CandidateRequest{
		ConnectionType: connType,
		Candidate:      candidate,
	}

	_, err := c.call("SignalingServer.Candidate", req)

	return err
}
