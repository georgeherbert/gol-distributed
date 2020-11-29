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
			msg, _ := <-messages
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
	//fmt.Println(row, column)
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
	for _, row := range world[1:len(world) - 1] {
		for _, cell := range row {
			cellAsString := strconv.Itoa(int(cell)) + "\n"
			fmt.Fprintf(engine, cellAsString)
		}
	}
}

func main() {
	engine, _ := net.Dial("tcp", "127.0.0.1:8040")
	messages := make(chan string)
	go handleEngine(engine, messages)
	for {
		heightString, widthString, threadsString := <-messages, <-messages, <-messages
		height, width, threads := netStringToInt(heightString), netStringToInt(widthString), netStringToInt(threadsString)
		heightToReceive := height + 2
		fmt.Sprintf("", threads)
		world := initialiseWorld(heightToReceive, width, messages)
		fmt.Println("Received world")
		for {
			nextWorld := calcNextState(world)
			world = [][]byte{nextWorld[len(nextWorld) - 1]}
			world = append(world, nextWorld...)
			world = append(world, nextWorld[0])
			aliveCells := calcNumAliveCells(world[1:len(world) - 1])
			aliveCellsString := strconv.Itoa(aliveCells) + "\n"
			fmt.Fprintf(engine, aliveCellsString)
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
