package proto

import "github.com/mortenson/grpc-game-example/pkg/backend"

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
