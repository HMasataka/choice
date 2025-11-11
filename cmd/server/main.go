package main

import (
	"log"
	"net/http"

	"github.com/gorilla/rpc/v2"
	"github.com/gorilla/rpc/v2/json2"
)

type Args struct{}

type Reply struct {
	Message string
}

type MathArgs struct {
	A int `json:"a"`
	B int `json:"b"`
}

type MathReply struct {
	Result int `json:"result"`
}

type HelloService struct{}

func (h *HelloService) SayHello(r *http.Request, args *Args, reply *Reply) error {
	reply.Message = "hello world"
	return nil
}

func (h *HelloService) AddNumbers(r *http.Request, args *MathArgs, reply *MathReply) error {
	reply.Result = args.A + args.B
	return nil
}

func main() {
	server := rpc.NewServer()
	server.RegisterCodec(json2.NewCodec(), "application/json")

	service := &HelloService{}
	server.RegisterService(service, "")

	mux := http.NewServeMux()
	mux.Handle("/", server)

	log.Println("Starting gorilla JSON-RPC server on :8081")
	if err := http.ListenAndServe(":8081", mux); err != nil {
		log.Fatal(err)
	}
}
