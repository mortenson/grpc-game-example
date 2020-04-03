package main

import (
	"io"
	"log"
	"net"

	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/mortenson/grpc-game-example/proto"

	"google.golang.org/grpc"
)

type server struct {
	proto.UnimplementedGameServer
	Game *backend.Game
}

func (s server) Stream(srv proto.Game_StreamServer) error {
	log.Println("start new server")
	ctx := srv.Context()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := srv.Recv()
		if err == io.EOF {
			log.Println("exit")
			return nil
		}
		if err != nil {
			log.Printf("receive error %v", err)
			continue
		}

		connect := req.GetConnect()
		player := connect.GetPlayer()
		if connect != nil && player != "" {
			s.Game.Mux.Lock()
			s.Game.Players[player] = &backend.Player{
				Position:  backend.Coordinate{X: 10, Y: 10},
				Name:      player,
				Direction: backend.DirectionStop,
				Icon:      'P',
			}
			s.Game.Mux.Unlock()
			resp := proto.Response{
				Action: &proto.Response_Initialize{
					Initialize: &proto.Initialize{
						Position: &proto.Coordinate{X: 10, Y: 10},
					},
				},
			}
			if err := srv.Send(&resp); err != nil {
				log.Printf("send error %v", err)
			}
			log.Printf("sent initialize message")
		}
	}
}

func main() {
	lis, err := net.Listen("tcp", ":8888")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	game := backend.NewGame()
	game.Start()
	s := grpc.NewServer()
	proto.RegisterGameServer(s, server{Game: &game})

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
