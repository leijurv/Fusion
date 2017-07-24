package main

import (
	"fmt"
	"net"
)

func main() {
	fmt.Println("Hello!")
}

func Client(serverAddr string) error {
	ln, err := net.Listen("tcp", ":5021")
	if err != nil {
		return err
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go ClientReceivedSSHConnection(&conn, serverAddr)
	}
}

func Server() error {
	ln, err := net.Listen("tcp", ":5022")
	if err != nil {
		return err
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go ServerReceivedClientConnection(&conn)
	}
}
