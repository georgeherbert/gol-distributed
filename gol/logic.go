package gol

import (
	"sync"
	"time"
)

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
func calcNextState(world [][]byte) [][]byte {
	var nextWorld [][]byte
	for y, row := range world {
		nextWorld = append(nextWorld, []byte{})
		for x, element := range row {
			neighbours := getNeighbours(world, y, x)
			liveNeighbours := calcLiveNeighbours(neighbours)
			value := calcValue(element, liveNeighbours)
			nextWorld[y] = append(nextWorld[y], value)
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
func gol(world [][]byte, turns int, turnsChan chan<- int, aliveCellsChan chan<- int) ([][]byte, int) {
	var turn int
	var completedTurns int
	mutexTurnsWorld := &sync.Mutex{}
	ticker := time.NewTicker(2 * time.Second)
	// Ticker
	go func() {
		for {
			<-ticker.C
			mutexTurnsWorld.Lock()
			turnsChan <- completedTurns
			aliveCellsChan <- calcNumAliveCells(world)
			mutexTurnsWorld.Unlock()
		}
	}()
	for turn = 0; turn < turns; turn++ {
		mutexTurnsWorld.Lock()
		world = calcNextState(world)
		completedTurns = turn + 1
		mutexTurnsWorld.Unlock()
	}
	return world, completedTurns
}

//TODO remove all references to ioChannels, events channels, etc.








