package main

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
)

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
	world := make([][]byte, height)
	for y := range world {
		world[y] = make([]byte, width)
	}
	for y, row := range world {
		for x := range row {
			msg := <-messages
			cell := netStringToInt(msg)
			world[y][x] = byte(cell)
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

func sendRowToEngine(row []byte, engine net.Conn) {
	writer := bufio.NewWriter(engine)
	for _, element := range row {
		writer.WriteString(fmt.Sprintf("%d\n", int(element)))
	}
	writer.Flush()
}

func receiveRowFromEngine(width int, messages <-chan string) []byte {
	var row []byte
	for i := 0; i < width; i++ {
		row = append(row, byte(netStringToInt(<-messages)))
	}
	return row
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

func sendWorldToEngine(engine net.Conn, world [][]byte) {
	fmt.Println("Sending world to engine")
	writer := bufio.NewWriter(engine)
	for _, row := range world[1:len(world) - 1] {
		for _, cell := range row {
			writer.WriteString(fmt.Sprintf("%d\n", int(cell)))
		}
	}
	writer.Flush()
}

func main() {
	engine, _ := net.Dial("tcp", "127.0.0.1:8040")
	messages := make(chan string)
	go handleEngine(engine, messages)
	for {
		heightString, widthString := <-messages, <-messages
		height, width := netStringToInt(heightString), netStringToInt(widthString)
		heightToReceive := height + 2
		world := initialiseWorld(heightToReceive, width, messages)
		fmt.Println("Received world")
		for {
			nextWorld := calcNextState(world)

			// Report the number of alive cells to the engine
			aliveCells := calcNumAliveCells(nextWorld)
			aliveCellsString := strconv.Itoa(aliveCells) + "\n"
			fmt.Fprintf(engine, aliveCellsString)

			// Send the top row and bottom row to the controller
			sendRowToEngine(nextWorld[0], engine)
			sendRowToEngine(nextWorld[len(nextWorld) - 1], engine)

			// Receive a new top row and bottom row from the controller
			//world[0] = receiveRowFromEngine(width, messages)
			//world[len(world) - 1] = receiveRowFromEngine(width, messages)
			topRow := receiveRowFromEngine(width, messages)
			bottomRow := receiveRowFromEngine(width, messages)

			// Add the new top and bottom rows to the next world
			world = [][]byte{bottomRow}
			world = append(world, nextWorld...)
			world = append(world, topRow)

			status := <-messages
			if status == "DONE\n" {
				break
			} else if status == "SEND_WORLD\n" {
				sendWorldToEngine(engine, world)
			}
		}
		sendWorldToEngine(engine, world)
	}
}
