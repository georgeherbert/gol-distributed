package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"strconv"
)

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

// Converts a string receives over tcp to an integer
func netStringToInt(netString string) int {
	integer, _ := strconv.Atoi(netString[:len(netString)-1])
	return integer
}

// Initialises the world, getting the values from the server
func initialiseWorld(height int, width int, messages <-chan string) [][]byte {
	data := <-messages
	world := make([][]byte, height)
	for y := range world {
		world[y] = make([]byte, width)
	}
	i := 0
	for y, row := range world {
		for x := range row {
			world[y][x] = data[i]
			i += 1
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
func calcLiveNeighbours(neighbours []byte) int {
	liveNeighbours := 0
	for _, neighbour := range neighbours {
		if neighbour == alive {
			liveNeighbours += 1
		}
	}
	return liveNeighbours
}

// Returns the new value of a cell given its current value and number of live neighbours
func calcValue(item byte, liveNeighbours int) byte {
	calculatedValue := byte(dead)
	if item == alive {
		if liveNeighbours == 2 || liveNeighbours == 3 {
			calculatedValue = byte(alive)
		}
	} else {
		if liveNeighbours == 3 {
			calculatedValue = byte(alive)
		}
	}
	return calculatedValue
}

// Returns the next state of a world given the current state
func calcNextState(world [][]byte) [][]byte {
	var nextWorld [][]byte
	for y, row := range world[1:len(world) - 1] {
		nextWorld = append(nextWorld, []byte{})
		for x, element := range row {
			neighbours := getNeighbours(world, y + 1, x)
			liveNeighbours := calcLiveNeighbours(neighbours)
			value := calcValue(element, liveNeighbours)
			nextWorld[y] = append(nextWorld[y], value)
		}
	}
	return nextWorld
}

// Sends a row in the world to the engine
func sendRowToEngine(row []byte, engine net.Conn) {
	writer := bufio.NewWriter(engine)
	for _, element := range row {
		writer.WriteByte(element)
	}
	writer.WriteString("\n")
	writer.Flush()
}

// Receives a row from the engine to be placed in the world
func receiveRowFromEngine(width int, messages <-chan string) []byte {
	data := <-messages
	var row []byte
	for i := 0; i < width; i++ {
		row = append(row, data[i])
	}
	return row
}

// Returns the number of alive cells in part of a world
func calcNumAliveCells(world [][]byte) int {
	total := 0
	for _, row := range world {
		for _, element := range row {
			if element == alive {
				total += 1
			}
		}
	}
	return total
}

// Sends part of the world to the engine
func sendPartToEngine(engine net.Conn, world [][]byte) {
	fmt.Println("Sending world to engine")
	writer := bufio.NewWriter(engine)
	for _, row := range world[1:len(world) - 1] {
		for _, cell := range row {
			writer.WriteByte(cell)
		}
	}
	writer.WriteString("\n")
	writer.Flush()
}

// Receives part of a world to process and returns it to the engine once processed
func main() {
	addressPtr := flag.String("address_engine", "127.0.0.1:8040", "Specify the address of the GoL engine. Defaults to 127.0.0.1:8040.")
	flag.Parse()
	shutDown := false
	engine, err := net.Dial("tcp", *addressPtr)
	messages := make(chan string)
	if err == nil {
		go handleEngine(engine, messages)
	} else {
		fmt.Printf("Error: no engine at address %s\n", *addressPtr)
		shutDown = true // Means it will never go into the loop
	}
	for !shutDown { // This loops until a shutdown message is sent from the engine
		firstMessage := <-messages
		var heightString string
		if firstMessage == "SHUT_DOWN\n" { // Worker may not be used, so the first thing it could receive is a shut down message
			break
		} else {
			heightString = firstMessage
		}
		widthString := <-messages
		height, width := netStringToInt(heightString), netStringToInt(widthString)
		heightToReceive := height + 2
		world := initialiseWorld(heightToReceive, width, messages)
		fmt.Println("Received world")
		for {
			nextWorld := calcNextState(world)
			aliveCells := calcNumAliveCells(nextWorld)
			fmt.Fprintf(engine, "%d\n", aliveCells) // Report the number of alive cells to the engine
			// Send the top row and bottom row to the controller
			sendRowToEngine(nextWorld[0], engine)
			sendRowToEngine(nextWorld[len(nextWorld) - 1], engine)
			// Receive a new top row and bottom row from the controller
			topRow := receiveRowFromEngine(width, messages)
			bottomRow := receiveRowFromEngine(width, messages)
			// Add the new top and bottom rows to the next world
			world = [][]byte{topRow}
			world = append(world, nextWorld...)
			world = append(world, bottomRow)
			status := <-messages
			if status == "DONE\n" {
				break
			} else if status == "SHUT_DOWN\n" {
				shutDown = true
				break
			} else if status == "SEND_WORLD\n" {
				sendPartToEngine(engine, world)
			}
		}
		sendPartToEngine(engine, world)
	}
}
