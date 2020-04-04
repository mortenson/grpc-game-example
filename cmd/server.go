package main

import (
	"io"
	"log"
	"net"
	"sync"

	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/mortenson/grpc-game-example/proto"

	"google.golang.org/grpc"
)

type client struct {
	StreamServer proto.Game_StreamServer
}

type server struct {
	proto.UnimplementedGameServer
	Game    *backend.Game
	Clients map[string]*client
	Mux     sync.Mutex
}

func (s server) Broadcast(resp *proto.Response) {
	s.Mux.Lock()
	for name, client := range s.Clients {
		if err := client.StreamServer.Send(resp); err != nil {
			log.Printf("broadcast error %v", err)
		}
		log.Printf("broadcasted message to %s", name)
	}
	s.Mux.Unlock()
}

func (s server) Stream(srv proto.Game_StreamServer) error {
	log.Println("start new server")
	ctx := srv.Context()
	currentPlayer := ""
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
		if connect != nil && connect.GetPlayer() != "" {
			currentPlayer = connect.GetPlayer()
			s.Game.Mux.Lock()
			s.Game.Players[currentPlayer] = &backend.Player{
				Position:  backend.Coordinate{X: 10, Y: 10},
				Name:      currentPlayer,
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
			s.Mux.Lock()
			s.Clients[currentPlayer] = &client{
				StreamServer: srv,
			}
			if err := srv.Send(&resp); err != nil {
				log.Printf("send error %v", err)
			}
			log.Printf("sent initialize message")

			resp = proto.Response{
				Action: &proto.Response_Addplayer{
					Addplayer: &proto.AddPlayer{
						Position: &proto.Coordinate{X: 10, Y: 10},
					},
				},
			}
			s.Broadcast(&resp)
		}

		if currentPlayer == "" {
			continue
		}

		move := req.GetMove()
		if move != nil {
			s.Game.Mux.Lock()
			switch move.Direction {
			case proto.Move_UP:
				s.Game.Players[currentPlayer].Direction = backend.DirectionUp
			case proto.Move_DOWN:
				s.Game.Players[currentPlayer].Direction = backend.DirectionDown
			case proto.Move_LEFT:
				s.Game.Players[currentPlayer].Direction = backend.DirectionLeft
			case proto.Move_RIGHT:
				s.Game.Players[currentPlayer].Direction = backend.DirectionRight
			case proto.Move_STOP:
				s.Game.Players[currentPlayer].Direction = backend.DirectionStop
			}
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
	server := &server{Game: &game, Clients: make(map[string]*client)}
	proto.RegisterGameServer(s, server)

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
