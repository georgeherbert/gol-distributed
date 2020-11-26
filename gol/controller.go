package gol

import (
	"bufio"
	"fmt"
	"net"

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

// Returns the world with its initial values filled
func sendWorld(height int, width int, ioInput <-chan uint8, conn net.Conn) {
	for i := 0; i < height * width; i++ {
		fmt.Fprintf(conn, "%d\n", <-ioInput)
	}
}

// Reports the number of alive cells every time it receives data
func reportAliveCells(events chan<- Event, done chan<- bool, reader *bufio.Reader) {
	for {
		turnsString, _ := reader.ReadString('\n')
		if turnsString == "DONE\n" {
			done <- true
			break
		}
		aliveCellsString, _ := reader.ReadString('\n')
		turns := netStringToInt(turnsString)
		aliveCells := netStringToInt(aliveCellsString)
		events <- AliveCellsCount{
			CompletedTurns: turns,
			CellsCount:     aliveCells,
		}
	}
}

// Converts a string receives over tcp to an integer
func netStringToInt(netString string) int {
	integer, _ := strconv.Atoi(netString[:len(netString) - 1])
	return integer
}

func receiveWorld(height int, width int, reader *bufio.Reader) [][]byte {
	world := make([][]byte, height)
	for y := range world {
		world[y] = make([]byte, width)
	}
	for y, row := range world {
		for x := range row {
			msg, _ := reader.ReadString('\n')
			cell := netStringToInt(msg)
			world[y][x] = byte(cell)
		}
	}
	return world
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

// Writes to a file and sends the correct event
func writeFile(world [][]byte, fileName string, turns int, ioCommand chan<- ioCommand, ioFileName chan<- string,
	ioOutputChannel chan<- uint8, events chan<- Event) {
	outputFileName := fileName + "x" + strconv.Itoa(turns)
	ioCommand <- ioOutput
	ioFileName <- outputFileName
	for _, row := range world {
		for _, element := range row {
			ioOutputChannel <- element
		}
	}
	events <- ImageOutputComplete{ // implements Event
		CompletedTurns: turns,
		Filename:       outputFileName,
	}
}

// Distributor divides the work between workers and interacts with other goroutines.
func controller(p Params, c distributorChannels) {
	fileName := strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight)
	sendFileName(fileName, c.ioCommand, c.ioFileName)

	// Dials the engine
	conn, _ := net.Dial("tcp", "127.0.0.1:8030")
	reader := bufio.NewReader(conn)

	// Send the image height and width to the server
	fmt.Fprintf(conn, "%d\n", p.ImageHeight)
	fmt.Fprintf(conn, "%d\n", p.ImageWidth)

	// Send the number of turns to the server
	fmt.Fprintf(conn, "%d\n", p.Turns)

	// Send the world to the server
	sendWorld(p.ImageHeight, p.ImageWidth, c.ioInput, conn)

	// Report the alive cells until the engine is done
	done := make(chan bool)
	go reportAliveCells(c.events, done, reader)
	<-done

	// Receives the world back from the server once all rounds are complete
	world := receiveWorld(p.ImageHeight, p.ImageWidth, reader)

	// Once the final turn is complete
	aliveCells := getAliveCells(world)
	c.events <- FinalTurnComplete{
		CompletedTurns: p.Turns,
		Alive:          aliveCells,
	}
	c.ioCommand <- ioCheckIdle // Make sure that the Io has finished any output before exiting.
	<-c.ioIdle
	c.events <- StateChange{p.Turns, Quitting}
	close(c.events) // Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.

	//world := initialiseWorld(p.ImageHeight, p.ImageWidth, c.ioInput)
	//
	//turnsChan, aliveCellsChan := make(chan int), make(chan int)
	//go reportAliveCells(c.events, turnsChan, aliveCellsChan)
	//completedWorld, completedTurns := engine(world, p.Turns, turnsChan, aliveCellsChan)
	//aliveCells := getAliveCells(completedWorld)
	//c.events <- FinalTurnComplete{
	//	CompletedTurns: completedTurns,
	//	Alive:          aliveCells,
	//}
	////writeFile(completedWorld, fileName, completedTurns, c.ioCommand, c.ioFileName, c.ioOutput, c.events)
	//c.ioCommand <- ioCheckIdle // Make sure that the Io has finished any output before exiting.
	//<-c.ioIdle
	//c.events <- StateChange{completedTurns, Quitting}
	//close(c.events) // Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
}
