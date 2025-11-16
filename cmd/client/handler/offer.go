package handler

import (
	"encoding/json"
	"fmt"

	"github.com/HMasataka/choice/cmd/client/lib"
	"github.com/pion/webrtc/v4"
)

type OfferCommand struct {
	BaseCommand
	Offer string `long:"offer" description:"Offer JSON" required:"true"`
}

func NewOfferCommand() *OfferCommand {
	return &OfferCommand{}
}

func (cmd *OfferCommand) Execute(args []string) error {
	c := lib.NewClient(cmd.ServerURL)

	var offer webrtc.SessionDescription
	if err := json.Unmarshal([]byte(cmd.Offer), &offer); err != nil {
		return fmt.Errorf("invalid offer JSON: %w", err)
	}

	resp, err := c.Offer(&offer)
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
