package main

import (
	"flag"
	"fmt"
	"net"
)

var flagListenMode bool
var flagAddress string

func init() {
	flag.BoolVar(&flagListenMode, "l", false, "Should listen?")
	flag.StringVar(&flagAddress, "address", "localhost:5022", "Address of the server")
}

func main() {
	flag.Parse()

	if !flagListenMode {
		err := Client(flagAddress)
		if err != nil {
			panic(err)
		}
	} else {
		err := Server()
		if err != nil {
			panic(err)
		}
	}
}

func Client(serverAddr string) error {
	fmt.Println("Starting client...")
	ln, err := net.Listen("tcp", ":5021")
	if err != nil {
		return err
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go ClientReceivedSSHConnection(conn, serverAddr)
	}
}

func Server() error {
	fmt.Println("Starting server...")
	ln, err := net.Listen("tcp", ":5022")
	if err != nil {
		return err
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go ServerReceivedClientConnection(conn)
	}
}
