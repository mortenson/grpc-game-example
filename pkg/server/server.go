package server

import (
	"io"
	"log"
	"sync"

	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/mortenson/grpc-game-example/proto"
)

type client struct {
	StreamServer proto.Game_StreamServer
}

type GameServer struct {
	proto.UnimplementedGameServer
	Game    *backend.Game
	Clients map[string]*client
	Mux     sync.Mutex
}

func (s GameServer) Broadcast(resp *proto.Response) {
	s.Mux.Lock()
	for name, client := range s.Clients {
		if err := client.StreamServer.Send(resp); err != nil {
			log.Printf("broadcast error %v", err)
		}
		log.Printf("broadcasted message to %s", name)
	}
	s.Mux.Unlock()
}

func (s GameServer) HandleConnect(req *proto.Request, srv proto.Game_StreamServer) string {
	connect := req.GetConnect()
	if connect == nil {
		// @todo error
	}
	currentPlayer := connect.GetPlayer()
	s.Game.Mux.Lock()
	players := make([]*proto.Player, 0)
	for _, player := range s.Game.Players {
		players = append(players, &proto.Player{
			Player:   player.Name,
			Position: &proto.Coordinate{X: int32(player.Position.X), Y: int32(player.Position.Y)},
		})
	}
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
				Players:  players,
			},
		},
	}
	if err := srv.Send(&resp); err != nil {
		log.Printf("send error %v", err)
	}
	log.Printf("sent initialize message for %v", currentPlayer)

	resp = proto.Response{
		Player: currentPlayer,
		Action: &proto.Response_Addplayer{
			Addplayer: &proto.AddPlayer{
				Position: &proto.Coordinate{X: 10, Y: 10},
			},
		},
	}
	s.Broadcast(&resp)

	s.Mux.Lock()
	s.Clients[currentPlayer] = &client{
		StreamServer: srv,
	}
	s.Mux.Unlock()

	return currentPlayer
}

func (s GameServer) HandleMove(currentPlayer string, req *proto.Request, srv proto.Game_StreamServer) {
	move := req.GetMove()
	if move == nil {
		// @todo error
	}
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
	s.Game.Mux.Unlock()
}

func (s GameServer) Stream(srv proto.Game_StreamServer) error {
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

		if req.GetConnect() != nil {
			currentPlayer = s.HandleConnect(req, srv)
		}

		if currentPlayer == "" {
			continue
		}

		switch req.GetAction().(type) {
		case *proto.Request_Move:
			s.HandleMove(currentPlayer, req, srv)
		}
	}
}

func NewGameServer(game *backend.Game) *GameServer {
	server := &GameServer{Game: game, Clients: make(map[string]*client)}
	game.OnPositionChange = func(player *backend.Player) {
		resp := proto.Response{
			Player: player.Name,
			Action: &proto.Response_Updateplayer{
				Updateplayer: &proto.UpdatePlayer{
					Position: &proto.Coordinate{X: int32(player.Position.X), Y: int32(player.Position.Y)},
				},
			},
		}
		log.Printf("update %s", player.Name)
		server.Broadcast(&resp)
	}
	return server
}
