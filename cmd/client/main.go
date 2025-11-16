package main

import (
	"log"

	"github.com/HMasataka/choice/cmd/client/handler"
	"github.com/jessevdk/go-flags"
)

type Options struct{}

var opts Options

func main() {
	parser := flags.NewParser(&opts, flags.Default)
	parser.AddCommand("join", "Join a session", "", handler.NewJoinCommand())
	parser.AddCommand("offer", "Send offer", "", handler.NewOfferCommand())
	parser.AddCommand("answer", "Send answer", "", handler.NewAnswerCommand())
	parser.AddCommand("candidate", "Add ICE candidate", "", handler.NewCandidateCommand())

	_, err := parser.Parse()
	if err != nil {
		log.Fatal(err)
	}
}
