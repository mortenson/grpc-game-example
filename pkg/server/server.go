package server

import (
	"log"
	"sync"
	"time"

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

	// @todo Choose a start position away from other players?
	startCoordinate := backend.Coordinate{X: 0, Y: 0}

	// Add the new player.
	playerID, err := uuid.Parse(connect.Id)
	if err != nil {
		// @todo handle
	}
	player := &backend.Player{
		Name:           connect.Name,
		Icon:           'P',
		IdentifierBase: backend.IdentifierBase{UUID: playerID},
	}
	player.Move(startCoordinate)
	s.Game.AddEntity(player)

	// Build a slice of current entities.
	entities := make([]*proto.Entity, 0)
	s.Game.Mu.RLock()
	for _, entity := range s.Game.Entities {
		protoEntity := proto.GetProtoEntity(entity)
		if protoEntity != nil {
			entities = append(entities, protoEntity)
		}
	}
	s.Game.Mu.RUnlock()

	// @todo handle cases where connection is too fast
	time.Sleep(time.Second * 1)
	// Send the client an initialize message.
	resp := proto.Response{
		Action: &proto.Response_Initialize{
			Initialize: &proto.Initialize{
				Entities: entities,
			},
		},
	}
	if err := srv.Send(&resp); err != nil {
		log.Printf("send error %v", err)
	}
	log.Printf("sent initialize message for %s", connect.Name)

	// Inform all other clients of the new player.
	resp = proto.Response{
		Action: &proto.Response_AddEntity{
			AddEntity: &proto.AddEntity{
				Entity: proto.GetProtoEntity(player),
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
		Action: &proto.Response_RemoveEntity{
			RemoveEntity: &proto.RemoveEntity{
				Id: playerID.String(),
			},
		},
	}
	s.Broadcast(&resp)
}

// Stream is the main loop for dealing with individual players.
func (s *GameServer) Stream(srv proto.Game_StreamServer) error {
	log.Println("start new server")
	ctx := srv.Context()
	var playerID uuid.UUID
	isConnected := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := srv.Recv()
		if err != nil {
			log.Printf("receive error %v", err)
			if isConnected {
				s.RemoveClient(playerID, srv)
			}
			continue
		}

		if req.GetConnect() != nil {
			playerID = s.HandleConnectRequest(req, srv)
			isConnected = true
		}

		if !isConnected {
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
		Action: &proto.Response_UpdateEntity{
			UpdateEntity: &proto.UpdateEntity{
				Entity: proto.GetProtoEntity(change.Entity),
			},
		},
	}
	s.Broadcast(&resp)
}

func (s *GameServer) HandleAddEntityChange(change backend.AddEntityChange) {
	resp := proto.Response{
		Action: &proto.Response_AddEntity{
			AddEntity: &proto.AddEntity{
				Entity: proto.GetProtoEntity(change.Entity),
			},
		},
	}
	s.Broadcast(&resp)
}

func (s *GameServer) HandleRemoveEntityChange(change backend.RemoveEntityChange) {
	resp := proto.Response{
		Action: &proto.Response_RemoveEntity{
			RemoveEntity: &proto.RemoveEntity{
				Id: change.Entity.ID().String(),
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
			case backend.AddEntityChange:
				change := change.(backend.AddEntityChange)
				s.HandleAddEntityChange(change)
			case backend.RemoveEntityChange:
				change := change.(backend.RemoveEntityChange)
				s.HandleRemoveEntityChange(change)
			}
		}
	}()
}
