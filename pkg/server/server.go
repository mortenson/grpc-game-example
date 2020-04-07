package server

import (
	"log"
	"sync"

	"github.com/golang/protobuf/ptypes"

	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/mortenson/grpc-game-example/proto"
)

// client contains information about connected clients.
type client struct {
	StreamServer proto.Game_StreamServer
}

// GameServer is used to stream game information with clients.
type GameServer struct {
	proto.UnimplementedGameServer
	Game    *backend.Game
	Clients map[string]*client
	Mux     sync.RWMutex
}

// NewGameServer constructs a new game server struct.
func NewGameServer(game *backend.Game) *GameServer {
	server := &GameServer{Game: game, Clients: make(map[string]*client)}
	server.WatchChanges()
	return server
}

// Broadcast sends a response to all clients.
func (s *GameServer) Broadcast(resp *proto.Response) {
	s.Mux.RLock()
	for name, client := range s.Clients {
		if err := client.StreamServer.Send(resp); err != nil {
			log.Printf("broadcast error %v", err)
		}
		log.Printf("broadcasted %+v message to %s", resp, name)
	}
	s.Mux.RUnlock()
}

// HandleConnectRequest processes new players.
func (s *GameServer) HandleConnectRequest(req *proto.Request, srv proto.Game_StreamServer) string {
	connect := req.GetConnect()
	currentPlayer := connect.GetPlayer()
	// Build a slice of current players.
	players := make([]*proto.Player, 0)
	s.Game.Mux.RLock()
	for _, player := range s.Game.Players {
		players = append(players, &proto.Player{
			Player:   player.Name,
			Position: proto.GetProtoCoordinate(player.Position),
		})
	}
	// Build a slice of current lasers.
	lasers := make([]*proto.Laser, 0)
	for uuid, laser := range s.Game.Lasers {
		starttime, err := ptypes.TimestampProto(laser.StartTime)
		if err != nil {
			// @todo handle
			continue
		}
		lasers = append(lasers, &proto.Laser{
			Direction: proto.GetProtoDirection(laser.Direction),
			Uuid:      uuid.String(),
			Starttime: starttime,
			Position:  proto.GetProtoCoordinate(laser.InitialPosition),
		})
	}
	s.Game.Mux.RUnlock()

	// Send the client an initialize message.
	resp := proto.Response{
		Action: &proto.Response_Initialize{
			Initialize: &proto.Initialize{
				Position: &proto.Coordinate{X: 10, Y: 10},
				Players:  players,
				Lasers:   lasers,
			},
		},
	}
	if err := srv.Send(&resp); err != nil {
		log.Printf("send error %v", err)
	}
	log.Printf("sent initialize message for %v", currentPlayer)

	// Add the new player.
	s.Game.Mux.Lock()
	s.Game.Players[currentPlayer] = &backend.Player{
		Position: backend.Coordinate{X: 10, Y: 10},
		Name:     currentPlayer,
		Icon:     'P',
	}
	s.Game.Mux.Unlock()

	// Inform all other clients of the new player.
	resp = proto.Response{
		Player: currentPlayer,
		Action: &proto.Response_Addplayer{
			Addplayer: &proto.AddPlayer{
				Position: &proto.Coordinate{X: 10, Y: 10},
			},
		},
	}
	s.Broadcast(&resp)

	// Add the new client.
	s.Mux.Lock()
	s.Clients[currentPlayer] = &client{
		StreamServer: srv,
	}
	s.Mux.Unlock()

	// Return the new player name.
	return currentPlayer
}

// HandleMoveRequest makes a request to the game engine to move a player.
func (s *GameServer) HandleMoveRequest(currentPlayer string, req *proto.Request, srv proto.Game_StreamServer) {
	move := req.GetMove()
	s.Game.ActionChannel <- backend.MoveAction{
		PlayerName: currentPlayer,
		Direction:  proto.GetBackendDirection(move.Direction),
	}
}

func (s *GameServer) HandleLaserRequest(currentPlayer string, req *proto.Request, srv proto.Game_StreamServer) {
	move := req.GetLaser()
	s.Game.ActionChannel <- backend.LaserAction{
		PlayerName: currentPlayer,
		Direction:  proto.GetBackendDirection(move.Direction),
	}
}

// RemoveClient removes a client from the game, usually in response to a logout
// or series error.
func (s *GameServer) RemoveClient(playerName string, srv proto.Game_StreamServer) {
	s.Mux.Lock()
	delete(s.Clients, playerName)
	s.Mux.Unlock()
	s.Game.Mux.Lock()
	delete(s.Game.Players, playerName)
	delete(s.Game.LastAction, playerName)
	s.Game.Mux.Unlock()

	resp := proto.Response{
		Player: playerName,
		Action: &proto.Response_Removeplayer{
			Removeplayer: &proto.RemovePlayer{},
		},
	}
	s.Broadcast(&resp)
}

// Stream is the main loop for dealing with individual players.
func (s *GameServer) Stream(srv proto.Game_StreamServer) error {
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
		if err != nil {
			log.Printf("receive error %v", err)
			if currentPlayer != "" {
				s.RemoveClient(currentPlayer, srv)
			}
			continue
		}

		if req.GetConnect() != nil {
			currentPlayer = s.HandleConnectRequest(req, srv)
		}

		// A local variable is used to track the current player.
		if currentPlayer == "" {
			continue
		}

		switch req.GetAction().(type) {
		case *proto.Request_Move:
			s.HandleMoveRequest(currentPlayer, req, srv)
		case *proto.Request_Laser:
			s.HandleLaserRequest(currentPlayer, req, srv)
		}
	}
}

// HandlePositionChange broadcasts position changes to clients.
func (s *GameServer) HandlePositionChange(change backend.PositionChange) {
	resp := proto.Response{
		Player: change.PlayerName,
		Action: &proto.Response_Updateplayer{
			Updateplayer: &proto.UpdatePlayer{
				Position: proto.GetProtoCoordinate(change.Position),
			},
		},
	}
	s.Broadcast(&resp)
}

func (s *GameServer) HandleLaserChange(change backend.LaserChange) {
	timestamp, err := ptypes.TimestampProto(change.Laser.StartTime)
	if err != nil {
		// @todo handle
		return
	}
	resp := proto.Response{
		Action: &proto.Response_Addlaser{
			Addlaser: &proto.AddLaser{
				Laser: &proto.Laser{
					Direction: proto.GetProtoDirection(change.Laser.Direction),
					Uuid:      change.UUID.String(),
					Starttime: timestamp,
					Position:  proto.GetProtoCoordinate(change.Laser.GetPosition()),
				},
			},
		},
	}
	s.Broadcast(&resp)
}

// WatchChanges waits for new game engine changes and broadcasts to clients.
func (s *GameServer) WatchChanges() {
	go func() {
		for {
			change := <-s.Game.ChangeChannel
			switch change.(type) {
			case backend.PositionChange:
				change := change.(backend.PositionChange)
				s.HandlePositionChange(change)
			case backend.LaserChange:
				change := change.(backend.LaserChange)
				s.HandleLaserChange(change)
			}
		}
	}()
}
