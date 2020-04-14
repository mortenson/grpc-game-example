package main

import (
	"context"
	"log"
	"math/rand"
	"time"

	"github.com/gdamore/tcell"
	"github.com/google/uuid"
	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/mortenson/grpc-game-example/pkg/client"
	"github.com/mortenson/grpc-game-example/pkg/frontend"
	"github.com/mortenson/grpc-game-example/proto"
	"github.com/rivo/tview"
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

type connectInfo struct {
	PlayerName string
	Address    string
}

func connectApp(info *connectInfo) *tview.Application {
	backgroundColor := tcell.Color234
	app := tview.NewApplication()
	flex := tview.NewFlex().
		SetDirection(tview.FlexRow)
	flex.SetBorder(true).
		SetTitle("Connect to tshooter server").
		SetBackgroundColor(backgroundColor)
	errors := tview.NewTextView().
		SetText(" Use the tab key to change fields, and enter to press a button")
	errors.SetBackgroundColor(backgroundColor)
	form := tview.NewForm()
	form.AddInputField("Player name", "", 16, nil, nil).
		AddInputField("Server address", ":8888", 32, nil, nil).
		AddButton("Connect", func() {
			info.PlayerName = form.GetFormItem(0).(*tview.InputField).GetText()
			info.Address = form.GetFormItem(1).(*tview.InputField).GetText()
			if info.PlayerName == "" || info.Address == "" {
				errors.SetText(" All fields are required.")
				return
			}
			app.Stop()
		}).
		AddButton("Quit", func() {
			app.Stop()
		})
	form.SetLabelColor(tcell.ColorWhite).
		SetButtonBackgroundColor(tcell.Color24).
		SetFieldBackgroundColor(tcell.Color24).
		SetBackgroundColor(backgroundColor)
	flex.AddItem(errors, 1, 1, false)
	flex.AddItem(form, 0, 1, false)
	app.SetRoot(flex, true).SetFocus(form)
	return app
}

func main() {
	game := backend.NewGame()
	game.IsAuthoritative = false
	view := frontend.NewView(game)
	game.Start()

	info := connectInfo{}
	connectApp := connectApp(&info)
	connectApp.Run()

	conn, err := grpc.Dial(info.Address, grpc.WithInsecure())
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

	client := client.NewGameClient(stream, game, view)
	client.Start()

	playerID := uuid.New()
	client.Connect(playerID, info.PlayerName)

	view.Start()
}
