package handler

type BaseCommand struct {
	ServerURL string `long:"server" description:"Server URL" default:"http://localhost:8081"`
}
