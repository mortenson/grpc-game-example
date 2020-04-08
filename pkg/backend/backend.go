package backend

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Game is the backend engine for the game. It can be used regardless of how
// game data is rendered, or if a game server is being used.
type Game struct {
	Players       map[string]*Player
	Lasers        map[uuid.UUID]Laser
	Mux           sync.RWMutex
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
		Lasers:        make(map[uuid.UUID]Laser),
	}
	return &game
}

// Start begins the main game loop, which waits for new actions and updates the
// game state occordinly.
func (game *Game) Start() {
	// Read actions from the channel.
	go func() {
		for {
			action := <-game.ActionChannel
			action.Perform(game)
		}
	}()
	// Respond to laser collisions.
	go func() {
		for {
			game.Mux.Lock()
			for id, laser := range game.Lasers {
				laserPosition := laser.GetPosition()
				didCollide := false
				for _, player := range game.Players {
					player.Mux.Lock()
					if player.Position.X == laserPosition.X && player.Position.Y == laserPosition.Y {
						didCollide = true
						player.Position.X = 0
						player.Position.Y = 0
						change := PlayerKilledChange{
							PlayerName:    player.Name,
							SpawnPosition: player.Position,
						}
						player.Mux.Unlock()
						select {
						case game.ChangeChannel <- change:
							// no-op.
						default:
							// no-op.
						}
					} else {
						player.Mux.Unlock()
					}
				}
				if didCollide {
					delete(game.Lasers, id)
					change := LaserRemoveChange{
						UUID: id,
					}
					select {
					case game.ChangeChannel <- change:
						// no-op.
					default:
						// no-op.
					}
				}
			}
			game.Mux.Unlock()
			time.Sleep(time.Millisecond * 20)
		}
	}()
}

func (game *Game) GetPlayer(playerName string) *Player {
	game.Mux.RLock()
	defer game.Mux.RUnlock()
	player, ok := game.Players[playerName]
	if !ok {
		return nil
	}
	return player
}

func (game *Game) CheckLastActionTime(actionKey string, throttle int) bool {
	lastAction, ok := game.LastAction[actionKey]
	if ok && lastAction.After(time.Now().Add(-1*time.Duration(throttle)*time.Millisecond)) {
		return false
	}
	return true
}

func (game *Game) UpdateLastActionTime(actionKey string) {
	game.Mux.Lock()
	defer game.Mux.Unlock()
	game.LastAction[actionKey] = time.Now()
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

type PlayerState int

const (
	PlayerAlive PlayerState = iota
	PlayerDead
)

// Player contains information unique to local and remote players.
type Player struct {
	Position Coordinate
	Name     string
	Icon     rune
	State    PlayerState
	Mux      sync.RWMutex
}

type Laser struct {
	InitialPosition Coordinate
	Direction       Direction
	StartTime       time.Time
}

func (laser Laser) GetPosition() Coordinate {
	difference := time.Now().Sub(laser.StartTime)
	moves := int(math.Floor(float64(difference.Milliseconds()) / float64(20)))
	position := laser.InitialPosition
	switch laser.Direction {
	case DirectionUp:
		position.Y -= moves
	case DirectionDown:
		position.Y += moves
	case DirectionLeft:
		position.X -= moves
	case DirectionRight:
		position.X += moves
	}
	return position
}

// Change is sent by the game engine in response to Actions.
type Change interface{}

// PositionChange is sent when the game engine moves a player.
type PositionChange struct {
	Change
	PlayerName string
	Direction  Direction
	Position   Coordinate
}

type PlayerKilledChange struct {
	Change
	PlayerName    string
	SpawnPosition Coordinate
}

type LaserChange struct {
	Change
	UUID  uuid.UUID
	Laser Laser
}

type LaserRemoveChange struct {
	Change
	UUID uuid.UUID
}

// Action is sent by the client when attempting to change game state. The
// engine can choose to reject Actions if they are invalid or performed too
// frequently.
type Action interface {
	Perform(game *Game)
}

// MoveAction is sent when a user presses an arrow key.
type MoveAction struct {
	Direction  Direction
	PlayerName string
}

// Perform contains backend logic required to move a player.
func (action MoveAction) Perform(game *Game) {
	player := game.GetPlayer(action.PlayerName)
	if player == nil {
		return
	}
	actionKey := fmt.Sprintf("%T_%s", action, action.PlayerName)
	if !game.CheckLastActionTime(actionKey, 50) {
		return
	}
	player.Mux.Lock()
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
	// Inform the client that the player moved.
	change := PositionChange{
		PlayerName: player.Name,
		Direction:  action.Direction,
		Position:   player.Position,
	}
	player.Mux.Unlock()
	select {
	case game.ChangeChannel <- change:
		// no-op.
	default:
		// no-op.
	}
	game.UpdateLastActionTime(actionKey)
}

type LaserAction struct {
	Direction  Direction
	PlayerName string
}

func (action LaserAction) Perform(game *Game) {
	player := game.GetPlayer(action.PlayerName)
	if player == nil {
		return
	}
	actionKey := fmt.Sprintf("%T_%s", action, action.PlayerName)
	if !game.CheckLastActionTime(actionKey, 500) {
		return
	}
	player.Mux.RLock()
	laser := Laser{
		InitialPosition: player.Position,
		StartTime:       time.Now(),
		Direction:       action.Direction,
	}
	player.Mux.RUnlock()
	// Initialize the laser to the side of the player.
	switch action.Direction {
	case DirectionUp:
		laser.InitialPosition.Y--
	case DirectionDown:
		laser.InitialPosition.Y++
	case DirectionLeft:
		laser.InitialPosition.X--
	case DirectionRight:
		laser.InitialPosition.X++
	}
	game.Mux.Lock()
	laserUUID := uuid.New()
	game.Lasers[laserUUID] = laser
	game.Mux.Unlock()
	change := LaserChange{
		Laser: laser,
		UUID:  laserUUID,
	}
	select {
	case game.ChangeChannel <- change:
		// no-op.
	default:
		// no-op.
	}
	game.UpdateLastActionTime(actionKey)
}
