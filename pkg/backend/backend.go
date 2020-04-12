package backend

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Game is the backend engine for the game. It can be used regardless of how
// game data is rendered, or if a game server is being used.
type Game struct {
	Entities        map[uuid.UUID]Identifier
	Map             [][]rune
	Mu              sync.RWMutex
	ChangeChannel   chan Change
	ActionChannel   chan Action
	LastAction      map[string]time.Time
	Score           map[uuid.UUID]int
	NewRoundAt      time.Time
	RoundWinner     uuid.UUID
	WaitForRound    bool
	IsAuthoritative bool
}

// NewGame constructs a new Game struct.
func NewGame() *Game {
	game := Game{
		Entities:        make(map[uuid.UUID]Identifier),
		ActionChannel:   make(chan Action, 1),
		LastAction:      make(map[string]time.Time),
		ChangeChannel:   make(chan Change, 1),
		IsAuthoritative: true,
		WaitForRound:    false,
		Score:           make(map[uuid.UUID]int),
		Map:             MapDefault,
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
			if game.WaitForRound {
				continue
			}
			game.Mu.Lock()
			action.Perform(game)
			game.Mu.Unlock()
		}
	}()
	// If the game isn't authoritative, some other system determines when
	// players are hit.
	if !game.IsAuthoritative {
		return
	}
	// Check for player death.
	go func() {
		for {
			game.Mu.Lock()
			// Build a map of all entity positions.
			collisionMap := make(map[Coordinate][]Identifier)
			for _, entity := range game.Entities {
				positioner, ok := entity.(Positioner)
				if !ok {
					continue
				}
				position := positioner.Position()
				collisionMap[position] = append(collisionMap[position], entity)
			}
			// Check if any collide.
			for _, entities := range collisionMap {
				if len(entities) <= 1 {
					continue
				}
				// Get the first laser, if present.
				// @todo Make this generic is there are more "this kills you"
				// entity types.
				hasLaser := false
				var laserOwnerID uuid.UUID
				for _, entity := range entities {
					laser, ok := entity.(*Laser)
					if ok {
						hasLaser = true
						laserOwnerID = laser.OwnerID
						break
					}
				}
				if !hasLaser {
					continue
				}
				// Handle entities that collided with the laser.
				for _, entity := range entities {
					switch entity.(type) {
					case *Player:
						player := entity.(*Player)
						game.Move(player.ID(), Coordinate{X: 0, Y: 0})
						change := PlayerRespawnChange{
							Player: player,
						}
						select {
						case game.ChangeChannel <- change:
						default:
						}
						if player.ID() != laserOwnerID {
							game.AddScore(laserOwnerID)
						}
					default:
						change := RemoveEntityChange{
							Entity: entity,
						}
						select {
						case game.ChangeChannel <- change:
						default:
						}
						game.RemoveEntity(entity.ID())
					}
				}
			}
			// Remove lasers that hit walls. Players _should_ never collide
			// with walls (this is how you get out of bounds gltiches...)
			for _, wall := range game.GetMapWalls() {
				entities, ok := collisionMap[wall]
				if !ok {
					continue
				}
				for _, entity := range entities {
					switch entity.(type) {
					case *Laser:
						change := RemoveEntityChange{
							Entity: entity,
						}
						select {
						case game.ChangeChannel <- change:
						default:
						}
						game.RemoveEntity(entity.ID())
					}
				}
			}
			game.Mu.Unlock()
			time.Sleep(time.Millisecond * 20)
		}
	}()
}

func (game *Game) GetMapWalls() []Coordinate {
	mapCenterX := len(game.Map[0]) / 2
	mapCenterY := len(game.Map) / 2
	walls := make([]Coordinate, 0)
	for mapY, row := range game.Map {
		for mapX, col := range row {
			if col != '█' {
				continue
			}
			walls = append(walls, Coordinate{
				X: mapX - mapCenterX,
				Y: mapY - mapCenterY,
			})
		}
	}
	return walls
}

func (game *Game) AddEntity(entity Identifier) {
	game.Entities[entity.ID()] = entity
}

func (game *Game) UpdateEntity(entity Identifier) {
	// @todo is replacing the struct bad?
	game.Entities[entity.ID()] = entity
}

func (game *Game) GetEntity(id uuid.UUID) Identifier {
	return game.Entities[id]
}

func (game *Game) RemoveEntity(id uuid.UUID) {
	delete(game.Entities, id)
}

func (game *Game) AddScore(id uuid.UUID) {
	game.Score[id]++
	if game.Score[id] >= 10 {
		game.Score = make(map[uuid.UUID]int)
		game.WaitForRound = true
		game.NewRoundAt = time.Now().Add(time.Second * 10)
		game.RoundWinner = id
		// @todo add wait for round change
		go func() {
			time.Sleep(time.Second * 10)
			game.Mu.Lock()
			game.WaitForRound = false
			game.Mu.Unlock()
			// @todo add start round change
		}()
	}
}

func (game *Game) Move(id uuid.UUID, position Coordinate) {
	game.Entities[id].(Mover).Move(position)
}

func (game *Game) CheckLastActionTime(actionKey string, throttle int) bool {
	lastAction, ok := game.LastAction[actionKey]
	if ok && lastAction.After(time.Now().Add(-1*time.Duration(throttle)*time.Millisecond)) {
		return false
	}
	return true
}

func (game *Game) UpdateLastActionTime(actionKey string) {
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

type Identifier interface {
	ID() uuid.UUID
}

type Positioner interface {
	Position() Coordinate
}

type Mover interface {
	Move(Coordinate)
}

type IdentifierBase struct {
	UUID uuid.UUID
}

func (e IdentifierBase) ID() uuid.UUID {
	return e.UUID
}

// Change is sent by the game engine in response to Actions.
type Change interface{}

// MoveChange is sent when the game engine moves an entity.
type MoveChange struct {
	Change
	Entity    Identifier
	Direction Direction
	Position  Coordinate
}

type AddEntityChange struct {
	Change
	Entity Identifier
}

type RemoveEntityChange struct {
	Change
	Entity Identifier
}

type PlayerRespawnChange struct {
	Change
	Player *Player
}

// Action is sent by the client when attempting to change game state. The
// engine can choose to reject Actions if they are invalid or performed too
// frequently.
type Action interface {
	Perform(game *Game)
}

// MoveAction is sent when a user presses an arrow key.
type MoveAction struct {
	Direction Direction
	ID        uuid.UUID
}

// Perform contains backend logic required to move an entity.
func (action MoveAction) Perform(game *Game) {
	entity := game.GetEntity(action.ID)
	if entity == nil {
		return
	}
	actionKey := fmt.Sprintf("%T:%s", action, entity.ID().String())
	if !game.CheckLastActionTime(actionKey, 50) {
		return
	}
	position := entity.(Positioner).Position()
	// Move the entity.
	switch action.Direction {
	case DirectionUp:
		position.Y--
	case DirectionDown:
		position.Y++
	case DirectionLeft:
		position.X--
	case DirectionRight:
		position.X++
	}
	// Check if position collides with a wall.
	for _, wall := range game.GetMapWalls() {
		if position == wall {
			return
		}
	}
	game.Move(entity.ID(), position)
	// Inform the client that the entity moved.
	change := MoveChange{
		Entity:    entity,
		Direction: action.Direction,
		Position:  position,
	}
	select {
	case game.ChangeChannel <- change:
	default:
	}
	game.UpdateLastActionTime(actionKey)
}
