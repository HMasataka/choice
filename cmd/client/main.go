package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/HMasataka/choice/payload/handshake"
	"github.com/gorilla/rpc/v2/json2"
	"github.com/jessevdk/go-flags"
	"github.com/pion/webrtc/v4"
)

const (
	defaultServerURL = "http://localhost:8081"
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

type Options struct {
	Server string `long:"server" description:"Server URL" default:"http://localhost:8081"`
}

type JoinCommand struct {
	SessionID string `long:"session-id" description:"Session ID" required:"true"`
	UserID    string `long:"user-id" description:"User ID" required:"true"`
	Offer     string `long:"offer" description:"Offer JSON" required:"true"`
}

func (cmd *JoinCommand) Execute(args []string) error {
	client := NewClient(opts.Server)

	var offer webrtc.SessionDescription
	if err := json.Unmarshal([]byte(cmd.Offer), &offer); err != nil {
		return fmt.Errorf("invalid offer JSON: %w", err)
	}

	resp, err := client.Join(cmd.SessionID, cmd.UserID, &offer)
	if err != nil {
		return fmt.Errorf("join failed: %w", err)
	}

	respJSON, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling response: %w", err)
	}
	fmt.Printf("Response: %s\n", respJSON)
	return nil
}

type OfferCommand struct {
	Offer string `long:"offer" description:"Offer JSON" required:"true"`
}

func (cmd *OfferCommand) Execute(args []string) error {
	client := NewClient(opts.Server)

	var offer webrtc.SessionDescription
	if err := json.Unmarshal([]byte(cmd.Offer), &offer); err != nil {
		return fmt.Errorf("invalid offer JSON: %w", err)
	}

	resp, err := client.Offer(&offer)
	if err != nil {
		return fmt.Errorf("offer failed: %w", err)
	}

	respJSON, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling response: %w", err)
	}
	fmt.Printf("Response: %s\n", respJSON)
	return nil
}

type AnswerCommand struct {
	Answer string `long:"answer" description:"Answer JSON" required:"true"`
}

func (cmd *AnswerCommand) Execute(args []string) error {
	client := NewClient(opts.Server)

	var answer webrtc.SessionDescription
	if err := json.Unmarshal([]byte(cmd.Answer), &answer); err != nil {
		return fmt.Errorf("invalid answer JSON: %w", err)
	}

	if err := client.Answer(&answer); err != nil {
		return fmt.Errorf("answer failed: %w", err)
	}

	fmt.Println("Answer sent successfully")
	return nil
}

type CandidateCommand struct {
	Type      string `long:"type" description:"Connection type (publisher or subscriber)" required:"true"`
	Candidate string `long:"candidate" description:"Candidate JSON" required:"true"`
}

func (cmd *CandidateCommand) Execute(args []string) error {
	client := NewClient(opts.Server)

	var candidate webrtc.ICECandidateInit
	if err := json.Unmarshal([]byte(cmd.Candidate), &candidate); err != nil {
		return fmt.Errorf("invalid candidate JSON: %w", err)
	}

	if err := client.Candidate(handshake.ConnectionType(cmd.Type), candidate); err != nil {
		return fmt.Errorf("candidate failed: %w", err)
	}

	fmt.Println("Candidate sent successfully")
	return nil
}

var opts Options

func main() {
	parser := flags.NewParser(&opts, flags.Default)
	parser.AddCommand("join", "Join a session", "", &JoinCommand{})
	parser.AddCommand("offer", "Send offer", "", &OfferCommand{})
	parser.AddCommand("answer", "Send answer", "", &AnswerCommand{})
	parser.AddCommand("candidate", "Add ICE candidate", "", &CandidateCommand{})

	_, err := parser.Parse()
	if err != nil {
		log.Fatal(err)
	}
}
