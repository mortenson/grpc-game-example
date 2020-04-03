package frontend

import (
	"time"

	"github.com/gdamore/tcell"
	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/rivo/tview"
)

type View struct {
	Game          *backend.Game
	App           *tview.Application
	CurrentPlayer *backend.Player
}

func NewView(game *backend.Game) *View {
	app := tview.NewApplication()
	view := &View{
		Game: game,
		App:  app,
	}
	box := tview.NewBox().SetBorder(true).SetTitle("grpc-game-example")
	box.SetDrawFunc(func(screen tcell.Screen, x int, y int, width int, height int) (int, int, int, int) {
		width = width - 1
		height = height - 1
		centerY := y + height/2
		centerX := x + width/2
		for x := 1; x < width; x++ {
			for y := 1; y < height; y++ {
				screen.SetContent(x, y, ' ', nil, tcell.StyleDefault.Foreground(tcell.ColorWhite))
			}
		}
		screen.SetContent(centerX, centerY, 'O', nil, tcell.StyleDefault.Foreground(tcell.ColorWhite))
		for _, player := range game.Players {
			player.Mux.Lock()
			screen.SetContent(centerX+player.Position.X, centerY+player.Position.Y, player.Icon, nil, tcell.StyleDefault.Foreground(tcell.ColorRed))
			player.Mux.Unlock()
		}
		return 0, 0, 0, 0
	})
	// Handle player movement input.
	box.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if view.CurrentPlayer == nil {
			return e
		}
		view.CurrentPlayer.Mux.Lock()
		switch e.Key() {
		case tcell.KeyUp:
			view.CurrentPlayer.Direction = backend.DirectionUp
		case tcell.KeyDown:
			view.CurrentPlayer.Direction = backend.DirectionDown
		case tcell.KeyLeft:
			view.CurrentPlayer.Direction = backend.DirectionLeft
		case tcell.KeyRight:
			view.CurrentPlayer.Direction = backend.DirectionRight
		}
		view.CurrentPlayer.Mux.Unlock()
		return e
	})
	app.SetRoot(box, true).SetFocus(box)
	return view
}

func (view *View) Start() error {
	// Main loop - re-draw at ~60 FPS.
	go func() {
		for {
			view.App.Draw()
			time.Sleep(17 * time.Microsecond)
		}
	}()
	return view.App.Run()
}
