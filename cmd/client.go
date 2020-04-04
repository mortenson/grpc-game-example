package main

import (
	"context"
	"io"
	"log"
	"math/rand"
	"time"

	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/mortenson/grpc-game-example/pkg/frontend"
	"github.com/mortenson/grpc-game-example/proto"
	"google.golang.org/grpc"
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randSeq(n int) string {
	rand.Seed(time.Now().Unix())
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func main() {
	game := backend.NewGame()
	view := frontend.NewView(&game)
	game.Start()

	playerName := randSeq(6)

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
	// @todo
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
						X: init.Position.X,
						Y: init.Position.Y,
					},
					Name:      playerName,
					Direction: backend.DirectionStop,
					Icon:      'P',
				}
				game.Mux.Lock()
				game.Players[playerName] = &currentPlayer
				for _, player := range init.Players {
					game.Players[player.Player] = &backend.Player{
						Position: backend.Coordinate{
							X: player.Position.X,
							Y: player.Position.Y,
						},
						Name:      player.Player,
						Direction: backend.DirectionStop,
						Icon:      'P',
					}
				}
				game.Mux.Unlock()
				view.CurrentPlayer = &currentPlayer
			}
			add := resp.GetAddplayer()
			if add != nil {
				newPlayer := backend.Player{
					Position: backend.Coordinate{
						X: add.Position.X,
						Y: add.Position.Y,
					},
					Name:      resp.Player,
					Direction: backend.DirectionStop,
					Icon:      'P',
				}
				game.Mux.Lock()
				game.Players[resp.Player] = &newPlayer
				game.Mux.Unlock()
			}
			update := resp.GetUpdateplayer()
			if update != nil && game.Players[resp.Player] != nil {
				game.Players[resp.Player].Mux.Lock()
				game.Players[resp.Player].Position.X = update.Position.X
				game.Players[resp.Player].Position.Y = update.Position.Y
				game.Players[resp.Player].Mux.Unlock()
			}
		}
	}()

	view.OnDirectionChange = func(player *backend.Player) {
		direction := proto.Move_STOP
		switch player.Direction {
		case backend.DirectionUp:
			direction = proto.Move_UP
		case backend.DirectionDown:
			direction = proto.Move_DOWN
		case backend.DirectionLeft:
			direction = proto.Move_LEFT
		case backend.DirectionRight:
			direction = proto.Move_RIGHT
		}
		req := proto.Request{
			Action: &proto.Request_Move{
				Move: &proto.Move{
					Direction: direction,
				},
			},
		}
		stream.Send(&req)
	}

	view.Start()
}
