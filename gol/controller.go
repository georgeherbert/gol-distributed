package gol

import (
	"bufio"
	"fmt"
	"net"
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
	keyPresses <-chan rune
}

// Sends the file name to io.go so the world can be initialised
func sendFileName(fileName string, ioCommand chan<- ioCommand, ioFileName chan<- string) {
	ioCommand <- ioInput
	ioFileName <- fileName
}

// Sends the world with its initial values filled
func sendWorld(height int, width int, ioInput <-chan uint8, conn net.Conn) {
	writer := bufio.NewWriter(conn)
	for i := 0; i < height * width; i++ {
		writer.WriteString(fmt.Sprintf("%d\n", <-ioInput))
		//fmt.Fprintf(conn, "%d\n", <-ioInput)
	}
	writer.Flush()
}

// Manages key presses
func manageKeyPresses(keyPresses <-chan rune, quit chan<- bool, conn net.Conn) {
	for {
		key := <-keyPresses
		if key == 115 { // save
			fmt.Fprintf(conn, "SAVE\n")
		} else if key == 113 { // stop
			fmt.Fprintf(conn, "QUIT\n")
			quit <- true
		} else if key == 112 { // pause/resume
			fmt.Fprintf(conn, "PAUSE\n")
		}
	}
}

func handleEngine(events chan<- Event, reader *bufio.Reader, done chan<- bool, imageHeight int, imageWidth int,
	fileName string, ioCommand chan<- ioCommand, ioFileName chan<- string, ioOutput chan<- uint8) {
	for {
		operation, _ := reader.ReadString('\n')
		if operation == "REPORT_ALIVE\n" {
			reportAliveCells(events, reader)
		} else if operation == "DONE\n" {
			done <- true
			break // Stops loop trying to receive inputs as would otherwise receive the cell values as inputs
		} else if operation == "SENDING_WORLD\n" {
			world, completedTurns := receiveWorld(imageHeight, imageWidth, reader)
			writeFile(world, fileName, completedTurns, ioCommand, ioFileName, ioOutput, events)
		} else if operation == "PAUSING\n" || operation == "RESUMING\n" {
			completedTurnsString, _ := reader.ReadString('\n')
			completedTurns := netStringToInt(completedTurnsString)
			var newState State
			if operation == "PAUSING\n" {
				newState = Paused
			} else {
				newState = Continuing
			}
			events <- StateChange{completedTurns, newState}
		}
	}
}

func reportAliveCells(events chan<- Event, reader *bufio.Reader) {
	turnsString, _ := reader.ReadString('\n')
	aliveCellsString, _ := reader.ReadString('\n')
	turns := netStringToInt(turnsString)
	aliveCells := netStringToInt(aliveCellsString)
	events <- AliveCellsCount{
		CompletedTurns: turns,
		CellsCount:     aliveCells,
	}
}

// Converts a string receives over tcp to an integer
func netStringToInt(netString string) int {
	integer, _ := strconv.Atoi(netString[:len(netString) - 1])
	return integer
}

func receiveWorld(height int, width int, reader *bufio.Reader) ([][]byte, int) {
	completedTurnsString, _ := reader.ReadString('\n')
	completedTurns := netStringToInt(completedTurnsString)
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
	return world, completedTurns
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

//TODO: Look into bufio Writer https://medium.com/golangspec/introduction-to-bufio-package-in-golang-ad7d1877f762
// Distributor divides the work between workers and interacts with other goroutines.
func controller(p Params, c distributorChannels) {
	// Dials the engine and establishes reader
	conn, _ := net.Dial("tcp", "127.0.0.1:8030")
	reader := bufio.NewReader(conn)

	//TODO: Make it so this doesn't have to be specified when reconnecting
	fileName := strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight)

	if p.Rejoin == false {
		fmt.Fprintf(conn, "INITIALISE\n")

		sendFileName(fileName, c.ioCommand, c.ioFileName)

		fmt.Fprintf(conn, "%d\n", p.ImageHeight) // Send image height to server
		fmt.Fprintf(conn, "%d\n", p.ImageWidth)  // Send image width to server
		fmt.Fprintf(conn, "%d\n", p.Turns)       // Send number of turns to server
		fmt.Fprintf(conn, "%d\n", p.Threads)     // Send number of threads to server

		sendWorld(p.ImageHeight, p.ImageWidth, c.ioInput, conn) // Send the world to the server
	}

	quit := make(chan bool)
	go manageKeyPresses(c.keyPresses, quit, conn)
	done := make(chan bool) // Used to stop execution until the turns are done executing otherwise receiveWorld will start trying to receive
	go handleEngine(c.events, reader, done, p.ImageHeight, p.ImageWidth, fileName, c.ioCommand, c.ioFileName, c.ioOutput) // Report the alive cells until the engine is done
	select {
	case <-done:
		// Receives the world back from the server once all rounds are complete
		world, completedTurns := receiveWorld(p.ImageHeight, p.ImageWidth, reader)
		fmt.Fprintf(conn, "DONE\n") // Sends this message back to the controller to let it know it has receives the message
		// Once the final turn is complete
		aliveCells := getAliveCells(world)
		c.events <- FinalTurnComplete{
			CompletedTurns: completedTurns,
			Alive:          aliveCells,
		}
		writeFile(world, fileName, completedTurns, c.ioCommand, c.ioFileName, c.ioOutput, c.events)
		c.ioCommand <- ioCheckIdle // Make sure that the Io has finished any output before exiting.
		<-c.ioIdle
		c.events <- StateChange{completedTurns, Quitting}
	case <-quit:
	}
	close(c.events) // Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
}
