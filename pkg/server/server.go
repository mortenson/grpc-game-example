package server

import (
	"errors"
	"log"
	"math/rand"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/uuid"

	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/mortenson/grpc-game-example/proto"
)

const (
	clientTimeout = 15
	maxClients    = 8
)

// client contains information about connected clients.
type client struct {
	streamServer proto.Game_StreamServer
	done         chan error
	id           uuid.UUID
}

// GameServer is used to stream game information with clients.
type GameServer struct {
	proto.UnimplementedGameServer
	game     *backend.Game
	clients  map[uuid.UUID]*client
	mu       sync.RWMutex
	password string
}

// NewGameServer constructs a new game server struct.
func NewGameServer(game *backend.Game, password string) *GameServer {
	server := &GameServer{
		game:     game,
		clients:  make(map[uuid.UUID]*client),
		password: password,
	}
	server.WatchChanges()
	return server
}

func (s *GameServer) removeClient(currentClient *client) {
	s.mu.Lock()
	delete(s.clients, currentClient.id)
	s.mu.Unlock()
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
	if len(s.clients) >= maxClients {
		return errors.New("The server is full")
	}
	log.Println("start new server")
	ctx := srv.Context()
	done := make(chan error)
	var currentClient *client
	// Cancel client if they timeout.
	lastMessage := time.Now()
	streamStartTime := time.Now()
	timeoutTicker := time.NewTicker(1 * time.Minute)
	go func() {
		for {
			if currentClient != nil && time.Now().Sub(lastMessage).Minutes() > clientTimeout {
				done <- errors.New("you have been timed out")
				return
			} else if currentClient == nil && time.Now().Sub(streamStartTime).Seconds() > 30 {
				done <- errors.New("failed to connect within minimum timeframe")
				return
			}
			<-timeoutTicker.C
		}
	}()
	// Wait for stream requests.
	go func() {
		for {
			req, err := srv.Recv()
			if err != nil {
				log.Printf("receive error %v", err)
				done <- errors.New("failed to receive request")
				return
			}
			log.Printf("got message %+v", req)

			if currentClient != nil {
				lastMessage = time.Now()
			}

			if currentClient == nil && req.GetConnect() != nil {
				playerID, err := s.handleConnectRequest(req, srv)
				if err != nil {
					done <- err
					return
				}
				// Add the new client.
				s.mu.Lock()
				currentClient = &client{
					streamServer: srv,
					id:           playerID,
					done:         make(chan error),
				}
				s.clients[playerID] = currentClient
				s.mu.Unlock()
			}

			if currentClient == nil {
				continue
			}

			switch req.GetAction().(type) {
			case *proto.Request_Move:
				s.handleMoveRequest(req, currentClient)
			case *proto.Request_Laser:
				s.handleLaserRequest(req, currentClient)
			}
		}
	}()
	// Wait for stream to be done.
	var doneError error
	select {
	case <-ctx.Done():
		doneError = ctx.Err()
	case doneError = <-done:
	}
	timeoutTicker.Stop()
	log.Printf(`stream done with error "%v"`, doneError)
	// Remove client if it was added.
	if currentClient != nil {
		log.Printf("%s - removing client", currentClient.id)
		s.removeClient(currentClient)
		s.removePlayer(currentClient.id)
	}
	return doneError
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
	s.mu.Lock()
	for id, currentClient := range s.clients {
		if err := currentClient.streamServer.Send(resp); err != nil {
			log.Printf("%s - broadcast error %v", id, err)
			currentClient.done <- errors.New("failed to broadcast message")
			continue
		}
		log.Printf("%s - broadcasted %+v", resp, id)
	}
	s.mu.Unlock()
}

// handleConnectRequest processes new players.
func (s *GameServer) handleConnectRequest(req *proto.Request, srv proto.Game_StreamServer) (uuid.UUID, error) {
	// When debugging failed connections adding a sleep has worked in the past.
	//time.Sleep(time.Second * 1)

	connect := req.GetConnect()
	playerID, err := uuid.Parse(connect.Id)
	if err != nil {
		return playerID, err
	}

	// Exit as early as possible if password is wrong.
	if connect.Password != s.password {
		return playerID, errors.New("invalid password provided")
	}

	// Check if player already exists.
	s.game.Mu.RLock()
	if s.game.GetEntity(playerID) != nil {
		return playerID, errors.New("duplicate player ID provided")
	}
	s.game.Mu.RUnlock()

	re := regexp.MustCompile("^[a-zA-Z0-9]+$")
	if !re.MatchString(connect.Name) {
		return playerID, errors.New("invalid name provided")
	}
	icon, _ := utf8.DecodeRuneInString(strings.ToUpper(connect.Name))

	// Choose a random spawn point.
	spawnPoints := s.game.GetMapSpawnPoints()
	rand.Seed(time.Now().Unix())
	i := rand.Int() % len(spawnPoints)
	startCoordinate := spawnPoints[i]

	// Add the player.
	player := &backend.Player{
		Name:            connect.Name,
		Icon:            icon,
		IdentifierBase:  backend.IdentifierBase{UUID: playerID},
		CurrentPosition: startCoordinate,
	}
	s.game.Mu.Lock()
	s.game.AddEntity(player)
	s.game.Mu.Unlock()

	// Build a slice of current entities.
	s.game.Mu.RLock()
	entities := make([]*proto.Entity, 0)
	for _, entity := range s.game.Entities {
		protoEntity := proto.GetProtoEntity(entity)
		if protoEntity != nil {
			entities = append(entities, protoEntity)
		}
	}
	s.game.Mu.RUnlock()

	// Send the client an initialize message.
	resp := proto.Response{
		Action: &proto.Response_Initialize{
			Initialize: &proto.Initialize{
				Entities: entities,
			},
		},
	}
	if err := srv.Send(&resp); err != nil {
		s.removePlayer(playerID)
		return playerID, err
	}
	log.Printf("%s - sent initialize message", connect.Id)

	// Inform all other clients of the new player.
	resp = proto.Response{
		Action: &proto.Response_AddEntity{
			AddEntity: &proto.AddEntity{
				Entity: proto.GetProtoEntity(player),
			},
		},
	}
	s.broadcast(&resp)

	return playerID, nil
}

// handleMoveRequest makes a request to the game engine to move a player.
func (s *GameServer) handleMoveRequest(req *proto.Request, currentClient *client) {
	move := req.GetMove()
	s.game.ActionChannel <- backend.MoveAction{
		ID:        currentClient.id,
		Direction: proto.GetBackendDirection(move.Direction),
		Created:   time.Now(),
	}
}

func (s *GameServer) handleLaserRequest(req *proto.Request, currentClient *client) {
	laser := req.GetLaser()
	id, err := uuid.Parse(laser.Id)
	if err != nil {
		currentClient.done <- errors.New("invalid laser ID provided")
		return
	}
	s.game.Mu.RLock()
	if s.game.GetEntity(id) != nil {
		currentClient.done <- errors.New("duplicate laser ID provided")
		return
	}
	s.game.Mu.RUnlock()
	s.game.ActionChannel <- backend.LaserAction{
		OwnerID:   currentClient.id,
		ID:        id,
		Direction: proto.GetBackendDirection(laser.Direction),
		Created:   time.Now(),
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
		log.Fatalf("unable to parse new round timestamp %v", s.game.NewRoundAt)
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
