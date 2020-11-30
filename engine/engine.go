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

// Converts a string receives over tcp to an integer
func netStringToInt(netString string) int {
	integer, _ := strconv.Atoi(netString[:len(netString)-1])
	return integer
}

// Initialises the world, getting the values from the server (only used if there are 0 turns)
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

// Returns part of a world given the number of threads, the part number, the startY, and the endY
func getPart(world [][]byte, threads int, partNum int, startY int, endY int) [][]byte {
	var worldPart [][]byte
	if threads == 1 {
		worldPart = append(worldPart, world[len(world) - 1])
		worldPart = append(worldPart, world...)
		worldPart = append(worldPart, world[0])
	} else {
		if partNum == 0 {
			worldPart = append(worldPart, world[len(world)-1])
			worldPart = append(worldPart, world[:endY + 1]...)
		} else if partNum == threads - 1 {
			worldPart = append(worldPart, world[startY - 1:]...)
			worldPart = append(worldPart, world[0])
		} else {
			worldPart = append(worldPart, world[startY - 1:endY+1]...)
		}
	}
	return worldPart
}

func sendPartToWorker(part [][]byte, worker net.Conn) {
	writer := bufio.NewWriter(worker)
	for _, row := range part {
		for _, cell := range row {
			writer.WriteString(fmt.Sprintf( "%d\n", cell))
		}
	}
	writer.Flush()
}

func sendToAll(connections *[]net.Conn, value string) {
	for _, conn := range *connections {
		fmt.Fprintf(conn, value)
	}
}

// Sends the number of alive cells every 2 seconds
func ticker(mutexDone *sync.Mutex, done *bool, mutexControllers *sync.Mutex, mutexTurnsWorld *sync.Mutex,
	completedTurns *int, controllers *[]net.Conn, numAliveCells *int) {
	ticker := time.NewTicker(2 * time.Second)
	go func() {
		for {
			<-ticker.C
			mutexDone.Lock()
			if !*done {
				mutexControllers.Lock()
				mutexTurnsWorld.Lock()
				fmt.Printf("%d Turns Completed\n", *completedTurns)
				sendToAll(controllers, "REPORT_ALIVE\n")
				sendToAll(controllers, fmt.Sprintf("%d\n", *completedTurns))
				sendToAll(controllers, fmt.Sprintf("%d\n", *numAliveCells))
				mutexTurnsWorld.Unlock()
				mutexControllers.Unlock()
			} else {
				break
			}
			mutexDone.Unlock()
		}
	}()
}

// Received key presses from the controller and processes them
func handleKeyPresses(messagesController <-chan string, mutexControllers *sync.Mutex, mutexTurnsWorld *sync.Mutex,
	controllers *[]net.Conn, world *[][]byte, completedTurns *int, pause chan<- bool, send chan bool) {
	paused := false
	for {
		action := <-messagesController
		if action == "PAUSE\n" {
			pause <- true
			mutexControllers.Lock()
			mutexTurnsWorld.Lock()
			if paused {
				sendToAll(controllers, "RESUMING\n")
			} else {
				sendToAll(controllers, "PAUSING\n")
			}
			paused = !paused
			sendToAll(controllers, fmt.Sprintf("%d\n", *completedTurns))
			mutexTurnsWorld.Unlock()
			mutexControllers.Unlock()
			fmt.Println("Paused/Resumed")
		}
		if !paused {
			if action == "SAVE\n" {
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
			} else if action == "QUIT\n" {
				fmt.Println("A controller has quit")
			}  else if action == "DONE\n" {
				break
			}
		}
	}
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

func receiveWorldFromWorkers(height int, sectionHeight int, width int, messagesChannels []chan string) [][]byte {
	world := make([][]byte, height)
	for _, channel := range messagesChannels {
		part := make([][]byte, sectionHeight)
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

func main() {
	portControllerPtr := flag.String("port_controller", ":8030", "port to listen on for controllers")
	portWorkerPtr := flag.String("port_worker", ":8040", "port to listen on")
	flag.Parse()

	//TODO: Maybe put this stuff in a function seeing as the two blocks are identical

	// Controllers stuff
	lnController, _ := net.Listen("tcp", *portControllerPtr)
	messagesController := make(chan string)
	mutexControllers := &sync.Mutex{} // Used whenever sending data to client to stop multiple things being sent at once
	controllers := new([]net.Conn)
	go handleNewControllers(lnController, messagesController, mutexControllers, controllers)

	// Workers stuff
	lnWorker, _ := net.Listen("tcp", *portWorkerPtr)
	messagesWorker := new([]chan string)
	mutexWorkers := &sync.Mutex{}
	workers := new([]net.Conn)
	go handleNewWorkers(lnWorker, messagesWorker, mutexWorkers, workers)

	for {
		if <-messagesController == "INITIALISE\n" { // This stops a new connection attempting to rejoin once all turns are complete breaking the engine
			heightString, widthString, turnsString, threadsString := <-messagesController, <-messagesController, <-messagesController, <-messagesController
			height, width, turns, threads := netStringToInt(heightString), netStringToInt(widthString), netStringToInt(turnsString), netStringToInt(threadsString)

			workersUsed := (*workers)[:threads]

			done := false
			completedTurns := 0
			mutexDone := &sync.Mutex{}
			mutexTurnsWorld := &sync.Mutex{}

			world := initialiseWorld(height, width, messagesController)

			if turns > 0 {     // If there are more than 0 turns, process them
				fmt.Println("Received details")

				mutexWorkers.Lock()
				sectionHeight := height / threads
				sendToAll(&workersUsed, fmt.Sprintf("%d\n", sectionHeight))
				sendToAll(&workersUsed, widthString)
				for i := 0; i < threads; i++ {
					startY := i * sectionHeight
					endY := startY + sectionHeight
					part := getPart(world, threads, i, startY, endY)
					sendPartToWorker(part, (*workers)[i])
				}
				mutexWorkers.Unlock()

				numAliveCells := 0
				go ticker(mutexDone, &done, mutexControllers, mutexTurnsWorld, &completedTurns, controllers, &numAliveCells)

				pause := make(chan bool)
				send := make(chan bool)
				go handleKeyPresses(messagesController, mutexControllers, mutexTurnsWorld, controllers, &world, &completedTurns, pause, send)

				for turn := 0; turn < turns; turn++ {
					select {
					case <-pause:
						<-pause
					default:
					}
					mutexTurnsWorld.Lock()
					numAliveCells := 0
					for _, channel := range (*messagesWorker)[:threads] {
						numAliveCellsPartString := <-channel // TODO: Number of alive cells should be received from all workers
						numAliveCellsPart := netStringToInt(numAliveCellsPartString)
						numAliveCells += numAliveCellsPart
					}
					completedTurns = turn + 1

					// Receive the top and bottom rows from each worker
					rowsFromWorkers := make([][][]byte, threads)
					for i, workerChan := range (*messagesWorker)[:threads] {
						topRow := receiveRowFromWorker(width, workerChan)
						rowsFromWorkers[i] = append(rowsFromWorkers[i], topRow)
						bottomRow := receiveRowFromWorker(width, workerChan)
						rowsFromWorkers[i] = append(rowsFromWorkers[i], bottomRow)
					}

					// Send the top and bottom rows to each worker
					for i, _ := range rowsFromWorkers {
						topRowPos, bottomRowPos := i - 1, i + 1
						if i == 0 {
							topRowPos = len(rowsFromWorkers) - 1
						}
						if i == len(rowsFromWorkers) - 1 {
							bottomRowPos = 0
						}
						sendRowToWorker(rowsFromWorkers[topRowPos][0], (*workers)[i])
						sendRowToWorker(rowsFromWorkers[bottomRowPos][1], (*workers)[i])
					}

					mutexWorkers.Lock()
					select {
					case <-send:
						sendToAll(&workersUsed, "SEND_WORLD\n")
						world = receiveWorldFromWorkers(height, sectionHeight, width, (*messagesWorker)[:threads])
						send <- true
					default:
						if completedTurns != turns {
							sendToAll(&workersUsed, "CONTINUE\n")
						} else {
							sendToAll(&workersUsed, "DONE\n")
						}
					}
					mutexWorkers.Unlock()
					mutexTurnsWorld.Unlock()
				}

				mutexTurnsWorld.Lock()
				world = receiveWorldFromWorkers(height, sectionHeight, width, (*messagesWorker)[:threads])
				mutexTurnsWorld.Unlock()
			}

			// Once it has done all the iterations, send a message to the controller to let it know it is done
			mutexDone.Lock()
			done = true
			mutexControllers.Lock()
			sendToAll(controllers, "DONE\n")
			mutexControllers.Unlock()
			mutexDone.Unlock()

			// Send the world back to the controller
			mutexControllers.Lock()
			for _, conn := range *controllers {
				sendWorld(world, conn, completedTurns)
			}
			mutexControllers.Unlock()

			fmt.Printf("Computed %d turns of %dx%d\n", completedTurns, height, width)

			mutexControllers.Lock()
			*controllers = (*controllers)[:0] // Clear the controllers once processing the current board is finished
			mutexControllers.Unlock()
		}
	}
}
