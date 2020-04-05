package frontend

import (
	"time"

	"github.com/gdamore/tcell"
	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/rivo/tview"
)

// View renders the game and handles user interaction.
type View struct {
	Game          *backend.Game
	App           *tview.Application
	CurrentPlayer *backend.Player
}

// NewView construsts a new View struct.
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
		for _, laser := range game.Lasers {
			laserPosition := laser.GetPosition()
			screen.SetContent(centerX+laserPosition.X, centerY+laserPosition.Y, 'x', nil, tcell.StyleDefault.Foreground(tcell.ColorYellow))
		}
		return 0, 0, 0, 0
	})
	// Handle player movement input.
	box.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if view.CurrentPlayer == nil {
			return e
		}
		// Movement
		direction := backend.DirectionStop
		switch e.Key() {
		case tcell.KeyUp:
			direction = backend.DirectionUp
		case tcell.KeyDown:
			direction = backend.DirectionDown
		case tcell.KeyLeft:
			direction = backend.DirectionLeft
		case tcell.KeyRight:
			direction = backend.DirectionRight
		}
		if direction != backend.DirectionStop {
			game.ActionChannel <- backend.MoveAction{
				PlayerName: view.CurrentPlayer.Name,
				Direction:  direction,
			}
		}
		// Lasers
		laserDirection := backend.DirectionStop
		switch e.Rune() {
		case 'w':
			laserDirection = backend.DirectionUp
		case 's':
			laserDirection = backend.DirectionDown
		case 'a':
			laserDirection = backend.DirectionLeft
		case 'd':
			laserDirection = backend.DirectionRight
		}
		if laserDirection != backend.DirectionStop {
			game.ActionChannel <- backend.LaserAction{
				PlayerName: view.CurrentPlayer.Name,
				Direction:  laserDirection,
			}
		}
		return e
	})
	app.SetRoot(box, true).SetFocus(box)
	return view
}

// Start starts the frontend game loop.
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
