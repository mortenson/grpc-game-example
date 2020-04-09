package main

import (
	"github.com/google/uuid"
	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/mortenson/grpc-game-example/pkg/frontend"
)

func main() {
	currentPlayer := backend.Player{
		Name:            "Alice",
		Icon:            'A',
		IdentifierBase:  backend.IdentifierBase{uuid.New()},
		CurrentPosition: backend.Coordinate{X: -1, Y: -5},
	}
	game := backend.NewGame()
	game.AddEntity(&currentPlayer)
	view := frontend.NewView(game)
	view.CurrentPlayer = currentPlayer.ID()

	game.Start()
	view.Start()
}
