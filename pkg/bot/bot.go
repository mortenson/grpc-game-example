package bot

import (
	"time"

	"github.com/beefsack/go-astar"
	"github.com/google/uuid"
	"github.com/mortenson/grpc-game-example/pkg/backend"
)

type world struct {
	tiles map[backend.Coordinate]*tile
}

type tileKind int

const (
	tileWall tileKind = iota
	tileNone
)

type tile struct {
	position backend.Coordinate
	world    *world
	kind     tileKind
}

func (t *tile) PathNeighbors() []astar.Pather {
	neighbors := []astar.Pather{}
	for _, difference := range []backend.Coordinate{
		backend.Coordinate{X: -1, Y: 0},
		backend.Coordinate{X: 1, Y: 0},
		backend.Coordinate{X: 0, Y: -1},
		backend.Coordinate{X: 0, Y: 1},
	} {
		position := t.position.Add(difference)
		neighbor, ok := t.world.tiles[position]
		if ok && neighbor.kind == tileNone {
			neighbors = append(neighbors, neighbor)
		}
	}
	return neighbors
}

func (t *tile) PathNeighborCost(to astar.Pather) float64 {
	return 1
}

func (t *tile) PathEstimatedCost(to astar.Pather) float64 {
	toT := to.(*tile)
	return float64(backend.Distance(t.position, toT.position))
}

type bot struct {
	playerID uuid.UUID
}

type Bots struct {
	bots []*bot
	game *backend.Game
	done chan bool
}

func NewBots(game *backend.Game) *Bots {
	return &Bots{
		game: game,
		bots: make([]*bot, 0),
		done: make(chan bool),
	}
}

func (bots *Bots) AddBot(name string) {
	playerID := uuid.New()
	player := &backend.Player{
		Name:            name,
		Icon:            'b',
		IdentifierBase:  backend.IdentifierBase{playerID},
		CurrentPosition: backend.Coordinate{X: -1, Y: 9},
	}
	bots.game.Mu.Lock()
	bots.game.AddEntity(player)
	bots.game.Mu.Unlock()
	bots.bots = append(bots.bots, &bot{playerID: playerID})
}

func getShootDirection(world *world, c1 backend.Coordinate, c2 backend.Coordinate) backend.Direction {
	direction := backend.DirectionStop
	diffCoordinate := backend.Coordinate{
		X: 0,
		Y: 0,
	}
	if c1.X == c2.X {
		if c2.Y < c1.Y {
			diffCoordinate.Y = -1
			direction = backend.DirectionUp
		} else if c2.Y > c1.Y {
			diffCoordinate.Y = 1
			direction = backend.DirectionDown
		}
	} else if c1.Y == c2.Y {
		if c2.X < c1.X {
			diffCoordinate.X = -1
			direction = backend.DirectionLeft
		} else if c2.X > c1.X {
			diffCoordinate.X = 1
			direction = backend.DirectionRight
		}
	}
	if direction == backend.DirectionStop {
		return direction
	}
	newPosition := c1.Add(diffCoordinate)
	for {
		if newPosition == c2 {
			break
		}
		tile, ok := world.tiles[newPosition]
		if ok && tile.kind == tileWall {
			return backend.DirectionStop
		}
		newPosition = newPosition.Add(diffCoordinate)
	}
	return direction
}

func (bots *Bots) Start() {
	go func() {
		world := &world{
			tiles: make(map[backend.Coordinate]*tile),
		}
		for symbol, positions := range bots.game.GetMapSymbols() {
			for _, position := range positions {
				if symbol == '█' {
					world.tiles[position] = &tile{
						position: position,
						world:    world,
						kind:     tileWall,
					}
				} else {
					world.tiles[position] = &tile{
						position: position,
						world:    world,
						kind:     tileNone,
					}
				}
			}
		}
		for {
			bots.game.Mu.RLock()
			// Get all player positions.
			playerPositions := make(map[uuid.UUID]backend.Coordinate, 0)
			for _, entity := range bots.game.Entities {
				switch entity.(type) {
				case *backend.Player:
					player := entity.(*backend.Player)
					playerPositions[entity.ID()] = player.Position()
				}
			}
			for _, bot := range bots.bots {
				player := bots.game.GetEntity(bot.playerID).(*backend.Player)
				playerPosition := player.Position()
				// Find the closest position.
				closestPosition := backend.Coordinate{}
				move := false
				shootDirection := backend.DirectionStop
				shoot := false
				for id, position := range playerPositions {
					if id == player.ID() {
						continue
					}
					shootDirection = getShootDirection(world, playerPosition, position)
					if shootDirection != backend.DirectionStop {
						shoot = true
						break
					}
					if !move || (backend.Distance(position, playerPosition) < backend.Distance(closestPosition, playerPosition)) {
						closestPosition = position
						move = true
					}
				}
				if shoot {
					bots.game.ActionChannel <- backend.LaserAction{
						ID:        uuid.New(),
						OwnerID:   player.ID(),
						Direction: shootDirection,
						Created:   time.Now(),
					}
					continue
				}
				if !move {
					continue
				}
				fromTile, ok := world.tiles[playerPosition]
				if !ok {
					continue
				}
				toTile, ok := world.tiles[closestPosition]
				if !ok {
					continue
				}
				path, _, found := astar.Path(toTile, fromTile)
				if !found {
					continue
				}
				var moveTowards backend.Coordinate
				if len(path) > 1 {
					moveTowards = path[1].(*tile).position
				} else {
					moveTowards = path[0].(*tile).position
				}
				xDiff := moveTowards.X - playerPosition.X
				yDiff := moveTowards.Y - playerPosition.Y
				direction := backend.DirectionStop
				if xDiff < 0 {
					direction = backend.DirectionLeft
				} else if xDiff > 0 {
					direction = backend.DirectionRight
				} else if yDiff < 0 {
					direction = backend.DirectionUp
				} else if yDiff > 0 {
					direction = backend.DirectionDown
				}
				if direction == backend.DirectionStop {
					continue
				}
				bots.game.ActionChannel <- backend.MoveAction{
					ID:        player.ID(),
					Direction: direction,
					Created:   time.Now(),
				}
			}
			bots.game.Mu.RUnlock()
			time.Sleep(time.Millisecond * 200)
		}
		<-bots.done
	}()
}

func (bots *Bots) Stop() {
	bots.done <- true
}
