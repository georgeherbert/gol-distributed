package gol

import (
	"sync"
	"time"
	"uk.ac.bris.cs/gameoflife/util"
)

// Returns the world with its initial values filled
func initialiseWorld(height int, width int, ioInput <-chan uint8, events chan<- Event) [][]byte {
	world := make([][]byte, height)
	for y := range world {
		world[y] = make([]byte, width)
	}
	for y, row := range world {
		for x := range row {
			cell := <-ioInput
			world[y][x] = cell
			if cell == 255 {
				events <- CellFlipped{
					CompletedTurns: 0,
					Cell: util.Cell{
						X: x,
						Y: y,
					},
				}
			}
		}
	}
	events <- TurnComplete{
		CompletedTurns: 0,
	}
	return world
}

// Returns the neighbours of a cell at given coordinates
func getNeighbours(world [][]byte, row int, column int) []byte {
	rowAbove, rowBelow := row - 1, row + 1
	if row == 0 {
		rowAbove = len(world[0]) - 1
	} else if row == len(world[0]) - 1 {
		rowBelow = 0
	}
	columnLeft, columnRight := column - 1, column + 1
	if column == 0 {
		columnLeft = len(world[0]) - 1
	} else if column == len(world[0]) - 1 {
		columnRight = 0
	}
	neighbours := []byte{world[rowAbove][columnLeft], world[rowAbove][column], world[rowAbove][columnRight],
		world[row][columnLeft], world[row][columnRight], world[rowBelow][columnLeft], world[rowBelow][column],
		world[rowBelow][columnRight]}
	return neighbours
}

// Returns the number of live neighbours from a set of neighbours
func calcLiveNeighbours(neighbours []byte) int {
	liveNeighbours := 0
	for _, neighbour := range neighbours {
		if neighbour == 255 {
			liveNeighbours += 1
		}
	}
	return liveNeighbours
}

// Returns the new value of a cell given its current value and number of live neighbours
func calcValue(item byte, liveNeighbours int) byte {
	calculatedValue := byte(0)
	if item == 255 {
		if liveNeighbours == 2 || liveNeighbours == 3 {
			calculatedValue = byte(255)
		}
	} else {
		if liveNeighbours == 3 {
			calculatedValue = byte(255)
		}
	}
	return calculatedValue
}

// Returns the next state of a world given the current state
func calcNextState(world [][]byte, events chan<- Event, turn int) [][]byte {
	var nextWorld [][]byte
	for y, row := range world {
		nextWorld = append(nextWorld, []byte{})
		for x, element := range row {
			neighbours := getNeighbours(world, y, x)
			liveNeighbours := calcLiveNeighbours(neighbours)
			value := calcValue(element, liveNeighbours)
			nextWorld[y] = append(nextWorld[y], value)
			if value != world[y][x] {
				events <- CellFlipped{
					CompletedTurns: turn,
					Cell: util.Cell{
						X: x,
						Y: y,
					},
				}
			}
		}
	}
	return nextWorld
}

// Returns the number of alive cells in a world
func calcNumAliveCells(world [][]byte) int {
	total := 0
	for _, row := range world {
		for _, element := range row {
			if element == 255 {
				total += 1
			}
		}
	}
	return total
}

// Distributor divides the work between workers and interacts with other goroutines.
func gol(imageHeight int, imageWidth int, turns int, ioInput <-chan uint8, events chan<- Event) ([][]byte, int) {
	world := initialiseWorld(imageHeight, imageWidth, ioInput, events)
	var turn int
	var completedTurns int
	mutexTurnsWorld := &sync.Mutex{}
	ticker := time.NewTicker(2 * time.Second)
	// Ticker
	go func() {
		for {
			<-ticker.C
			mutexTurnsWorld.Lock()
			events <- AliveCellsCount{
				CompletedTurns: completedTurns,
				CellsCount:     calcNumAliveCells(world),
			}
			mutexTurnsWorld.Unlock()
		}
	}()
	for turn = 0; turn < turns; turn++ {
		mutexTurnsWorld.Lock()
		world = calcNextState(world, events, turn)
		completedTurns = turn + 1
		mutexTurnsWorld.Unlock()
		events <- TurnComplete{
			CompletedTurns: completedTurns,
		}
	}
	return world, completedTurns
}
