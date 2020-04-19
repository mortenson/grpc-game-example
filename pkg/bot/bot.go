package bot

import (
	"time"

	"github.com/beefsack/go-astar"
	"github.com/google/uuid"
	"github.com/mortenson/grpc-game-example/pkg/backend"
)

// bot controls a player in the game.
type bot struct {
	playerID uuid.UUID
}

// Bots controls all bots added to a game.
type Bots struct {
	bots []*bot
	game *backend.Game
}

// NewBots creates a new bots instance.
func NewBots(game *backend.Game) *Bots {
	return &Bots{
		game: game,
		bots: make([]*bot, 0),
	}
}

// AddBot adds a new bot to the game.
func (bots *Bots) AddBot(name string) *backend.Player {
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
	return player
}

// world tracks all game tiles and is used for astar traversal.
type world struct {
	tiles map[backend.Coordinate]*tile
}

// tileKind differentiates walls from normal tiles.
type tileKind int

const (
	tileWall tileKind = iota
	tileNone
)

// tile represents a point on the map.
type tile struct {
	position backend.Coordinate
	world    *world
	kind     tileKind
}

// PathNeighbors is used by beefsack/astar to traverse.
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

// PathNeighborCost is used by beefsack/astar to determine the cost of a move.
func (t *tile) PathNeighborCost(to astar.Pather) float64 {
	return 1
}

// PathEstimatedCost estimates the cost of moving between two points.
func (t *tile) PathEstimatedCost(to astar.Pather) float64 {
	toT := to.(*tile)
	return float64(t.position.Distance(toT.position))
}

// getShootDirection determines if a straight path between c1 and c2 is not
// blocked by a wall, and if not returns the direction of the path.
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

// Start starts the goroutine used to determine bot moves.
func (bots *Bots) Start() {
	go func() {
		world := &world{
			tiles: make(map[backend.Coordinate]*tile),
		}
		for symbol, positions := range bots.game.GetMapByType() {
			for _, position := range positions {
				if symbol == backend.MapTypeWall {
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
			bots.game.Mu.RUnlock()
			for _, bot := range bots.bots {
				bots.game.Mu.RLock()
				player := bots.game.GetEntity(bot.playerID).(*backend.Player)
				bots.game.Mu.RUnlock()
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
					// Check if we're on top of the player and move if so.
					if position == playerPosition {
						closestPosition = position.Add(backend.Coordinate{
							X: 1,
							Y: 1,
						})
						move = true
						break
					}
					// See if a player can be shot at.
					shootDirection = getShootDirection(world, playerPosition, position)
					if shootDirection != backend.DirectionStop {
						shoot = true
						break
					}
					// Find a close player to move to.
					if !move || (position.Distance(playerPosition) < closestPosition.Distance(playerPosition)) {
						closestPosition = position
						move = true
					}
				}
				// Shooting takes priority over moving.
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
				// Ensure that the tiles we're moving from/to exist.
				fromTile, ok := world.tiles[playerPosition]
				if !ok {
					continue
				}
				toTile, ok := world.tiles[closestPosition]
				if !ok {
					continue
				}
				// Find a path using the astar algorithm.
				path, _, found := astar.Path(toTile, fromTile)
				if !found {
					continue
				}
				// Move on the path.
				var moveTowards backend.Coordinate
				if len(path) > 1 {
					moveTowards = path[1].(*tile).position
				} else {
					moveTowards = path[0].(*tile).position
				}
				// Determine the direction to move to reach the point.
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
			time.Sleep(time.Millisecond * 200)
		}
	}()
}
