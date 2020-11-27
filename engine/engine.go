package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"
)

// Converts a string receives over tcp to an integer
func netStringToInt(netString string) int {
	integer, _ := strconv.Atoi(netString[:len(netString) - 1])
	return integer
}

// Initialises the world, getting the values from the server
func initialiseWorld(height int, width int, reader *bufio.Reader) [][]byte {
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

// Returns the world with its final values filled
func sendWorld(world [][]byte, conn net.Conn, completedTurns int) {
	fmt.Fprintf(conn, "%d\n", completedTurns)
	for _, row := range world {
		for _, element := range row {
			fmt.Fprintf(conn, "%d\n", element)
		}
	}
}

func main() {
	portPtr := flag.String("port", ":8030", "port to listen on")
	ln, _ := net.Listen("tcp", *portPtr)
	for {
		conn, _ := ln.Accept()
		reader := bufio.NewReader(conn)

		heightString, _ := reader.ReadString('\n')
		widthString, _ := reader.ReadString('\n')
		turnsString, _ := reader.ReadString('\n')
		height := netStringToInt(heightString)
		width := netStringToInt(widthString)
		turns := netStringToInt(turnsString)
		world := initialiseWorld(height, width, reader)
		fmt.Printf("Received %dx%d\n", height, width)

		var turn int
		var completedTurns int
		done := false
		mutexDone := &sync.Mutex{}
		mutexTurnsWorld := &sync.Mutex{}
		mutexSending := &sync.Mutex{} // Used whenever sending data to client to stop multiple things being sent at once
		ticker := time.NewTicker(2 * time.Second)
		go func() {
			for {
				<-ticker.C
				mutexDone.Lock()
				if !done {
					mutexSending.Lock()
					mutexTurnsWorld.Lock()
					fmt.Printf("%d Turns Completed\n", completedTurns)
					fmt.Fprintf(conn, "REPORT_ALIVE\n")
					fmt.Fprintf(conn, "%d\n", completedTurns)
					fmt.Fprintf(conn, "%d\n", calcNumAliveCells(world))
					mutexTurnsWorld.Unlock()
					mutexSending.Unlock()
				}
				mutexDone.Unlock()
			}
		}()
		pause := make(chan bool)
		go func() {
			paused := false
			for {
				action, _ := reader.ReadString('\n')
				if action == "SAVE\n" {
					mutexSending.Lock()
					mutexTurnsWorld.Lock()
					fmt.Fprintf(conn, "SENDING_WORLD\n")
					sendWorld(world, conn, completedTurns)
					mutexTurnsWorld.Unlock()
					mutexSending.Unlock()
					fmt.Println("Sent World")
				} else if action == "STOP\n" {
					fmt.Println("STOP")
				} else if action == "PAUSE\n"  {
					pause <- true
					mutexSending.Lock()
					mutexTurnsWorld.Lock()
					if paused {
						fmt.Fprintf(conn, "RESUMING\n")
						paused = false
					} else {
						fmt.Fprintf(conn, "PAUSING\n")
						paused = true
					}
					fmt.Fprintf(conn, "%d\n", completedTurns)
					mutexTurnsWorld.Unlock()
					mutexSending.Unlock()
					fmt.Println("Paused/Resumed")
				} else if action == "RESUME\n" {
					fmt.Println("RESUME")
				}
			}
		}()
		for turn = 0; turn < turns; turn++ {
			select {
			case <-pause:
				<-pause
			default:
			}
			mutexTurnsWorld.Lock()
			world = calcNextState(world)
			completedTurns = turn + 1
			mutexTurnsWorld.Unlock()
		}

		// Once it has done all the iterations, send a message to the controller to let it know it is done
		mutexDone.Lock()
		done = true
		fmt.Fprintf(conn, "DONE\n")
		mutexDone.Unlock()

		// Send the world back to the controller
		mutexSending.Lock()
		sendWorld(world, conn, completedTurns)
		mutexSending.Unlock()
		fmt.Printf("Computed %d turns of %dx%d\n", completedTurns, height, width)
	}
}
