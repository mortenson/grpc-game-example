package main

// Connects to a server for play.

import (
	"log"
	"regexp"

	"github.com/gdamore/tcell"
	"github.com/google/uuid"
	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/mortenson/grpc-game-example/pkg/client"
	"github.com/mortenson/grpc-game-example/pkg/frontend"
	"github.com/mortenson/grpc-game-example/proto"
	"github.com/rivo/tview"
	"google.golang.org/grpc"
)

const (
	backgroundColor = tcell.Color234
	textColor       = tcell.ColorWhite
	fieldColor      = tcell.Color24
)

type connectInfo struct {
	PlayerName string
	Address    string
	Password   string
}

// It feels wrong to have this much frontend code in a command file, but this
// is done as the frontend package has no awareness of the client/server model,
// and as a result should not have UIs like this.
// Maybe, if anything, it shows how you can compose tview applications?
func connectApp(info *connectInfo) *tview.Application {
	app := tview.NewApplication()
	flex := tview.NewFlex().
		SetDirection(tview.FlexRow)
	flex.SetBorder(true).
		SetTitle("Connect to tshooter server").
		SetBackgroundColor(backgroundColor)
	errors := tview.NewTextView().
		SetText(" Use the tab key to change fields, and enter to submit")
	errors.SetBackgroundColor(backgroundColor)
	form := tview.NewForm()
	re := regexp.MustCompile("^[a-zA-Z0-9]+$")
	form.AddInputField("Player name", "", 16, func(textCheck string, lastChar rune) bool {
		result := re.MatchString(textCheck)
		if !result {
			errors.SetText(" Only alphanumeric characters are allowed")
		}
		return result
	}, nil).
		AddInputField("Server address", ":8888", 32, nil, nil).
		AddPasswordField("Server password", "", 32, '*', nil).
		AddButton("Connect", func() {
			info.PlayerName = form.GetFormItem(0).(*tview.InputField).GetText()
			info.Address = form.GetFormItem(1).(*tview.InputField).GetText()
			info.Password = form.GetFormItem(2).(*tview.InputField).GetText()
			if info.PlayerName == "" || info.Address == "" {
				errors.SetText(" All fields are required.")
				return
			}
			app.Stop()
		}).
		AddButton("Quit", func() {
			app.Stop()
		})
	form.SetLabelColor(textColor).
		SetButtonBackgroundColor(fieldColor).
		SetFieldBackgroundColor(fieldColor).
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
	client := client.NewGameClient(game, view)

	playerID := uuid.New()
	err = client.Connect(grpcClient, playerID, info.PlayerName, info.Password)
	if err != nil {
		log.Fatalf("connect request failed %v", err)
	}
	client.Start()

	view.Start()

	err = <-view.Done
	if err != nil {
		log.Fatal(err)
	}
}
