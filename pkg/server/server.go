package server

import (
	"log"
	"sync"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/uuid"

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
	Clients map[uuid.UUID]*client
	Mux     sync.RWMutex
}

// NewGameServer constructs a new game server struct.
func NewGameServer(game *backend.Game) *GameServer {
	server := &GameServer{Game: game, Clients: make(map[uuid.UUID]*client)}
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
func (s *GameServer) HandleConnectRequest(req *proto.Request, srv proto.Game_StreamServer) uuid.UUID {
	connect := req.GetConnect()
	// Build a slice of current entities.
	entities := make([]*proto.Entity, 0)
	for _, entity := range s.Game.Entities {
		protoEntity := proto.GetProtoEntity(entity)
		if protoEntity != nil {
			entities = append(entities, &proto.Entity{Entity: protoEntity})
		}
	}
	// @todo Choose a start position away from other players?
	startCoordinate := backend.Coordinate{X: 0, Y: 0}

	// Send the client an initialize message.
	resp := proto.Response{
		Action: &proto.Response_Initialize{
			Initialize: &proto.Initialize{
				Position: proto.GetProtoCoordinate(startCoordinate),
				Entities: entities,
			},
		},
	}
	if err := srv.Send(&resp); err != nil {
		log.Printf("send error %v", err)
	}
	log.Printf("sent initialize message for %s", connect.Name)

	// Add the new player.
	playerID, err := uuid.Parse(connect.Id)
	if err != nil {
		// @todo handle
	}
	player := &backend.Player{
		Name:           connect.Name,
		Icon:           'P',
		IdentifierBase: backend.IdentifierBase{playerID},
	}
	player.Move(startCoordinate)
	s.Game.AddEntity(player)

	// Inform all other clients of the new player.
	resp = proto.Response{
		Id: connect.Id,
		Action: &proto.Response_Addentity{
			Addentity: &proto.AddEntity{
				Entity: &proto.Entity{
					Entity: &proto.Entity_Player{
						Player: &proto.Player{
							Name:     player.Name,
							Position: proto.GetProtoCoordinate(player.Position()),
						},
					},
				},
			},
		},
	}
	s.Broadcast(&resp)

	// Add the new client.
	s.Mux.Lock()
	s.Clients[player.ID()] = &client{
		StreamServer: srv,
	}
	s.Mux.Unlock()

	// Return the new player ID.
	return player.ID()
}

// HandleMoveRequest makes a request to the game engine to move a player.
func (s *GameServer) HandleMoveRequest(playerID uuid.UUID, req *proto.Request, srv proto.Game_StreamServer) {
	move := req.GetMove()
	s.Game.ActionChannel <- backend.MoveAction{
		ID:        playerID,
		Direction: proto.GetBackendDirection(move.Direction),
	}
}

func (s *GameServer) HandleLaserRequest(playerID uuid.UUID, req *proto.Request, srv proto.Game_StreamServer) {
	move := req.GetLaser()
	s.Game.ActionChannel <- backend.LaserAction{
		OwnerID:   playerID,
		Direction: proto.GetBackendDirection(move.Direction),
	}
}

// RemoveClient removes a client from the game, usually in response to a logout
// or series error.
func (s *GameServer) RemoveClient(playerID uuid.UUID, srv proto.Game_StreamServer) {
	s.Mux.Lock()
	delete(s.Clients, playerID)
	s.Mux.Unlock()
	s.Game.RemoveEntity(playerID)

	resp := proto.Response{
		Id: playerID.String(),
		Action: &proto.Response_Removeentity{
			Removeentity: &proto.RemoveEntity{},
		},
	}
	s.Broadcast(&resp)
}

// Stream is the main loop for dealing with individual players.
func (s *GameServer) Stream(srv proto.Game_StreamServer) error {
	log.Println("start new server")
	ctx := srv.Context()
	var playerID uuid.UUID
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := srv.Recv()
		if err != nil {
			log.Printf("receive error %v", err)
			if playerID.String() != "" {
				s.RemoveClient(playerID, srv)
			}
			continue
		}

		if req.GetConnect() != nil {
			playerID = s.HandleConnectRequest(req, srv)
		}

		// A local variable is used to track the current player.
		if playerID.String() == "" {
			continue
		}

		switch req.GetAction().(type) {
		case *proto.Request_Move:
			s.HandleMoveRequest(playerID, req, srv)
		case *proto.Request_Laser:
			s.HandleLaserRequest(playerID, req, srv)
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

func (s *GameServer) HandleLaserRemoveChange(change backend.LaserRemoveChange) {
	resp := proto.Response{
		Action: &proto.Response_Removelaser{
			Removelaser: &proto.RemoveLaser{
				Uuid: change.UUID.String(),
			},
		},
	}
	s.Broadcast(&resp)
}

func (s *GameServer) HandlePlayerKilledChange(change backend.PlayerKilledChange) {
	resp := proto.Response{
		Player: change.PlayerName,
		Action: &proto.Response_Playerkilled{
			Playerkilled: &proto.PlayerKilled{
				SpawnPosition: proto.GetProtoCoordinate(change.SpawnPosition),
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
			case backend.LaserRemoveChange:
				change := change.(backend.LaserRemoveChange)
				s.HandleLaserRemoveChange(change)
			case backend.PlayerKilledChange:
				change := change.(backend.PlayerKilledChange)
				s.HandlePlayerKilledChange(change)
			}
		}
	}()
}
