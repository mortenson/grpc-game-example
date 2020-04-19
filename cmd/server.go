package main

// Runs a game server.

import (
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/mortenson/grpc-game-example/pkg/bot"
	"github.com/mortenson/grpc-game-example/pkg/server"
	"github.com/mortenson/grpc-game-example/proto"

	"google.golang.org/grpc"
)

func main() {
	port := flag.Int("port", 8888, "The port to listen on.")
	password := flag.String("password", "", "The server password.")
	numBots := flag.Int("bots", 0, "The number of bots to add to the server.")
	flag.Parse()

	log.Printf("listening on port %d", *port)
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	game := backend.NewGame()

	bots := bot.NewBots(game)
	for i := 0; i < *numBots; i++ {
		bots.AddBot(fmt.Sprintf("Bob %d", i))
	}

	game.Start()
	bots.Start()

	s := grpc.NewServer()
	server := server.NewGameServer(game, *password)
	proto.RegisterGameServer(s, server)

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
