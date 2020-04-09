package proto

import (
	"log"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/uuid"
	"github.com/mortenson/grpc-game-example/pkg/backend"
)

func GetBackendDirection(protoDirection Direction) backend.Direction {
	direction := backend.DirectionStop
	switch protoDirection {
	case Direction_UP:
		direction = backend.DirectionUp
	case Direction_DOWN:
		direction = backend.DirectionDown
	case Direction_LEFT:
		direction = backend.DirectionLeft
	case Direction_RIGHT:
		direction = backend.DirectionRight
	}
	return direction
}

func GetProtoDirection(direction backend.Direction) Direction {
	protoDirection := Direction_STOP
	switch direction {
	case backend.DirectionUp:
		protoDirection = Direction_UP
	case backend.DirectionDown:
		protoDirection = Direction_DOWN
	case backend.DirectionLeft:
		protoDirection = Direction_LEFT
	case backend.DirectionRight:
		protoDirection = Direction_RIGHT
	}
	return protoDirection
}

func GetBackendCoordinate(protoCoordinate *Coordinate) backend.Coordinate {
	return backend.Coordinate{
		X: int(protoCoordinate.X),
		Y: int(protoCoordinate.Y),
	}
}

func GetProtoCoordinate(coordinate backend.Coordinate) *Coordinate {
	return &Coordinate{
		X: int32(coordinate.X),
		Y: int32(coordinate.Y),
	}
}

func GetBackendEntity(protoEntity *Entity) backend.Identifier {
	switch protoEntity.Entity.(type) {
	case *Entity_Player:
		protoPlayer := protoEntity.Entity.(*Entity_Player).Player
		entityID, err := uuid.Parse(protoPlayer.Id)
		if err != nil {
			// @todo handle
			return nil
		}
		player := &backend.Player{
			IdentifierBase: backend.IdentifierBase{UUID: entityID},
			Name:           protoPlayer.Name,
			Icon:           'P',
		}
		player.Move(GetBackendCoordinate(protoPlayer.Position))
		return player
	case *Entity_Laser:
		protoLaser := protoEntity.Entity.(*Entity_Laser).Laser
		entityID, err := uuid.Parse(protoLaser.Id)
		if err != nil {
			// @todo handle
			return nil
		}
		timestamp, err := ptypes.Timestamp(protoLaser.StartTime)
		if err != nil {
			// @todo handle
			return nil
		}
		laser := &backend.Laser{
			IdentifierBase:  backend.IdentifierBase{UUID: entityID},
			InitialPosition: GetBackendCoordinate(protoLaser.InitialPosition),
			Direction:       GetBackendDirection(protoLaser.Direction),
			StartTime:       timestamp,
		}
		return laser
	}
	log.Fatalf("Cannot get backend entity for %T -> %+v", protoEntity, protoEntity)
	return nil
}

func GetProtoEntity(entity backend.Identifier) *Entity {
	switch entity.(type) {
	case *backend.Player:
		player := entity.(*backend.Player)
		protoPlayer := Entity_Player{
			Player: &Player{
				Id:       player.ID().String(),
				Name:     player.Name,
				Position: GetProtoCoordinate(player.Position()),
			},
		}
		return &Entity{Entity: &protoPlayer}
	case *backend.Laser:
		laser := entity.(*backend.Laser)
		timestamp, err := ptypes.TimestampProto(laser.StartTime)
		if err != nil {
			// @todo handle
			return nil
		}
		protoLaser := Entity_Laser{
			Laser: &Laser{
				Id:              laser.ID().String(),
				StartTime:       timestamp,
				InitialPosition: GetProtoCoordinate(laser.InitialPosition),
				Direction:       GetProtoDirection(laser.Direction),
			},
		}
		return &Entity{Entity: &protoLaser}
	}
	log.Fatalf("Cannot get proto entity for %T -> %+v", entity, entity)
	return nil
}
