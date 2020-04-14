package server

import (
	"log"
	"sync"
	"time"

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
	game    *backend.Game
	clients map[uuid.UUID]*client
	mu      sync.RWMutex
}

// NewGameServer constructs a new game server struct.
func NewGameServer(game *backend.Game) *GameServer {
	server := &GameServer{game: game, clients: make(map[uuid.UUID]*client)}
	server.WatchChanges()
	return server
}

func (s *GameServer) removePlayer(playerID uuid.UUID) {
	s.game.Mu.Lock()
	s.game.RemoveEntity(playerID)
	s.game.Mu.Unlock()

	resp := proto.Response{
		Action: &proto.Response_RemoveEntity{
			RemoveEntity: &proto.RemoveEntity{
				Id: playerID.String(),
			},
		},
	}
	s.broadcast(&resp)
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
				s.mu.Lock()
				delete(s.clients, playerID)
				s.mu.Unlock()
				s.removePlayer(playerID)
			}
			continue
		}
		log.Printf("got message %+v", req)

		if req.GetConnect() != nil {
			playerID = s.handleConnectRequest(req, srv)
			isConnected = true
		}

		if !isConnected {
			continue
		}

		switch req.GetAction().(type) {
		case *proto.Request_Move:
			s.handleMoveRequest(playerID, req, srv)
		case *proto.Request_Laser:
			s.handleLaserRequest(playerID, req, srv)
		}
	}
}

// WatchChanges waits for new game engine changes and broadcasts to clients.
func (s *GameServer) WatchChanges() {
	go func() {
		for {
			change := <-s.game.ChangeChannel
			switch change.(type) {
			case backend.MoveChange:
				change := change.(backend.MoveChange)
				s.handleMoveChange(change)
			case backend.AddEntityChange:
				change := change.(backend.AddEntityChange)
				s.handleAddEntityChange(change)
			case backend.RemoveEntityChange:
				change := change.(backend.RemoveEntityChange)
				s.handleRemoveEntityChange(change)
			case backend.PlayerRespawnChange:
				change := change.(backend.PlayerRespawnChange)
				s.handlePlayerRespawnChange(change)
			case backend.RoundOverChange:
				change := change.(backend.RoundOverChange)
				s.handleRoundOverChange(change)
			case backend.RoundStartChange:
				change := change.(backend.RoundStartChange)
				s.handleRoundStartChange(change)
			}
		}
	}()
}

// broadcast sends a response to all clients.
func (s *GameServer) broadcast(resp *proto.Response) {
	removals := []uuid.UUID{}
	s.mu.Lock()
	for id, client := range s.clients {
		if err := client.StreamServer.Send(resp); err != nil {
			log.Printf("broadcast error %v, removing %s", err, id)
			delete(s.clients, id)
			removals = append(removals, id)
		}
		log.Printf("broadcasted %+v message to %s", resp, id)
	}
	s.mu.Unlock()
	for _, id := range removals {
		s.removePlayer(id)
	}
}

// handleConnectRequest processes new players.
func (s *GameServer) handleConnectRequest(req *proto.Request, srv proto.Game_StreamServer) uuid.UUID {
	s.game.Mu.Lock()
	defer s.game.Mu.Unlock()

	connect := req.GetConnect()

	// @todo Choose a start position away from other players?
	startCoordinate := backend.Coordinate{X: 0, Y: 0}

	// Add the new player.
	playerID, err := uuid.Parse(connect.Id)
	if err != nil {
		// @todo handle
	}
	player := &backend.Player{
		Name:            connect.Name,
		Icon:            'P',
		IdentifierBase:  backend.IdentifierBase{UUID: playerID},
		CurrentPosition: startCoordinate,
	}
	s.game.AddEntity(player)

	// Build a slice of current entities.
	entities := make([]*proto.Entity, 0)
	for _, entity := range s.game.Entities {
		protoEntity := proto.GetProtoEntity(entity)
		if protoEntity != nil {
			entities = append(entities, protoEntity)
		}
	}

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
	s.broadcast(&resp)

	// Add the new client.
	s.mu.Lock()
	s.clients[player.ID()] = &client{
		StreamServer: srv,
	}
	s.mu.Unlock()

	// Return the new player ID.
	return player.ID()
}

// handleMoveRequest makes a request to the game engine to move a player.
func (s *GameServer) handleMoveRequest(playerID uuid.UUID, req *proto.Request, srv proto.Game_StreamServer) {
	move := req.GetMove()
	s.game.ActionChannel <- backend.MoveAction{
		ID:        playerID,
		Direction: proto.GetBackendDirection(move.Direction),
	}
}

func (s *GameServer) handleLaserRequest(playerID uuid.UUID, req *proto.Request, srv proto.Game_StreamServer) {
	laser := req.GetLaser()
	id, err := uuid.Parse(laser.Id)
	if err != nil {
		// @todo handle
		return
	}
	s.game.ActionChannel <- backend.LaserAction{
		OwnerID:   playerID,
		ID:        id,
		Direction: proto.GetBackendDirection(laser.Direction),
	}
}

func (s *GameServer) handleMoveChange(change backend.MoveChange) {
	resp := proto.Response{
		Action: &proto.Response_UpdateEntity{
			UpdateEntity: &proto.UpdateEntity{
				Entity: proto.GetProtoEntity(change.Entity),
			},
		},
	}
	s.broadcast(&resp)
}

func (s *GameServer) handleAddEntityChange(change backend.AddEntityChange) {
	resp := proto.Response{
		Action: &proto.Response_AddEntity{
			AddEntity: &proto.AddEntity{
				Entity: proto.GetProtoEntity(change.Entity),
			},
		},
	}
	s.broadcast(&resp)
}

func (s *GameServer) handleRemoveEntityChange(change backend.RemoveEntityChange) {
	resp := proto.Response{
		Action: &proto.Response_RemoveEntity{
			RemoveEntity: &proto.RemoveEntity{
				Id: change.Entity.ID().String(),
			},
		},
	}
	s.broadcast(&resp)
}

func (s *GameServer) handlePlayerRespawnChange(change backend.PlayerRespawnChange) {
	resp := proto.Response{
		Action: &proto.Response_PlayerRespawn{
			PlayerRespawn: &proto.PlayerRespawn{
				Player:     proto.GetProtoPlayer(change.Player),
				KilledById: change.KilledByID.String(),
			},
		},
	}
	s.broadcast(&resp)
}

func (s *GameServer) handleRoundOverChange(change backend.RoundOverChange) {
	s.game.Mu.RLock()
	defer s.game.Mu.RUnlock()
	timestamp, err := ptypes.TimestampProto(s.game.NewRoundAt)
	if err != nil {
		return
	}
	resp := proto.Response{
		Action: &proto.Response_RoundOver{
			RoundOver: &proto.RoundOver{
				RoundWinnerId: s.game.RoundWinner.String(),
				NewRoundAt:    timestamp,
			},
		},
	}
	s.broadcast(&resp)
}

func (s *GameServer) handleRoundStartChange(change backend.RoundStartChange) {
	players := []*proto.Player{}
	s.game.Mu.RLock()
	for _, entity := range s.game.Entities {
		player, ok := entity.(*backend.Player)
		if !ok {
			continue
		}
		players = append(players, proto.GetProtoPlayer(player))
	}
	s.game.Mu.RUnlock()
	resp := proto.Response{
		Action: &proto.Response_RoundStart{
			RoundStart: &proto.RoundStart{
				Players: players,
			},
		},
	}
	s.broadcast(&resp)
}
