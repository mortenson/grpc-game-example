package backend

import (
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

type Laser struct {
	IdentifierBase
	Positioner
	InitialPosition Coordinate
	Direction       Direction
	StartTime       time.Time
	OwnerID         uuid.UUID
}

func (laser *Laser) Position() Coordinate {
	difference := time.Now().Sub(laser.StartTime)
	moves := int(math.Floor(float64(difference.Milliseconds()) / float64(21)))
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

type LaserAction struct {
	Direction Direction
	ID        uuid.UUID
	OwnerID   uuid.UUID
}

func (action LaserAction) Perform(game *Game) {
	entity := game.GetEntity(action.OwnerID)
	if entity == nil {
		return
	}
	actionKey := fmt.Sprintf("%T:%s", action, entity.ID().String())
	if !game.checkLastActionTime(actionKey, laserThrottle) {
		return
	}
	laser := Laser{
		InitialPosition: entity.(Positioner).Position(),
		StartTime:       time.Now(),
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
	game.updateLastActionTime(actionKey)
}
