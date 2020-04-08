package client

import (
	"io"
	"log"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/uuid"
	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/mortenson/grpc-game-example/pkg/frontend"
	"github.com/mortenson/grpc-game-example/proto"
)

// GameClient is used to stream game information to a server and update the
// game state as needed.
type GameClient struct {
	CurrentPlayer *backend.Player
	Stream        proto.Game_StreamClient
	Game          *backend.Game
	View          *frontend.View
}

// NewGameClient constructs a new game client struct.
func NewGameClient(stream proto.Game_StreamClient, game *backend.Game, view *frontend.View) *GameClient {
	return &GameClient{
		Stream: stream,
		Game:   game,
		View:   view,
	}
}

// Connect connects a new player to the server.
func (c *GameClient) Connect(playerName string) {
	c.CurrentPlayer = &backend.Player{
		Name: playerName,
		Icon: 'P',
	}
	req := proto.Request{
		Action: &proto.Request_Connect{
			Connect: &proto.Connect{
				Player: playerName,
			},
		},
	}
	c.Stream.Send(&req)
}

// Start begins the goroutines needed to recieve server changes and send game
// changes.
func (c *GameClient) Start() {
	// Handle local game engine changes.
	go func() {
		for {
			change := <-c.Game.ChangeChannel
			switch change.(type) {
			case backend.PositionChange:
				change := change.(backend.PositionChange)
				c.HandlePositionChange(change)
			case backend.LaserChange:
				change := change.(backend.LaserChange)
				c.HandleLaserChange(change)
			}
		}
	}()
	// Handle stream messages.
	go func() {
		for {
			resp, err := c.Stream.Recv()
			if err == io.EOF {
				log.Fatalf("EOF")
				return
			}
			if err != nil {
				log.Fatalf("can not receive %v", err)
			}

			switch resp.GetAction().(type) {
			case *proto.Response_Initialize:
				c.HandleInitializeResponse(resp)
			case *proto.Response_Addplayer:
				c.HandleAddPlayerResponse(resp)
			case *proto.Response_Updateplayer:
				c.HandleUpdatePlayerResponse(resp)
			case *proto.Response_Removeplayer:
				c.HandleRemovePlayerResponse(resp)
			case *proto.Response_Addlaser:
				c.HandleAddLaserResponse(resp)
			case *proto.Response_Removelaser:
				c.HandleRemoveLaserResponse(resp)
			case *proto.Response_Playerkilled:
				c.HandlePlayerKilledResponse(resp)
			}
		}
	}()
}

// HandlePositionChange sends position changes as moves to the server.
func (c *GameClient) HandlePositionChange(change backend.PositionChange) {
	req := proto.Request{
		Action: &proto.Request_Move{
			Move: &proto.Move{
				Direction: proto.GetProtoDirection(change.Direction),
			},
		},
	}
	c.Stream.Send(&req)
}

func (c *GameClient) HandleLaserChange(change backend.LaserChange) {
	req := proto.Request{
		Action: &proto.Request_Laser{
			Laser: &proto.Laser{
				Direction: proto.GetProtoDirection(change.Laser.Direction),
				Uuid:      change.UUID.String(),
			},
		},
	}
	c.Stream.Send(&req)
}

// HandleInitializeResponse initializes the local player with information
// provided by the server.
func (c *GameClient) HandleInitializeResponse(resp *proto.Response) {
	c.Game.Mux.Lock()
	defer c.Game.Mux.Unlock()
	init := resp.GetInitialize()
	c.CurrentPlayer.Position = proto.GetBackendCoordinate(init.Position)
	c.Game.Players[c.CurrentPlayer.Name] = c.CurrentPlayer
	for _, player := range init.Players {
		c.Game.Players[player.Player] = &backend.Player{
			Position: proto.GetBackendCoordinate(player.Position),
			Name:     player.Player,
			Icon:     'P',
		}
	}
	for _, laser := range init.Lasers {
		laserUUID, err := uuid.Parse(laser.Uuid)
		if err != nil {
			// @todo handle
			continue
		}
		starttime, err := ptypes.Timestamp(laser.Starttime)
		if err != nil {
			// @todo handle
			continue
		}
		c.Game.Lasers[laserUUID] = backend.Laser{
			InitialPosition: proto.GetBackendCoordinate(laser.Position),
			Direction:       proto.GetBackendDirection(laser.Direction),
			StartTime:       starttime,
		}
	}
	c.View.CurrentPlayer = c.CurrentPlayer
}

// HandleAddPlayerResponse adds a new player to the game.
func (c *GameClient) HandleAddPlayerResponse(resp *proto.Response) {
	add := resp.GetAddplayer()
	newPlayer := backend.Player{
		Position: proto.GetBackendCoordinate(add.Position),
		Name:     resp.Player,
		Icon:     'P',
	}
	c.Game.Mux.Lock()
	c.Game.Players[resp.Player] = &newPlayer
	c.Game.Mux.Unlock()
}

// HandleUpdatePlayerResponse updates a player's position.
func (c *GameClient) HandleUpdatePlayerResponse(resp *proto.Response) {
	c.Game.Mux.RLock()
	defer c.Game.Mux.RUnlock()
	update := resp.GetUpdateplayer()
	if c.Game.Players[resp.Player] == nil {
		return
	}
	// @todo We should sync current player positions in case of desync.
	if resp.Player == c.CurrentPlayer.Name {
		return
	}
	c.Game.Players[resp.Player].Mux.Lock()
	c.Game.Players[resp.Player].Position = proto.GetBackendCoordinate(update.Position)
	c.Game.Players[resp.Player].Mux.Unlock()
}

// HandleRemovePlayerResponse removes a player from the game.
func (c *GameClient) HandleRemovePlayerResponse(resp *proto.Response) {
	c.Game.Mux.Lock()
	defer c.Game.Mux.Unlock()
	delete(c.Game.Players, resp.Player)
	delete(c.Game.LastAction, resp.Player)
}

func (c *GameClient) HandleAddLaserResponse(resp *proto.Response) {
	addLaser := resp.GetAddlaser()
	protoLaser := addLaser.GetLaser()
	uuid, err := uuid.Parse(protoLaser.Uuid)
	if err != nil {
		// @todo handle
		return
	}
	startTime, err := ptypes.Timestamp(protoLaser.Starttime)
	if err != nil {
		// @todo handle
		return
	}
	c.Game.Mux.Lock()
	c.Game.Lasers[uuid] = backend.Laser{
		InitialPosition: proto.GetBackendCoordinate(protoLaser.Position),
		Direction:       proto.GetBackendDirection(protoLaser.Direction),
		StartTime:       startTime,
	}
	c.Game.Mux.Unlock()
}

// @todo Does it make sense to sync this over the network? The server already
// tells us who got hit so local collision detection should remove it anyway.
func (c *GameClient) HandleRemoveLaserResponse(resp *proto.Response) {
	removeLaser := resp.GetRemovelaser()
	uuid, err := uuid.Parse(removeLaser.Uuid)
	if err != nil {
		// @todo handle
		return
	}
	c.Game.Mux.Lock()
	delete(c.Game.Lasers, uuid)
	c.Game.Mux.Unlock()
}

func (c *GameClient) HandlePlayerKilledResponse(resp *proto.Response) {
	playerKilled := resp.GetPlayerkilled()
	playerName := resp.GetPlayer()
	if c.Game.Players[playerName] == nil {
		// @todo handle
		return
	}
	c.Game.Players[playerName].Mux.Lock()
	c.Game.Players[playerName].Position = proto.GetBackendCoordinate(playerKilled.SpawnPosition)
	c.Game.Players[playerName].Mux.Unlock()
}
