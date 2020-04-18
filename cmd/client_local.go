package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/mortenson/grpc-game-example/pkg/bot"
	"github.com/mortenson/grpc-game-example/pkg/frontend"
)

func main() {
	numBots := flag.Int("bots", 1, "The number of bots to play against.")
	flag.Parse()

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

	bots := bot.NewBots(game)
	for i := 0; i < *numBots; i++ {
		bots.AddBot(fmt.Sprintf("Bob %d", i))
	}

	game.Start()
	view.Start()
	bots.Start()

	err := <-view.Done
	if err != nil {
		log.Fatal(err)
	}
}
