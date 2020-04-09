package main

import (
	"context"
	"log"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/mortenson/grpc-game-example/pkg/client"
	"github.com/mortenson/grpc-game-example/pkg/frontend"
	"github.com/mortenson/grpc-game-example/proto"
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
	view := frontend.NewView(game)
	game.Start()

	playerName := randSeq(6)

	conn, err := grpc.Dial(":8888", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("can not connect with server %v", err)
	}

	grpcClient := proto.NewGameClient(conn)
	stream, err := grpcClient.Stream(context.Background())
	if err != nil {
		panic(err)
	}
	ctx := stream.Context()

	go func() {
		<-ctx.Done()
		if err := ctx.Err(); err != nil {
			log.Println(err)
		}
		view.App.Stop()
	}()

	if err != nil {
		log.Fatalf("openn stream error %v", err)
	}

	playerID := uuid.New()
	client := client.NewGameClient(stream, game, view)
	client.Start()
	client.Connect(playerID, playerName)

	view.Start()
}
