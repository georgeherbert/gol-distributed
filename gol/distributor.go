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
func sendFileName(fileName string, ioCommand chan<- ioCommand, ioFileName chan<- string) {
	ioCommand <- ioInput
	ioFileName <- fileName
}

// Reports the number of alive cells every time it receives data
func reportAliveCells(events chan<- Event, turnsChan <-chan int, aliveCellsChan <-chan int) {
	for {
		turns := <-turnsChan
		aliveCells := <-aliveCellsChan
		events <- AliveCellsCount{
			CompletedTurns: turns,
			CellsCount:     aliveCells,
		}
	}
}

// Returns a slice of alive cells
func getAliveCells(world [][]byte) []util.Cell {
	var aliveCells []util.Cell
	for y, row := range world {
		for x, element := range row {
			if element == 255 {
				aliveCells = append(aliveCells, util.Cell{X: x, Y: y})
			}
		}
	}
	return aliveCells
}

// Distributor divides the work between workers and interacts with other goroutines.
func controller(p Params, c distributorChannels) {
	fileName := strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight)
	sendFileName(fileName, c.ioCommand, c.ioFileName)
	turnsChan, aliveCellsChan := make(chan int), make(chan int)
	go reportAliveCells(c.events, turnsChan, aliveCellsChan)
	world, completedTurns := gol(p.ImageHeight, p.ImageWidth, p.Turns, c.ioInput, turnsChan, aliveCellsChan)
	aliveCells := getAliveCells(world)
	c.events <- FinalTurnComplete{
		CompletedTurns: completedTurns,
		Alive:          aliveCells,
	}
	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
	c.events <- StateChange{completedTurns, Quitting}
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}
