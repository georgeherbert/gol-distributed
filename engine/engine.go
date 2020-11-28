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

func handleNewConnections(lnController net.Listener, messagesController chan<- string, mutexControllers *sync.Mutex, controllers *[]net.Conn) {
	for {
		controller, _ := lnController.Accept()
		fmt.Println("New controller")
		go handleConnection(controller, messagesController)
		mutexControllers.Lock()
		*controllers = append(*controllers, controller)
		mutexControllers.Unlock()
	}
}

// Converts a string receives over tcp to an integer
func netStringToInt(netString string) int {
	integer, _ := strconv.Atoi(netString[:len(netString)-1])
	return integer
}

func sendWorldToWorker(height int, width int, worker net.Conn, messages <-chan string) {
	for i := 0; i < height*width; i++ {
		fmt.Fprintf(worker, <-messages)
	}
}

func sendToAllControllers(controllers *[]net.Conn, value string) {
	for _, conn := range *controllers {
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
				sendToAllControllers(controllers, "REPORT_ALIVE\n")
				sendToAllControllers(controllers, fmt.Sprintf("%d\n", *completedTurns))
				sendToAllControllers(controllers, fmt.Sprintf("%d\n", *numAliveCells))
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
				sendToAllControllers(controllers, "RESUMING\n")
			} else {
				sendToAllControllers(controllers, "PAUSING\n")
			}
			paused = !paused
			sendToAllControllers(controllers, fmt.Sprintf("%d\n", *completedTurns))
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

func receiveWorldFromWorker(height int, width int, messages <-chan string) [][]byte {
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
	portControllerPtr := flag.String("port_controller", ":8030", "port to listen on for controllers")
	portWorkerPtr := flag.String("port_worker", ":8040", "port to listen on")
	flag.Parse()
	lnController, _ := net.Listen("tcp", *portControllerPtr)
	messagesController := make(chan string)
	mutexControllers := &sync.Mutex{} // Used whenever sending data to client to stop multiple things being sent at once
	controllers := new([]net.Conn)
	go handleNewConnections(lnController, messagesController, mutexControllers, controllers)

	// Workers stuff
	lnWorker, _ := net.Listen("tcp", *portWorkerPtr)
	var workers []net.Conn
	worker, _ := lnWorker.Accept()
	fmt.Println("New worker")
	workers = append(workers, worker)
	messagesWorker := make(chan string)
	go handleConnection(worker, messagesWorker)

	for {
		if <-messagesController == "INITIALISE\n" { // This stops a new connection attempting to rejoin once all turns are complete breaking the engine
			heightString, widthString, turnsString := <-messagesController, <-messagesController, <-messagesController
			height, width, turns := netStringToInt(heightString), netStringToInt(widthString), netStringToInt(turnsString)
			done := false
			mutexDone := &sync.Mutex{}
			completedTurns := 0
			mutexTurnsWorld := &sync.Mutex{}
			var world [][]byte // temporary while setting up worker
			if turns > 0 {     // If there are more than 0 turns, process them
				fmt.Println("Received details")
				fmt.Fprintf(worker, heightString)
				fmt.Fprintf(worker, widthString)
				sendWorldToWorker(height, width, worker, messagesController)
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
					numAliveCellsString := <-messagesWorker
					numAliveCells = netStringToInt(numAliveCellsString)
					completedTurns = turn + 1
					select {
					case <-send:
						fmt.Fprintf(worker, "SEND_WORLD\n")
						world = receiveWorldFromWorker(height, width, messagesWorker)
						send <- true
					default:
						if completedTurns != turns {
							fmt.Fprintf(worker, "CONTINUE\n")
						} else {
							fmt.Fprintf(worker, "DONE\n")
						}
					}
					mutexTurnsWorld.Unlock()
				}
				mutexTurnsWorld.Lock()
				world = receiveWorldFromWorker(height, width, messagesWorker)
				mutexTurnsWorld.Unlock()
			} else { // If turns == 0, just initialise the world variable as the values from the controller
				mutexTurnsWorld.Lock()
				world = initialiseWorld(height, width, messagesController)
				mutexTurnsWorld.Unlock()
			}
			// Once it has done all the iterations, send a message to the controller to let it know it is done
			mutexDone.Lock()
			done = true
			mutexControllers.Lock()
			sendToAllControllers(controllers, "DONE\n")
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
