package frontend

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell"
	"github.com/google/uuid"
	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/rivo/tview"
)

// View renders the game and handles user interaction.
type View struct {
	Game          *backend.Game
	App           *tview.Application
	CurrentPlayer uuid.UUID
	Paused        bool
}

// NewView construsts a new View struct.
func NewView(game *backend.Game) *View {
	app := tview.NewApplication()
	pages := tview.NewPages()
	view := &View{
		Game:   game,
		App:    app,
		Paused: false,
	}
	score := tview.NewTextView()
	score.SetBorder(true).SetTitle("score")
	updateScore := func() {
		text := ""
		game.Mu.RLock()
		for _, entity := range view.Game.Entities {
			player, ok := entity.(*backend.Player)
			if !ok {
				continue
			}
			score, ok := view.Game.Score[player.ID()]
			if !ok {
				score = 0
			}
			text += fmt.Sprintf("%s - %d\n", player.Name, score)
		}
		game.Mu.RUnlock()
		score.SetText(text)
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
		view.Game.Mu.RLock()
		for _, entity := range view.Game.Entities {
			positioner, ok := entity.(backend.Positioner)
			if !ok {
				continue
			}
			position := positioner.Position()
			var icon rune
			var color tcell.Color
			switch entity.(type) {
			case *backend.Player:
				icon = entity.(*backend.Player).Icon
				color = tcell.ColorGreen
			case *backend.Laser:
				icon = 'x'
				color = tcell.ColorRed
			default:
				continue
			}
			screen.SetContent(centerX+position.X, centerY+position.Y, icon, nil, tcell.StyleDefault.Foreground(color))
		}
		view.Game.Mu.RUnlock()
		return 0, 0, 0, 0
	})
	// Handle player movement input.
	box.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if view.Paused {
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
			view.Game.ActionChannel <- backend.MoveAction{
				ID:        view.CurrentPlayer,
				Direction: direction,
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
			view.Game.ActionChannel <- backend.LaserAction{
				OwnerID:   view.CurrentPlayer,
				ID:        uuid.New(),
				Direction: laserDirection,
			}
		}
		return e
	})
	pages.AddPage("viewport", box, true, true)
	pages.AddPage("score", score, true, false)
	app.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Rune() == 'p' {
			updateScore()
			pages.ShowPage("score")
		}
		if e.Key() == tcell.KeyESC {
			pages.HidePage("score")
		}
		return e
	})
	app.SetRoot(pages, true)
	return view
}

// Start starts the frontend game loop.
func (view *View) Start() error {
	// Main loop - re-draw at ~60 FPS.
	go func() {
		for {
			if view.Paused {
				continue
			}
			view.App.Draw()
			time.Sleep(17 * time.Microsecond)
		}
	}()
	return view.App.Run()
}
