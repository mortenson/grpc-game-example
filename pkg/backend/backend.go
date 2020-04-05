package backend

import (
	"sync"
	"time"
)

// Game is the backend engine for the game. It can be used regardless of how
// game data is rendered, or if a game server is being used.
type Game struct {
	Players       map[string]*Player
	Mux           sync.Mutex
	ChangeChannel chan Change
	ActionChannel chan Action
	LastAction    map[string]time.Time
}

// NewGame constructs a new Game struct.
func NewGame() *Game {
	game := Game{
		Players:       make(map[string]*Player),
		ActionChannel: make(chan Action, 1),
		LastAction:    make(map[string]time.Time),
		ChangeChannel: make(chan Change, 1),
	}
	return &game
}

// Start begins the main game loop, which waits for new actions and updates the
// game state occordinly.
func (game *Game) Start() {
	go func() {
		for {
			action := <-game.ActionChannel
			action.Perform(game)
		}
	}()
}

// Coordinate is used for all position-related variables.
type Coordinate struct {
	X int
	Y int
}

// Direction is used to represent Direction constants.
type Direction int

// Contains direction constants - DirectionStop will take no effect.
const (
	DirectionUp Direction = iota
	DirectionDown
	DirectionLeft
	DirectionRight
	DirectionStop
)

// Change is sent by the game engine in response to Actions.
type Change interface{}

// Action is sent by the client when attempting to change game state. The
// engine can choose to reject Actions if they are invalid or performed too
// frequently.
type Action interface {
	Perform(game *Game)
}

// MoveAction is sent when a user presses an arrow key.
type MoveAction struct {
	Action
	PlayerName string
	Direction  Direction
}

// Perform contains backend logic required to move a player.
func (action MoveAction) Perform(game *Game) {
	// Check that the player exists.
	game.Mux.Lock()
	defer game.Mux.Unlock()
	player, ok := game.Players[action.PlayerName]
	if !ok {
		return
	}
	player.Mux.Lock()
	defer player.Mux.Unlock()
	// Throttle the movement frequency for the player.
	// @todo If this pattern becomes common move to main loop.
	actionKey := "move_" + action.PlayerName
	lastAction, ok := game.LastAction[actionKey]
	if ok && lastAction.After(time.Now().Add(-50*time.Millisecond)) {
		return
	}
	// Move the player.
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
	// Update the last moved time.
	game.LastAction[actionKey] = time.Now()
	// Inform the client that the player moved.
	game.ChangeChannel <- PositionChange{
		PlayerName: player.Name,
		Direction:  action.Direction,
		Position:   player.Position,
	}
}

// PositionChange is sent when the game engine moves a player.
type PositionChange struct {
	Change
	PlayerName string
	Direction  Direction
	Position   Coordinate
}

// Player contains information unique to local and remote players.
type Player struct {
	Position Coordinate
	Name     string
	Icon     rune
	Mux      sync.Mutex
}
