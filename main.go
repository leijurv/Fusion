package main

import (
	"net"
	"flag"
	"fmt"
)

var flagListenMode bool
var flagAddress string

func init() {
	flag.BoolVar(&flagListenMode, "l", false, "Should listen?")
	flag.StringVar(&flagAddress, "address", "127.0.0.1:5022", "Address of the server")
}

func main() {
	flag.Parse()

	if !flagListenMode {
		err := Client(flagAddress)
		if err != nil {
			panic(err)
		}
	} else {
		Server()
	}
}

func Client(serverAddr string) error {
	fmt.Println("del cliento")
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
	fmt.Println("el servo")
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
