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

const (
	dead = 0
	alive = 255
)

// Handles the engine by taking in its input and putting it into the messages channel
func handleEngine(conn net.Conn, messages chan<- string) {
	reader := bufio.NewReader(conn)
	for {
		msg, err := reader.ReadString('\n')
		if err != nil { // EOF
			break
		}
		messages <- msg
	}
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
		writer.WriteByte(<-ioInput)
	}
	writer.WriteString("\n")
	writer.Flush()
}

// Manages key presses
func manageKeyPresses(keyPresses <-chan rune, quit chan<- bool, conn net.Conn) {
	for {
		key := <-keyPresses
		switch key {
		case 115: // Save (S)
			fmt.Fprintf(conn, "SAVE\n")
		case 113: // Quit (Q)
			quit <- true
		case 112: // Pause/Resume (P)
			fmt.Fprintf(conn, "PAUSE\n")
		case 107: // Shut Down (K)
			fmt.Fprintf(conn, "SHUT_DOWN\n")
		}
	}
}

// Receives commands from the engine and responds appropriately
func handleEngineCommands(events chan<- Event, messages chan string, done chan<- bool, imageHeight int, imageWidth int,
	fileName string, ioCommand chan<- ioCommand, ioFileName chan<- string, ioOutput chan<- uint8, quit chan<- bool) {
	CommandLoop:
		for {
			command := <-messages
			switch command {
			case "REPORT_ALIVE\n":
				reportAliveCells(events, messages)
			case "DONE\n":
				done <- true
				break CommandLoop // Stops loop trying to receive inputs as would otherwise receive the cell values as inputs
			case "SENDING_WORLD\n", "SHUTTING_DOWN\n":
				world, completedTurns := receiveWorld(imageHeight, imageWidth, messages)
				writeFile(world, fileName, completedTurns, ioCommand, ioFileName, ioOutput, events)
				if command == "SHUTTING_DOWN\n" {
					quit <- true // Causes the controller to quit
				}
			case "PAUSING\n", "RESUMING\n":
				completedTurnsString := <-messages
				completedTurns := netStringToInt(completedTurnsString)
				var newState State
				if command == "PAUSING\n" {
					newState = Paused
				} else {
					newState = Continuing
				}
				events <- StateChange{completedTurns, newState}
			}
	}
}

// Reports the number of alive cells
func reportAliveCells(events chan<- Event, messages <-chan string) {
	turnsString := <-messages
	aliveCellsString := <-messages
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

// Receives the world and the number of completed turns from the engine and returns them
func receiveWorld(height int, width int, messages <-chan string) ([][]byte, int) {
	completedTurnsString := <-messages
	completedTurns := netStringToInt(completedTurnsString)
	data := <-messages
	i := 0
	world := make([][]byte, height)
	for y := range world {
		world[y] = make([]byte, width)
	}
	for y, row := range world {
		for x := range row {
			switch data[i] {
			case dead:
				world[y][x] = byte(0)
			case alive:
				world[y][x] = byte(255)
			}
			i += 1
		}
	}
	return world, completedTurns
}

// Given a world, returns a slice of the alive cells
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
	events <- ImageOutputComplete{
		CompletedTurns: turns,
		Filename:       outputFileName,
	}
}

// Controller sends the world to the engine to be processed, receives it back and interacts with other subroutines
func controller(p Params, c distributorChannels) {
	if p.Engine == "" {
		p.Engine = "127.0.0.1:8030"
	}
	conn, err := net.Dial("tcp", p.Engine) // Dials the engine and establishes reader
	switch {
	case p.Threads > p.ImageHeight:
		fmt.Println("Error: You cannot have an image height greater than the number of workers.")
	case err == nil: // If there's no error it means the engine is active and can be connected to
		messages := make(chan string)
		go handleEngine(conn, messages)
		fileName := strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight)
		if p.Rejoin == false {
			fmt.Fprintf(conn, "INITIALISE\n")
			sendFileName(fileName, c.ioCommand, c.ioFileName)
			// Send height, width, turns and threads to the server
			fmt.Fprintf(conn, "%d\n%d\n%d\n%d\n", p.ImageHeight, p.ImageWidth, p.Turns, p.Threads)
			sendWorld(p.ImageHeight, p.ImageWidth, c.ioInput, conn) // Send the world to the server
		}
		quit := make(chan bool)
		done := make(chan bool) // Used to stop execution until the turns are done executing otherwise receiveWorld will start trying to receive
		go manageKeyPresses(c.keyPresses, quit, conn)
		go handleEngineCommands(c.events, messages, done, p.ImageHeight, p.ImageWidth, fileName, c.ioCommand, c.ioFileName, c.ioOutput, quit)
		select {
		case <-done: // Once all rounds are complete the code block below is executed
			world, completedTurns := receiveWorld(p.ImageHeight, p.ImageWidth, messages)
			fmt.Fprintf(conn, "DONE\n") // Sends this message back to the controller to let it know it has receives the message
			aliveCells := getAliveCells(world)
			c.events <- FinalTurnComplete{
				CompletedTurns: completedTurns,
				Alive:          aliveCells,
			}
			writeFile(world, fileName, completedTurns, c.ioCommand, c.ioFileName, c.ioOutput, c.events)
			c.ioCommand <- ioCheckIdle // Make sure that the Io has finished any output before exiting.
			<-c.ioIdle
			c.events <- StateChange{completedTurns, Quitting}
		case <-quit: // If the controller quits, this stops the code block above executing
		}
	case err != nil:
		fmt.Printf("Error: no engine at address %s\n", p.Engine)
	}
	close(c.events) // Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
}
