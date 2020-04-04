package main

import (
	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/mortenson/grpc-game-example/pkg/frontend"
)

func main() {
	currentPlayer := backend.Player{
		Position: backend.Coordinate{X: -1, Y: -5},
		Name:     "Alice",
		Icon:     'A',
	}
	game := backend.NewGame()
	game.Players[currentPlayer.Name] = &currentPlayer
	view := frontend.NewView(&game)
	view.CurrentPlayer = &currentPlayer

	game.Start()
	view.Start()
}
