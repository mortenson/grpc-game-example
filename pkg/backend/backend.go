package backend

import (
	"sync"
	"time"
)

type Coordinate struct {
	X int
	Y int
}

type Direction int

const (
	DirectionUp Direction = iota
	DirectionDown
	DirectionLeft
	DirectionRight
	DirectionStop
)

type Action interface {
	Perform(game *Game)
}

type MoveAction struct {
	Action
	PlayerName string
	Direction  Direction
}

type Player struct {
	Position Coordinate
	Name     string
	Icon     rune
	Mux      sync.Mutex
}

type Game struct {
	Players          map[string]*Player
	Mux              sync.Mutex
	OnPositionChange func(*Player)
	ActionChannel    chan Action
	LastAction       map[string]time.Time
}

func (action MoveAction) Perform(game *Game) {
	game.Mux.Lock()
	defer game.Mux.Unlock()
	player, ok := game.Players[action.PlayerName]
	if !ok {
		return
	}
	actionKey := "move_" + action.PlayerName
	lastAction, ok := game.LastAction[actionKey]
	if ok && lastAction.After(time.Now().Add(-50*time.Millisecond)) {
		return
	}
	player.Mux.Lock()
	switch action.Direction {
	case DirectionUp:
		player.Position.Y--
	case DirectionDown:
		player.Position.Y++
	case DirectionLeft:
		player.Position.X--
	case DirectionRight:
		player.Position.X++
	}
	player.Mux.Unlock()
	game.LastAction[actionKey] = time.Now()
	if game.OnPositionChange != nil {
		game.OnPositionChange(player)
	}
}

func NewGame() Game {
	game := Game{
		Players:       make(map[string]*Player),
		ActionChannel: make(chan Action, 1),
		LastAction:    make(map[string]time.Time),
	}
	return game
}

func (game *Game) Start() {
	go func() {
		for {
			action := <-game.ActionChannel
			action.Perform(game)
		}
	}()
}
