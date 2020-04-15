package backend

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	roundOverScore          = 10
	newRoundWaitTime        = 10 * time.Second
	collisionCheckFrequency = 20 * time.Millisecond
	moveThrottle            = 50 * time.Millisecond
	laserThrottle           = 500 * time.Millisecond
)

// Game is the backend engine for the game. It can be used regardless of how
// game data is rendered, or if a game server is being used.
type Game struct {
	Entities        map[uuid.UUID]Identifier
	gameMap         [][]rune
	Mu              sync.RWMutex
	ChangeChannel   chan Change
	ActionChannel   chan Action
	lastAction      map[string]time.Time
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
		lastAction:      make(map[string]time.Time),
		ChangeChannel:   make(chan Change, 1),
		IsAuthoritative: true,
		WaitForRound:    false,
		Score:           make(map[uuid.UUID]int),
		gameMap:         MapDefault,
	}
	return &game
}

// Start begins the main game loop, which waits for new actions and updates the
// game state occordinly.
func (game *Game) Start() {
	go game.watchActions()
	go game.watchCollisions()
}

func (game *Game) watchActions() {
	for {
		action := <-game.ActionChannel
		if game.WaitForRound {
			continue
		}
		game.Mu.Lock()
		action.Perform(game)
		game.Mu.Unlock()
	}
}

func (game *Game) getCollisionMap() map[Coordinate][]Identifier {
	collisionMap := map[Coordinate][]Identifier{}
	for _, entity := range game.Entities {
		positioner, ok := entity.(Positioner)
		if !ok {
			continue
		}
		position := positioner.Position()
		collisionMap[position] = append(collisionMap[position], entity)
	}
	return collisionMap
}

func (game *Game) watchCollisions() {
	for {
		game.Mu.Lock()
		for _, entities := range game.getCollisionMap() {
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
					// If the game isn't authoritative, another system decides
					// when players die and score is changed.
					if !game.IsAuthoritative {
						continue
					}
					player := entity.(*Player)
					// Don't allow players to kill themselves.
					if player.ID() == laserOwnerID {
						continue
					}
					spawnPoints := game.GetMapSpawnPoints()
					// Choose a spawn point furthest away from where the
					// player died.
					spawnPoint := spawnPoints[0]
					for _, sp := range game.GetMapSpawnPoints() {
						if distance(player.Position(), sp) > distance(player.Position(), spawnPoint) {
							spawnPoint = sp
						}
					}
					// For debugging.
					// spawnPoint = Coordinate{X: 0, Y: 0}
					player.Move(spawnPoint)
					change := PlayerRespawnChange{
						Player:     player,
						KilledByID: laserOwnerID,
					}
					game.sendChange(change)
					game.AddScore(laserOwnerID)
					if game.Score[laserOwnerID] >= roundOverScore {
						game.queueNewRound(laserOwnerID)
					}
				case *Laser:
					change := RemoveEntityChange{
						Entity: entity,
					}
					game.sendChange(change)
					game.RemoveEntity(entity.ID())
				}
			}
		}
		// Remove lasers that hit walls.
		collisionMap := game.getCollisionMap()
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
					game.sendChange(change)
					game.RemoveEntity(entity.ID())
				}
			}
		}
		game.Mu.Unlock()
		time.Sleep(collisionCheckFrequency)
	}
}

func (game *Game) AddEntity(entity Identifier) {
	game.Entities[entity.ID()] = entity
}

func (game *Game) UpdateEntity(entity Identifier) {
	game.Entities[entity.ID()] = entity
}

func (game *Game) GetEntity(id uuid.UUID) Identifier {
	return game.Entities[id]
}

func (game *Game) RemoveEntity(id uuid.UUID) {
	delete(game.Entities, id)
}

func (game *Game) startNewRound() {
	game.WaitForRound = false
	game.Score = map[uuid.UUID]int{}
	i := 0
	spawnPoints := game.GetMapSpawnPoints()
	for _, entity := range game.Entities {
		player, ok := entity.(*Player)
		if !ok {
			continue
		}
		player.Move(spawnPoints[i%len(spawnPoints)])
		i++
	}
	game.sendChange(RoundStartChange{})
}

func (game *Game) queueNewRound(roundWinner uuid.UUID) {
	game.WaitForRound = true
	game.NewRoundAt = time.Now().Add(newRoundWaitTime)
	game.RoundWinner = roundWinner
	game.sendChange(RoundOverChange{})
	go func() {
		time.Sleep(newRoundWaitTime)
		game.Mu.Lock()
		game.startNewRound()
		game.Mu.Unlock()
	}()
}

func (game *Game) AddScore(id uuid.UUID) {
	game.Score[id]++
}

func (game *Game) checkLastActionTime(actionKey string, throttle time.Duration) bool {
	lastAction, ok := game.lastAction[actionKey]
	if ok && lastAction.After(time.Now().Add(-1*throttle)) {
		return false
	}
	return true
}

func (game *Game) updateLastActionTime(actionKey string) {
	game.lastAction[actionKey] = time.Now()
}

func (game *Game) sendChange(change Change) {
	select {
	case game.ChangeChannel <- change:
	default:
	}
}

// Coordinate is used for all position-related variables.
type Coordinate struct {
	X int
	Y int
}

func distance(a Coordinate, b Coordinate) int {
	return int(math.Sqrt(math.Pow(float64(b.X-a.X), 2) + math.Pow(float64(b.Y-a.Y), 2)))
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

type RoundOverChange struct {
	Change
}

type RoundStartChange struct {
	Change
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
	Player     *Player
	KilledByID uuid.UUID
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
	mover, ok := entity.(Mover)
	if !ok {
		return
	}
	positioner, ok := entity.(Positioner)
	if !ok {
		return
	}
	actionKey := fmt.Sprintf("%T:%s", action, entity.ID().String())
	if !game.checkLastActionTime(actionKey, moveThrottle) {
		return
	}
	position := positioner.Position()
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
	mover.Move(position)
	// Inform the client that the entity moved.
	change := MoveChange{
		Entity:    entity,
		Direction: action.Direction,
		Position:  position,
	}
	game.sendChange(change)
	game.updateLastActionTime(actionKey)
}
