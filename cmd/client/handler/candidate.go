package handler

import (
	"encoding/json"
	"fmt"

	"github.com/HMasataka/choice/cmd/client/lib"
	"github.com/HMasataka/choice/payload/handshake"
	"github.com/pion/webrtc/v4"
)

type CandidateCommand struct {
	BaseCommand
	Type      string `long:"type" description:"Connection type (publisher or subscriber)" required:"true"`
	Candidate string `long:"candidate" description:"Candidate JSON" required:"true"`
}

func NewCandidateCommand() *CandidateCommand {
	return &CandidateCommand{}
}

func (cmd *CandidateCommand) Execute(args []string) error {
	c := lib.NewClient(cmd.ServerURL)

	var candidate webrtc.ICECandidateInit
	if err := json.Unmarshal([]byte(cmd.Candidate), &candidate); err != nil {
		return fmt.Errorf("invalid candidate JSON: %w", err)
	}

	if err := c.Candidate(handshake.ConnectionType(cmd.Type), candidate); err != nil {
		return fmt.Errorf("candidate failed: %w", err)
	}

	fmt.Println("Candidate sent successfully")

	return nil
}
