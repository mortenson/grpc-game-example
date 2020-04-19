package frontend

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/gdamore/tcell"
	"github.com/google/uuid"
	"github.com/mortenson/grpc-game-example/pkg/backend"
	"github.com/rivo/tview"
)

const (
	backgroundColor = tcell.Color234
	textColor       = tcell.ColorWhite
	playerColor     = tcell.ColorWhite
	wallColor       = tcell.Color24
	laserColor      = tcell.ColorRed
	drawFrequency   = 17 * time.Millisecond
)

// View renders the game and handles user interaction.
type View struct {
	Game          *backend.Game
	App           *tview.Application
	CurrentPlayer uuid.UUID
	pages         *tview.Pages
	drawCallbacks []func()
	viewPort      tview.Primitive
	Done          chan error
}

func centeredModal(p tview.Primitive) tview.Primitive {
	width := 0
	height := 0
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
		SetBackgroundColor(backgroundColor).
		SetTitle("Round complete")
	modal := centeredModal(textView)
	view.pages.AddPage("roundwait", modal, true, false)

	callback := func() {
		view.Game.Mu.RLock()
		defer view.Game.Mu.RUnlock()
		if view.Game.WaitForRound {
			view.pages.ShowPage("roundwait")
			seconds := int(view.Game.NewRoundAt.Sub(time.Now()).Seconds())
			if seconds < 0 {
				seconds = 0
			}
			player := view.Game.GetEntity(view.Game.RoundWinner).(*backend.Player)
			text := fmt.Sprintf("\nWinner: %s\n\n", player.Name)
			text += fmt.Sprintf("New round in %d seconds...", seconds)
			textView.SetText(text)
		} else {
			view.pages.HidePage("roundwait")
			view.App.SetFocus(view.viewPort)
		}
	}
	view.drawCallbacks = append(view.drawCallbacks, callback)
	view.pages.AddPage("roundwait", modal, true, false)
}

func setupScoreModal(view *View) {
	textView := tview.NewTextView()
	textView.SetBorder(true).SetTitle("Score").SetBackgroundColor(backgroundColor)
	modal := centeredModal(textView)

	callback := func() {
		view.Game.Mu.RLock()
		defer view.Game.Mu.RUnlock()
		text := ""
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
			} else if playerScore[i].Score < playerScore[j].Score {
				return false
			} else if strings.ToLower(playerScore[i].Name) < strings.ToLower(playerScore[j].Name) {
				return true
			}
			return false
		})
		for _, playerScore := range playerScore {
			text += fmt.Sprintf("%s - %d\n", playerScore.Name, playerScore.Score)
		}
		textView.SetText(text)
	}
	view.drawCallbacks = append(view.drawCallbacks, callback)
	view.pages.AddPage("score", modal, true, false)
}

func withinDrawBounds(x, y, width, height int) bool {
	return x < width && x > 0 && y < height && y > 0
}

func setupViewPort(view *View) {
	box := tview.NewBox().
		SetBorder(true).
		SetTitle("tshooter").
		SetBackgroundColor(backgroundColor)
	cameraX := 0
	cameraY := 0
	box.SetDrawFunc(func(screen tcell.Screen, x int, y int, width int, height int) (int, int, int, int) {
		view.Game.Mu.RLock()
		defer view.Game.Mu.RUnlock()
		style := tcell.StyleDefault.Background(backgroundColor)
		// Move camera
		currentEntity := view.Game.GetEntity(view.CurrentPlayer)
		if currentEntity == nil {
			return 0, 0, 0, 0
		}
		currentPlayerPosition := currentEntity.(*backend.Player).Position()
		cameraDiffX := float64(cameraX - currentPlayerPosition.X)
		cameraDiffY := float64(cameraY - currentPlayerPosition.Y)
		cameraDiffXMax := float64(width / 6)
		cameraDiffYMax := float64(height / 6)
		if math.Abs(cameraDiffX) > cameraDiffXMax {
			if cameraDiffX <= 0 {
				cameraX++
			} else {
				cameraX--
			}
		}
		if math.Abs(cameraDiffY) > cameraDiffYMax {
			if cameraDiffY <= 0 {
				cameraY++
			} else {
				cameraY--
			}
		}
		width = width - 1
		height = height - 1
		mapWidth, mapHeight := view.Game.GetMapDimensions()
		if width > mapWidth {
			cameraX = -1
		}
		if height > mapHeight {
			cameraY = -1
		}
		centerY := (y + height/2) - cameraY
		centerX := (x + width/2) - cameraX
		// Draw center point - useful for debugging
		// if withinDrawBounds(centerX, centerY, width, height) {
		// 	screen.SetContent(centerX, centerY, 'C', nil, style.Foreground(tcell.ColorWhite))
		// }
		// Draw entities
		for _, entity := range view.Game.Entities {
			positioner, ok := entity.(backend.Positioner)
			if !ok {
				continue
			}
			position := positioner.Position()
			drawX := centerX + position.X
			drawY := centerY + position.Y
			if !withinDrawBounds(drawX, drawY, width, height) {
				continue
			}
			var icon rune
			var color tcell.Color
			switch entity.(type) {
			case *backend.Player:
				icon = entity.(*backend.Player).Icon
				color = playerColor
			case *backend.Laser:
				icon = 'x'
				color = laserColor
			default:
				continue
			}
			// See if player is far from center of viewport.
			screen.SetContent(drawX, drawY, icon, nil, style.Foreground(color))
		}
		// Draw map
		for _, wall := range view.Game.GetMapByType()[backend.MapTypeWall] {
			x := centerX + wall.X
			y := centerY + wall.Y
			if !withinDrawBounds(x, y, width, height) {
				continue
			}
			screen.SetContent(x, y, '█', nil, style.Foreground(wallColor))
		}
		return 0, 0, 0, 0
	})
	// Handle player movement input.
	box.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
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
				Created:   time.Now(),
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
				Created:   time.Now(),
			}
		}
		return e
	})
	helpText := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText("← → ↑ ↓ move - wasd shoot - p players - esc close - ctrl+q quit").
		SetTextColor(textColor)
	helpText.SetBackgroundColor(backgroundColor)
	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(box, 0, 1, true).
		AddItem(helpText, 1, 1, false)
	view.pages.AddPage("viewport", flex, true, true)
	view.viewPort = box
}

// NewView construsts a new View struct.
func NewView(game *backend.Game) *View {
	app := tview.NewApplication()
	pages := tview.NewPages()
	view := &View{
		Game:          game,
		App:           app,
		pages:         pages,
		drawCallbacks: make([]func(), 0),
		Done:          make(chan error),
	}
	setupViewPort(view)
	setupScoreModal(view)
	setupRoundWaitModal(view)
	app.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Rune() == 'p' {
			pages.ShowPage("score")
		}
		switch e.Key() {
		case tcell.KeyEsc:
			pages.HidePage("score")
			app.SetFocus(view.viewPort)
		case tcell.KeyCtrlQ:
			fallthrough
		case tcell.KeyCtrlC:
			app.Stop()
			select {
			case view.Done <- nil:
			default:
			}
		}

		return e
	})
	app.SetRoot(pages, true)
	return view
}

// Start starts the frontend game loop.
func (view *View) Start() {
	drawTicker := time.NewTicker(drawFrequency)
	stop := make(chan bool)
	go func() {
		for {
			for _, callback := range view.drawCallbacks {
				view.App.QueueUpdate(callback)
			}
			view.App.Draw()
			<-drawTicker.C
			select {
			case <-stop:
				return
			default:
			}
		}
	}()
	go func() {
		err := view.App.Run()
		stop <- true
		drawTicker.Stop()
		select {
		case view.Done <- err:
		default:
		}
	}()
}
