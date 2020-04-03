package backend

import (
	"sync"
	"time"
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
	Players       []*Player
	CurrentPlayer *Player
}

func NewGame() Game {
	game := Game{}
	return game
}

func (game *Game) Start() {
	go func() {
		lastmove := map[string]time.Time{}
		for {
			for _, player := range game.Players {
				player.Mux.Lock()
				if player.Direction == DirectionStop || lastmove[player.Name].After(time.Now().Add(-50*time.Millisecond)) {
					player.Direction = DirectionStop
					player.Mux.Unlock()
					continue
				}
				switch player.Direction {
				case DirectionUp:
					player.Position.Y--
				case DirectionDown:
					player.Position.Y++
				case DirectionLeft:
					player.Position.X--
				case DirectionRight:
					player.Position.X++
				}
				player.Direction = DirectionStop
				lastmove[player.Name] = time.Now()
				player.Mux.Unlock()
			}
		}
	}()
}
