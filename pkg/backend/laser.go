package backend

import (
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

// Laser is an entity that is fired by players.
type Laser struct {
	IdentifierBase
	Positioner
	InitialPosition Coordinate
	Direction       Direction
	StartTime       time.Time
	OwnerID         uuid.UUID
}

// Position returns the laser position, which is calculated at runtime based on
// when the laser was fired.
func (laser *Laser) Position() Coordinate {
	difference := time.Now().Sub(laser.StartTime)
	moves := int(math.Floor(float64(difference.Milliseconds()) / float64(laserSpeed)))
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

// LaserAction is sent when a laser is fired.
type LaserAction struct {
	Direction Direction
	ID        uuid.UUID
	OwnerID   uuid.UUID
	Created   time.Time
}

// Perform spawns a laser next to the player who fired it.
func (action LaserAction) Perform(game *Game) {
	entity := game.GetEntity(action.OwnerID)
	if entity == nil {
		return
	}
	actionKey := fmt.Sprintf("%T:%s", action, entity.ID().String())
	if !game.checkLastActionTime(actionKey, action.Created, laserThrottle) {
		return
	}
	laser := Laser{
		InitialPosition: entity.(Positioner).Position(),
		StartTime:       action.Created,
		Direction:       action.Direction,
		IdentifierBase:  IdentifierBase{action.ID},
		OwnerID:         action.OwnerID,
	}
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
	game.AddEntity(&laser)
	change := AddEntityChange{
		Entity: &laser,
	}
	game.sendChange(change)
	game.updateLastActionTime(actionKey, action.Created)
}
