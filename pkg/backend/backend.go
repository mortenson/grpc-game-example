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
	collisionCheckFrequency = 10 * time.Millisecond
	moveThrottle            = 100 * time.Millisecond
	laserThrottle           = 500 * time.Millisecond
	laserSpeed              = 50
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
	spawnPointIndex int
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
		spawnPointIndex: 0,
	}
	return &game
}

// Start begins the main game loop, which waits for new actions and updates the
// game state occordinly.
func (game *Game) Start() {
	go game.watchActions()
	go game.watchCollisions()
}

// watchActions waits for new actions to come in and performs them.
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

// watchCollisions checks for entity collisions - al we care about now is when
// a laser and a player collide but this could probably be more generalized.
func (game *Game) watchCollisions() {
	for {
		game.Mu.Lock()
		spawnPoints := game.GetMapByType()[MapTypeSpawn]
		for _, entities := range game.getCollisionMap() {
			if len(entities) <= 1 {
				continue
			}
			// Get the first laser, if present.
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
					// Choose the next spawn point.
					spawnPoint := spawnPoints[game.spawnPointIndex%len(spawnPoints)]
					game.spawnPointIndex++
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
		for _, wall := range game.GetMapByType()[MapTypeWall] {
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

// getCollisionMap maps coordinates to sets of entities.
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

// AddEntity adds an entity to the game.
func (game *Game) AddEntity(entity Identifier) {
	game.Entities[entity.ID()] = entity
}

// UpdateEntity updates an entity.
func (game *Game) UpdateEntity(entity Identifier) {
	game.Entities[entity.ID()] = entity
}

// GetEntity gets an entity from the game.
func (game *Game) GetEntity(id uuid.UUID) Identifier {
	return game.Entities[id]
}

// RemoveEntity removes an entity from the game.
func (game *Game) RemoveEntity(id uuid.UUID) {
	delete(game.Entities, id)
}

// startNewRound resets the game state in order to
// start a new round.
func (game *Game) startNewRound() {
	game.WaitForRound = false
	game.Score = map[uuid.UUID]int{}
	i := 0
	spawnPoints := game.GetMapByType()[MapTypeSpawn]
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

// queueNewRound queues a new round to start.
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

// AddScore increments an entity's score.
func (game *Game) AddScore(id uuid.UUID) {
	game.Score[id]++
}

// checkLastActionTime checks the last time an action was performed.
func (game *Game) checkLastActionTime(actionKey string, created time.Time, throttle time.Duration) bool {
	lastAction, ok := game.lastAction[actionKey]
	if ok && lastAction.After(created.Add(-1*throttle)) {
		return false
	}
	return true
}

// updateLastActionTime sets the last action time.
// The actionKey should be unique to the action and the actor (entity).
func (game *Game) updateLastActionTime(actionKey string, created time.Time) {
	game.lastAction[actionKey] = created
}

// sendChange sends a change to the change channel.
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

// Add adds two coordinates.
func (c1 Coordinate) Add(c2 Coordinate) Coordinate {
	return Coordinate{
		X: c1.X + c2.X,
		Y: c1.Y + c2.Y,
	}
}

// Distance calculates the distance between two coordinates.
func (c1 Coordinate) Distance(c2 Coordinate) int {
	return int(math.Sqrt(math.Pow(float64(c2.X-c1.X), 2) + math.Pow(float64(c2.Y-c1.Y), 2)))
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

// Identifier is an entity that provides an ID method.
type Identifier interface {
	ID() uuid.UUID
}

// Positioner is an entity that has a position.
type Positioner interface {
	Position() Coordinate
}

// Mover is an entity that can be moved.
type Mover interface {
	Move(Coordinate)
}

// IdentifierBase is embedded to satisfy the Identifier interface.
type IdentifierBase struct {
	UUID uuid.UUID
}

// ID returns the UUID of an entity.
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

// RoundOverChange indicates that a round is over. Information about the new
// round should be retrieved from the game instance.
type RoundOverChange struct {
	Change
}

// RoundStartChange indicates that a round has started.
type RoundStartChange struct {
	Change
}

// AddEntityChange occurs when an entity is added in response to an action.
// Currently this is only used for new lasers and players joining the game.
type AddEntityChange struct {
	Change
	Entity Identifier
}

// RemoveEntityChange occurs when an entity has been removed from the game.
type RemoveEntityChange struct {
	Change
	Entity Identifier
}

// PlayerRespawnChange occurs when a player has been killed and is respawning.
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
	Created   time.Time
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
	if !game.checkLastActionTime(actionKey, action.Created, moveThrottle) {
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
	for _, wall := range game.GetMapByType()[MapTypeWall] {
		if position == wall {
			return
		}
	}
	// Check if position collides with a player.
	collidingEntities, ok := game.getCollisionMap()[position]
	if ok {
		for _, entity := range collidingEntities {
			_, ok := entity.(*Player)
			if ok {
				return
			}
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
	game.updateLastActionTime(actionKey, action.Created)
}
