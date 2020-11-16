package gol

import (
	//"fmt"
	"strconv"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFileName chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

// Sends the file name to io.go so the world can be initialised
func sendFileName(imageWidth int, imageHeight int, ioCommand chan<- ioCommand, ioFileName chan<- string) {
	ioCommand <- ioInput
	fileName := strconv.Itoa(imageWidth) + "x" + strconv.Itoa(imageHeight)
	ioFileName <- fileName
}

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
						X: y,
						Y: x,
					},
				}
			}
		}
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
func calculateLiveNeighbours(neighbours []byte) int {
	liveNeighbours := 0
	for _, neighbour := range neighbours {
		if neighbour == 255 {
			liveNeighbours += 1
		}
	}
	return liveNeighbours
}

// Returns the new value of a cell given its current value and number of live neighbours
func calculateValue(item byte, liveNeighbours int) byte {
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
func calculateNextState(world [][]byte, events chan<- Event) [][]byte {
	var nextWorld [][]byte
	for y, row := range world {
		nextWorld = append(nextWorld, []byte{})
		for x, element := range row {
			neighbours := getNeighbours(world, y, x)
			liveNeighbours := calculateLiveNeighbours(neighbours)
			value := calculateValue(element, liveNeighbours)
			nextWorld[y] = append(nextWorld[y], value)
			if value != world[y][x] {
				events <- CellFlipped{
					CompletedTurns: 0,
					Cell: util.Cell{
						X: y,
						Y: x,
					},
				}
			}
		}
	}
	return nextWorld
}

// Distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	// TODO: Create a 2D slice to store the world.
	// TODO: For all initially alive cells send a CellFlipped Event.
	// TODO: Execute all turns of the Game of Life.
	// TODO: Send correct Events when required, e.g. CellFlipped, TurnComplete and FinalTurnComplete.
	//		 See event.go for a list of all events.
	sendFileName(p.ImageWidth, p.ImageWidth, c.ioCommand, c.ioFileName)
	world := initialiseWorld(p.ImageHeight, p.ImageWidth, c.ioInput, c.events)
	var turn int
	for turn = 0; turn < p.Turns; turn++ {
		world = calculateNextState(world, c.events)
		c.events <- TurnComplete{
			CompletedTurns: turn,
		}
	}
	var aliveCells []util.Cell
	for y, row := range world {
		for x, element := range row {
			if element == 255 {
				aliveCells = append(aliveCells, util.Cell{X: x, Y: y})
			}
		}
	}
	c.events <- FinalTurnComplete{
		CompletedTurns: turn,
		Alive:          aliveCells,
	}
	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}
