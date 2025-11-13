package handler

import (
	"encoding/json"
	"fmt"

	"github.com/HMasataka/choice/cmd/client/lib"
	"github.com/pion/webrtc/v4"
)

type AnswerCommand struct {
	ServerURL string `long:"server" description:"Server URL" default:"http://localhost:8081"`
	Answer    string `long:"answer" description:"Answer JSON" required:"true"`
}

func NewAnswerCommand() *AnswerCommand {
	return &AnswerCommand{}
}

func (cmd *AnswerCommand) Execute(args []string) error {
	c := client.NewClient(cmd.ServerURL)

	var answer webrtc.SessionDescription
	if err := json.Unmarshal([]byte(cmd.Answer), &answer); err != nil {
		return fmt.Errorf("invalid answer JSON: %w", err)
	}

	if err := c.Answer(&answer); err != nil {
		return fmt.Errorf("answer failed: %w", err)
	}

	fmt.Println("Answer sent successfully")

	return nil
}
