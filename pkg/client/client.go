package client

import (
	"context"
	"io"
	"log"

	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/mortenson/grpc-game-example/pkg/frontend"
	"github.com/mortenson/grpc-game-example/proto"
	"google.golang.org/grpc"
)

type GameClient struct {
	CurrentPlayer *backend.Player
	Stream        proto.Game_StreamClient
	Game          *backend.Game
	View          *frontend.View
}

func NewGameClient(conn grpc.ClientConnInterface, game *backend.Game, view *frontend.View) *GameClient {
	client := proto.NewGameClient(conn)
	stream, err := client.Stream(context.Background())
	// @todo
	//ctx := stream.Context()
	if err != nil {
		log.Fatalf("openn stream error %v", err)
	}

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

	return &GameClient{
		Stream: stream,
		Game:   game,
		View:   view,
	}
}

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

func (c *GameClient) HandleInitialize(resp *proto.Response) {
	init := resp.GetInitialize()
	if init == nil {
		// @todo error
	}
	c.Game.Mux.Lock()
	c.CurrentPlayer.Position.X = int(init.Position.X)
	c.CurrentPlayer.Position.Y = int(init.Position.Y)
	c.Game.Players[c.CurrentPlayer.Name] = c.CurrentPlayer
	for _, player := range init.Players {
		c.Game.Players[player.Player] = &backend.Player{
			Position: backend.Coordinate{
				X: int(player.Position.X),
				Y: int(player.Position.Y),
			},
			Name: player.Player,
			Icon: 'P',
		}
	}
	c.Game.Mux.Unlock()
	c.View.CurrentPlayer = c.CurrentPlayer
}

func (c *GameClient) HandleAddPlayer(resp *proto.Response) {
	add := resp.GetAddplayer()
	if add == nil {
		// @todo error
	}
	newPlayer := backend.Player{
		Position: backend.Coordinate{
			X: int(add.Position.X),
			Y: int(add.Position.Y),
		},
		Name: resp.Player,
		Icon: 'P',
	}
	c.Game.Mux.Lock()
	c.Game.Players[resp.Player] = &newPlayer
	c.Game.Mux.Unlock()
}

func (c *GameClient) HandleUpdatePlayer(resp *proto.Response) {
	update := resp.GetUpdateplayer()
	if update != nil && c.Game.Players[resp.Player] != nil {
		c.Game.Players[resp.Player].Mux.Lock()
		c.Game.Players[resp.Player].Position.X = int(update.Position.X)
		c.Game.Players[resp.Player].Position.Y = int(update.Position.Y)
		c.Game.Players[resp.Player].Mux.Unlock()
	}
}

func (c *GameClient) Start() {
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
				c.HandleInitialize(resp)
			case *proto.Response_Addplayer:
				c.HandleAddPlayer(resp)
			case *proto.Response_Updateplayer:
				c.HandleUpdatePlayer(resp)
			}
		}
	}()
}
