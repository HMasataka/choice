package handler

import (
	"encoding/json"
	"fmt"

	"github.com/HMasataka/choice/cmd/client/lib"
	"github.com/pion/webrtc/v4"
)

type JoinCommand struct {
	ServerURL string `long:"server" description:"Server URL" default:"http://localhost:8081"`
	SessionID string `long:"session-id" description:"Session ID" required:"true"`
	UserID    string `long:"user-id" description:"User ID" required:"true"`
	Offer     string `long:"offer" description:"Offer JSON" required:"true"`
}

func NewJoinCommand() *JoinCommand {
	return &JoinCommand{}
}

func (cmd *JoinCommand) Execute(args []string) error {
	c := client.NewClient(cmd.ServerURL)

	var offer webrtc.SessionDescription
	if err := json.Unmarshal([]byte(cmd.Offer), &offer); err != nil {
		return fmt.Errorf("invalid offer JSON: %w", err)
	}

	resp, err := c.Join(cmd.SessionID, cmd.UserID, &offer)
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
