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

type rowsFromWorkers struct {
	topRow []byte
	bottomRow []byte
}

// Handles connections by taking in their input and putting it into the messages channel
func handleConnection(conn net.Conn, messages chan<- string) {
	reader := bufio.NewReader(conn)
	for {
		msg, err := reader.ReadString('\n')
		if err != nil { // EOF
			break
		}
		messages <- msg
	}
}

// Handles controllers joining by adding them to the slice of controllers and passing them to the handleConnections function
func handleNewControllers(ln net.Listener, messages chan<- string, mutex *sync.Mutex, connections *[]net.Conn) {
	for {
		connection, _ := ln.Accept()
		fmt.Println("New connection")
		go handleConnection(connection, messages)
		mutex.Lock()
		*connections = append(*connections, connection)
		mutex.Unlock()
	}
}

// Sets up connections and channel for the controller
func setUpControllers(portControllerPtr *string) (chan string, *sync.Mutex, *[]net.Conn) {
	lnController, _ := net.Listen("tcp", *portControllerPtr)
	messagesController := make(chan string)
	mutexControllers := &sync.Mutex{} // Used whenever sending data to client to stop multiple things being sent at once
	controllers := new([]net.Conn)
	go handleNewControllers(lnController, messagesController, mutexControllers, controllers)
	return messagesController, mutexControllers, controllers
}

// Handles workers joining by adding them to the slice of workers and passing them to the handleConnections function
func handleNewWorkers(ln net.Listener, messagesSlice *[]chan string, mutex *sync.Mutex, connections *[]net.Conn) {
	for {
		connection, _ := ln.Accept()
		fmt.Println("New connection")
		messages := make(chan string)
		*messagesSlice = append(*messagesSlice, messages)
		go handleConnection(connection, messages)
		mutex.Lock()
		*connections = append(*connections, connection)
		mutex.Unlock()
	}
}

// Sets up connections and channels for the workers
func setUpWorkers(portWorkerPtr *string) (*[]chan string, *sync.Mutex, *[]net.Conn) {
	lnWorker, _ := net.Listen("tcp", *portWorkerPtr)
	messagesWorker := new([]chan string)
	mutexWorkers := &sync.Mutex{}
	workers := new([]net.Conn)
	go handleNewWorkers(lnWorker, messagesWorker, mutexWorkers, workers)
	return messagesWorker, mutexWorkers, workers
}

// Converts a string receives over tcp to an integer
func netStringToInt(netString string) int {
	integer, _ := strconv.Atoi(netString[:len(netString)-1])
	return integer
}

func getDetails(messagesController <-chan string) (int, int, int, int) {
	heightString := <-messagesController
	widthString := <-messagesController
	turnsString := <-messagesController
	threadsString := <-messagesController
	height := netStringToInt(heightString)
	width := netStringToInt(widthString)
	turns := netStringToInt(turnsString)
	threads := netStringToInt(threadsString)
	return height, width, turns, threads
}

// Initialises the world, getting the values from the server (only used if there are 0 turns)
func initialiseWorld(height int, width int, messages <-chan string) [][]byte {
	world := make([][]byte, height)
	for y := range world {
		world[y] = make([]byte, width) // Create an array of bytes for each row
	}
	for y, row := range world {
		for x := range row {
			msg, _ := <-messages
			cell := netStringToInt(msg)
			world[y][x] = byte(cell) // Add each cell to the row
		}
	}
	return world
}

// Returns part of a world given the number of threads, the part number, the startY, and the endY
func getPart(world [][]byte, threads int, partNum int, startY int, endY int) [][]byte {
	var worldPart [][]byte
	if threads == 1 { // Having 1 thread is a special case as the top and bottom row will come from the same part
		worldPart = append(worldPart, world[len(world) - 1])
		worldPart = append(worldPart, world...)
		worldPart = append(worldPart, world[0])
	} else {
		if partNum == 0 { // If it is the first part add the bottom row of the world as the top row
			worldPart = append(worldPart, world[len(world)-1])
			worldPart = append(worldPart, world[:endY + 1]...)
		} else if partNum == threads - 1 { // If it is the last part add the top row of the world as the bottom row
			worldPart = append(worldPart, world[startY - 1:]...)
			worldPart = append(worldPart, world[0])
		} else {
			worldPart = append(worldPart, world[startY - 1:endY+1]...)
		}
	}
	return worldPart
}

// Sends a given part of the world to a worker
func sendPartToWorker(part [][]byte, worker net.Conn) {
	writer := bufio.NewWriter(worker)
	for _, row := range part {
		for _, cell := range row {
			writer.WriteString(fmt.Sprintf( "%d\n", cell))
		}
	}
	writer.Flush()
}

// Sends a message to every connection in the provided slice of connections
func sendToAll(connections []net.Conn, value string) {
	for _, conn := range connections {
		fmt.Fprintf(conn, value)
	}
}

// Returns a slice containing the height of each section that each worker will process
func calcSectionHeights(height int, threads int) []int {
	heightOfParts := make([]int, threads)
	for i := range heightOfParts{
		heightOfParts[i] = 0
	}
	partAssigning := 0
	for i := 0; i < height; i++ {
		heightOfParts[partAssigning] += 1
		if partAssigning == len(heightOfParts) - 1 {
			partAssigning = 0
		} else {
			partAssigning += 1
		}
	}
	return heightOfParts
}

// Returns a slice containing the initial y-values of the parts of the world that each worker will process
func calcStartYValues(sectionHeights []int) []int {
	startYValues := make([]int, len(sectionHeights))
	totalHeightAssigned := 0
	for i, height := range sectionHeights {
		startYValues[i] = totalHeightAssigned
		totalHeightAssigned += height
	}
	return startYValues
}

// Sends the number of alive cells every 2 seconds
func ticker(mutexDone *sync.Mutex, done *bool, mutexControllers *sync.Mutex, mutexTurnsWorld *sync.Mutex,
	completedTurns *int, controllers *[]net.Conn, numAliveCells *int) {
	ticker := time.NewTicker(2 * time.Second)
	for {
		<-ticker.C
		mutexDone.Lock()
		if !*done {
			mutexControllers.Lock()
			mutexTurnsWorld.Lock()
			fmt.Printf("%d Turns Completed\n", *completedTurns)
			sendToAll(*controllers, fmt.Sprintf("REPORT_ALIVE\n%d\n%d\n", *completedTurns, *numAliveCells))
			mutexTurnsWorld.Unlock()
			mutexControllers.Unlock()
		} else {
			break
		}
		mutexDone.Unlock()
	}
}

// Received key presses from the controller and processes them
func handleKeyPresses(messagesController <-chan string, mutexControllers *sync.Mutex, mutexTurnsWorld *sync.Mutex,
	controllers *[]net.Conn, world *[][]byte, completedTurns *int, pause chan<- bool, send chan bool, shutDown chan bool) {
	paused := false
	ActionsLoop:
		for {
			action := <-messagesController
			if action == "PAUSE\n" {
				pause <- true
				mutexControllers.Lock()
				mutexTurnsWorld.Lock()
				if paused {
					sendToAll(*controllers, "RESUMING\n")
				} else {
					sendToAll(*controllers, "PAUSING\n")
				}
				sendToAll(*controllers, fmt.Sprintf("%d\n", *completedTurns))
				paused = !paused
				mutexTurnsWorld.Unlock()
				mutexControllers.Unlock()
				fmt.Println("Paused/Resumed")
			} else if !paused {
				switch action {
				case "SAVE\n":
					send <- true
					<-send // Once ready to send, a value will be sent back in this channel
					mutexControllers.Lock()
					mutexTurnsWorld.Lock()
					for _, conn := range *controllers {
						fmt.Fprintf(conn, "SENDING_WORLD\n")
						sendWorld(*world, conn, *completedTurns)
					}
					mutexTurnsWorld.Unlock()
					mutexControllers.Unlock()
					fmt.Println("Sent World")
				case "SHUT_DOWN\n":
					shutDown <- true
				case "DONE\n":
					break ActionsLoop
				}
			}
		}
}

func getNumAliveCells(messagesWorker *[]chan string, threads int) int {
	numAliveCells := 0
	for _, channel := range (*messagesWorker)[:threads] {
		numAliveCellsPartString := <-channel
		numAliveCellsPart := netStringToInt(numAliveCellsPartString)
		numAliveCells += numAliveCellsPart
	}
	return numAliveCells
}

func sendRowToWorker(row []byte, worker net.Conn) {
	writer := bufio.NewWriter(worker)
	for _, element := range row {
		writer.WriteString(fmt.Sprintf("%d\n", int(element)))
	}
	writer.Flush()
}

func receiveRowFromWorker(width int, messages <-chan string) []byte {
	var row []byte
	for i := 0; i < width; i++ {
		row = append(row, byte(netStringToInt(<-messages)))
	}
	return row
}

func getRowsFromWorkers(threads int, messagesWorker *[]chan string, width int) []rowsFromWorkers {
	var rowsFromWorkersSlice []rowsFromWorkers
	for _, workerChan := range (*messagesWorker)[:threads] {
		topRow := receiveRowFromWorker(width, workerChan)
		bottomRow := receiveRowFromWorker(width, workerChan)
		rowsFromWorkersSlice = append(rowsFromWorkersSlice, rowsFromWorkers {
			topRow: topRow,
			bottomRow: bottomRow,
		})
	}
	return rowsFromWorkersSlice
}

func receiveWorldFromWorkers(height int, sectionHeights []int, width int, messagesChannels []chan string) [][]byte {
	world := make([][]byte, height)
	for i, channel := range messagesChannels {
		var part [][]byte
		part = make([][]byte, sectionHeights[i])
		for y := range part {
			part[y] = make([]byte, width)
		}
		for y, row := range part {
			for x := range row {
				msg, _ := <-channel
				cell := netStringToInt(msg)
				part[y][x] = byte(cell)
			}
		}
		world = append(world, part...)
	}
	return world
}

// Returns the world with its final values filled
func sendWorld(world [][]byte, conn net.Conn, completedTurns int) {
	fmt.Fprintf(conn, "%d\n", completedTurns)
	writer := bufio.NewWriter(conn)
	for _, row := range world {
		for _, element := range row {
			writer.WriteString(fmt.Sprintf( "%d\n", element))
		}
	}
	writer.Flush()
}

// Divides the work between workers, interacts with them and interacts with other subroutines
func main() {
	portControllerPtr := flag.String("port_controller", ":8030", "port to listen on for controllers")
	portWorkerPtr := flag.String("port_worker", ":8040", "port to listen on")
	flag.Parse()
	messagesController, mutexControllers, controllers := setUpControllers(portControllerPtr)
	messagesWorker, mutexWorkers, workers := setUpWorkers(portWorkerPtr)
	shutDown := false
	for !shutDown {
		if <-messagesController == "INITIALISE\n" { // This stops a new connection attempting to rejoin once all turns are complete breaking the engine
			height, width, turns, threads := getDetails(messagesController)
			mutexWorkers.Lock()
			workersUsed := (*workers)[:threads]
			mutexWorkers.Unlock()
			done := false
			completedTurns := 0
			mutexDone := &sync.Mutex{}
			mutexTurnsWorld := &sync.Mutex{}
			world := initialiseWorld(height, width, messagesController)
			if turns > 0 {     // If there are more than 0 turns, process them
				fmt.Println("Received details")
				mutexWorkers.Lock()
				sectionHeights := calcSectionHeights(height, threads)
				for i, worker := range workersUsed {
					fmt.Fprintf(worker, "%d\n", sectionHeights[i])
				}
				sendToAll(workersUsed, fmt.Sprintf("%d\n", width))
				startYValues := calcStartYValues(sectionHeights)
				for i := 0; i < threads; i++ {
					startY := startYValues[i]
					endY := startY + sectionHeights[i]
					part := getPart(world, threads, i, startY, endY)
					sendPartToWorker(part, (*workers)[i])
				}
				mutexWorkers.Unlock()
				numAliveCells := 0
				go ticker(mutexDone, &done, mutexControllers, mutexTurnsWorld, &completedTurns, controllers, &numAliveCells)
				pause := make(chan bool)
				send := make(chan bool)
				shutDownChan := make(chan bool)
				go handleKeyPresses(messagesController, mutexControllers, mutexTurnsWorld, controllers, &world,
					&completedTurns, pause, send, shutDownChan)
				for turn := 0; turn < turns; turn++ {
					select {
					case <-pause:
						<-pause
					default: // If the controller has not requested a pause just move onto performing the next turn of the world
					}
					mutexTurnsWorld.Lock()
					numAliveCells = getNumAliveCells(messagesWorker, threads)
					completedTurns = turn + 1

					// Receive the top and bottom rows from each worker
					rowsFromWorkersSlice := getRowsFromWorkers(threads, messagesWorker, width)

					// Send the top and bottom rows to each worker
					for i := range rowsFromWorkersSlice {
						workerAbove, workerBelow := i - 1, i + 1
						if i == 0 {
							workerAbove = len(rowsFromWorkersSlice) - 1
						}
						if i == len(rowsFromWorkersSlice) - 1 {
							workerBelow = 0
						}
						sendRowToWorker(rowsFromWorkersSlice[workerAbove].bottomRow, (*workers)[i])
						sendRowToWorker(rowsFromWorkersSlice[workerBelow].topRow, (*workers)[i])
					}
					mutexWorkers.Lock()
					select {
					case <-send:
						sendToAll(workersUsed, "SEND_WORLD\n")
						world = receiveWorldFromWorkers(height, sectionHeights, width, (*messagesWorker)[:threads])
						send <- true
					case <-shutDownChan:
						turn = turns
						sendToAll(*workers, "SHUT_DOWN\n")
						shutDown = true
					default:
						if completedTurns != turns {
							sendToAll(workersUsed, "CONTINUE\n")
						} else {
							sendToAll(workersUsed, "DONE\n")
						}
					}
					mutexWorkers.Unlock()
					mutexTurnsWorld.Unlock()
				}
				mutexTurnsWorld.Lock()
				world = receiveWorldFromWorkers(height, sectionHeights, width, (*messagesWorker)[:threads])
				mutexTurnsWorld.Unlock()
			}
			// Once it has done all the iterations, send a message to the controller to let it know it is done
			mutexDone.Lock()
			mutexControllers.Lock()
			done = true
			sendToAll(*controllers, "DONE\n")
			mutexDone.Unlock()
			for _, conn := range *controllers { // Send the world back to all of the controllers
				sendWorld(world, conn, completedTurns)
			}
			*controllers = (*controllers)[:0] // Clear the controllers once processing the current board is finished
			mutexControllers.Unlock()
		}
	}
	sendToAll(*controllers, "SHUTTING_DOWN\n")
}
