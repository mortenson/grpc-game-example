package main

import (
	"log"
	"net"

	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/mortenson/grpc-game-example/pkg/server"
	"github.com/mortenson/grpc-game-example/proto"

	"google.golang.org/grpc"
)

func main() {
	lis, err := net.Listen("tcp", ":8888")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	game := backend.NewGame()
	game.Start()

	s := grpc.NewServer()
	server := server.NewGameServer(&game)
	proto.RegisterGameServer(s, server)

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
