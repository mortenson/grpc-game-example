package backend

import "github.com/google/uuid"

// Player contains information unique to local and remote players.
type Player struct {
	IdentifierBase
	Positioner
	Mover
	position Coordinate
	Name     string
	Icon     rune
}

func (p *Player) Position() Coordinate {
	return p.position
}

func (p *Player) Move(c Coordinate) {
	p.position = c
}

type PlayerKilledChange struct {
	Change
	ID            uuid.UUID
	SpawnPosition Coordinate
}
