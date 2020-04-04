package main

import (
	"log"
	"math/rand"
	"time"

	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/mortenson/grpc-game-example/pkg/client"
	"github.com/mortenson/grpc-game-example/pkg/frontend"
	"google.golang.org/grpc"
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randSeq(n int) string {
	rand.Seed(time.Now().Unix())
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func main() {
	game := backend.NewGame()
	view := frontend.NewView(&game)
	game.Start()

	playerName := randSeq(6)

	conn, err := grpc.Dial(":8888", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("can not connect with server %v", err)
	}

	client := client.NewGameClient(conn, &game, view)
	client.Connect(playerName)
	client.Start()

	view.Start()
}
