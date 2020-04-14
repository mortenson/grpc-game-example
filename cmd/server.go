package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/mortenson/grpc-game-example/pkg/server"
	"github.com/mortenson/grpc-game-example/proto"

	"google.golang.org/grpc"
)

func main() {
	port := *flag.Int("port", 8888, "The port to listen on.")
	log.Printf("listening on port %d", port)
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	game := backend.NewGame()
	game.Start()

	s := grpc.NewServer()
	server := server.NewGameServer(game)
	proto.RegisterGameServer(s, server)

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
