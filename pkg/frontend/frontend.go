package frontend

import (
	"fmt"
	"sort"
	"strings"
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
	Pages         *tview.Pages
	DrawCallbacks []func()
	ViewPort      tview.Primitive
	CurrentPlayer uuid.UUID
	Paused        bool
}

func centeredModal(p tview.Primitive, width, height int) tview.Primitive {
	width = 0
	height = 0
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 1, false).
			AddItem(nil, 0, 1, false), width, 1, false).
		AddItem(nil, 0, 1, false)
}

func setupRoundWaitModal(view *View) {
	textView := tview.NewTextView()
	textView.SetTextAlign(tview.AlignCenter).
		SetScrollable(true).
		SetBorder(true).
		SetTitle("Round complete")
	modal := centeredModal(textView, 60, 5)
	view.Pages.AddPage("roundwait", modal, true, false)

	callback := func() {
		if view.Game.WaitForRound {
			view.Pages.ShowPage("roundwait")
			seconds := int(view.Game.NewRoundAt.Sub(time.Now()).Seconds())
			if seconds < 0 {
				seconds = 0
			}
			player := view.Game.GetEntity(view.Game.RoundWinner).(*backend.Player)
			text := fmt.Sprintf("\nWinner: %s\n\n", player.Name)
			text += fmt.Sprintf("New round in %d seconds...", seconds)
			textView.SetText(text)
		} else {
			view.Pages.HidePage("roundwait")
			view.App.SetFocus(view.ViewPort)
		}
	}
	view.DrawCallbacks = append(view.DrawCallbacks, callback)
	view.Pages.AddPage("roundwait", modal, true, false)
}

func setupScoreModal(view *View) {
	textView := tview.NewTextView()
	textView.SetBorder(true).SetTitle("Score")
	modal := centeredModal(textView, 60, 23)

	callback := func() {
		text := ""
		view.Game.Mu.RLock()
		type PlayerScore struct {
			Name  string
			Score int
		}
		playerScore := make([]PlayerScore, 0)
		for _, entity := range view.Game.Entities {
			player, ok := entity.(*backend.Player)
			if !ok {
				continue
			}
			score, ok := view.Game.Score[player.ID()]
			if !ok {
				score = 0
			}
			playerScore = append(playerScore, PlayerScore{
				Name:  player.Name,
				Score: score,
			})
		}
		sort.Slice(playerScore, func(i, j int) bool {
			if playerScore[i].Score > playerScore[j].Score {
				return true
			}
			if strings.ToLower(playerScore[i].Name) < strings.ToLower(playerScore[j].Name) {
				return true
			}
			return false
		})
		for _, playerScore := range playerScore {
			text += fmt.Sprintf("%s - %d\n", playerScore.Name, playerScore.Score)
		}
		view.Game.Mu.RUnlock()
		textView.SetText(text)
	}
	view.DrawCallbacks = append(view.DrawCallbacks, callback)
	view.Pages.AddPage("score", modal, true, false)
}

func setupViewPort(view *View) {
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
	view.Pages.AddPage("viewport", box, true, true)
	view.ViewPort = box
}

// NewView construsts a new View struct.
func NewView(game *backend.Game) *View {
	app := tview.NewApplication()
	pages := tview.NewPages()
	view := &View{
		Game:          game,
		App:           app,
		Paused:        false,
		Pages:         pages,
		DrawCallbacks: make([]func(), 0),
	}
	setupViewPort(view)
	setupScoreModal(view)
	setupRoundWaitModal(view)
	app.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Rune() == 'p' {
			pages.ShowPage("score")
		}
		if e.Key() == tcell.KeyESC {
			pages.HidePage("score")
			app.SetFocus(view.ViewPort)
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
			for _, callback := range view.DrawCallbacks {
				view.App.QueueUpdate(callback)
			}
			view.App.Draw()
			time.Sleep(17 * time.Microsecond)
		}
	}()
	return view.App.Run()
}
