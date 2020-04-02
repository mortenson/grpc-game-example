package main

import (
	"sync"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

type Coordinate struct {
	X int
	Y int
}

type Direction int32

const (
	DirectionUp Direction = iota
	DirectionDown
	DirectionLeft
	DirectionRight
	DirectionStop
)

type Player struct {
	Position  Coordinate
	Name      string
	Direction Direction
	Icon      rune
	Mux       sync.Mutex
}

type Game struct {
	Players []*Player
}

func main() {
	currentPlayer := Player{
		Position:  Coordinate{X: -1, Y: -5},
		Name:      "Alice",
		Icon:      'A',
		Direction: DirectionStop,
	}
	game := Game{Players: []*Player{
		&currentPlayer,
		&Player{
			Position:  Coordinate{X: 10, Y: 10},
			Name:      "Bob",
			Icon:      'B',
			Direction: DirectionStop,
		},
	}}
	box := tview.NewBox().SetBorder(true).SetTitle("grpc-game-example")
	box.SetDrawFunc(func(screen tcell.Screen, x int, y int, width int, height int) (int, int, int, int) {
		width = width - 1
		height = height - 1
		centerY := y + height/2
		centerX := x + width/2
		for x := 1; x < width; x++ {
			for y := 1; y < height; y++ {
				screen.SetContent(x, y, '.', nil, tcell.StyleDefault.Foreground(tcell.ColorWhite))
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
	box.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		switch e.Key() {
		case tcell.KeyUp:
			currentPlayer.Direction = DirectionUp
		case tcell.KeyDown:
			currentPlayer.Direction = DirectionDown
		case tcell.KeyLeft:
			currentPlayer.Direction = DirectionLeft
		case tcell.KeyRight:
			currentPlayer.Direction = DirectionRight
		default:
			currentPlayer.Direction = DirectionStop
		}
		return e
	})
	go func() {
		for {
			for _, player := range game.Players {
				player.Mux.Lock()
				switch player.Direction {
				case DirectionUp:
					player.Position.Y -= 1
				case DirectionDown:
					player.Position.Y += 1
				case DirectionLeft:
					player.Position.X -= 1
				case DirectionRight:
					player.Position.X += 1
				}
				player.Direction = DirectionStop
				player.Mux.Unlock()
			}
			//time.Sleep(1 * time.Second)
		}
	}()
	app := tview.NewApplication()
	if err := app.SetRoot(box, true).SetFocus(box).Run(); err != nil {
		panic(err)
	}
}
