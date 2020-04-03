package main

import (
	"context"
	"io"
	"log"

	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/mortenson/grpc-game-example/pkg/frontend"
	"github.com/mortenson/grpc-game-example/proto"
	"google.golang.org/grpc"
)

func main() {
	game := backend.NewGame()
	view := frontend.NewView(&game)
	game.Start()

	playerName := "Bob"

	conn, err := grpc.Dial(":8888", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("can not connect with server %v", err)
	}

	client := proto.NewGameClient(conn)
	stream, err := client.Stream(context.Background())
	if err != nil {
		log.Fatalf("openn stream error %v", err)
	}
	req := proto.Request{
		Action: &proto.Request_Connect{
			Connect: &proto.Connect{
				Player: playerName,
			},
		},
	}
	stream.Send(&req)
	//ctx := stream.Context()

	go func() {
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				log.Fatalf("EOF")
				return
			}
			if err != nil {
				log.Fatalf("can not receive %v", err)
			}
			init := resp.GetInitialize()
			if init != nil {
				currentPlayer := backend.Player{
					Position: backend.Coordinate{
						X: int(init.Position.X),
						Y: int(init.Position.Y),
					},
					Name:      playerName,
					Direction: backend.DirectionStop,
					Icon:      'P',
				}
				game.Mux.Lock()
				game.Players[playerName] = &currentPlayer
				game.Mux.Unlock()
				view.CurrentPlayer = &currentPlayer
			}
		}
	}()

	view.Start()
}
