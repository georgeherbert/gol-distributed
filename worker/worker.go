package main

import (
	"bufio"
	"fmt"
	"net"
)

func main() {
	conn, _ := net.Dial("tcp", "127.0.0.1:8040")
	reader := bufio.NewReader(conn)

	msg, _ := reader.ReadString('\n')
	fmt.Println(msg)
}
