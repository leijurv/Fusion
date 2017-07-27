package main

import (
	"flag"
	"fmt"
	"net"
)

var flagListenMode bool
var flagAddress string
var flagIfacePoll int
var flagRedundant bool
var flagRedundantDownload bool
var flagRedundantUpload bool

func init() {
	flag.BoolVar(&flagRedundant, "r", false, "Send packets on every interface instead of just one? Improves reliability.")
	flag.BoolVar(&flagRedundantDownload, "rd", false, "Redundant mode only for downloads")
	flag.BoolVar(&flagRedundantUpload, "ru", false, "Redundant mode only for uploads")
	flag.BoolVar(&flagListenMode, "l", false, "Should listen?")
	flag.StringVar(&flagAddress, "address", "localhost:5022", "Address of the server")
	flag.IntVar(&flagIfacePoll, "poll", 5, "How fast we should poll for new interfaces")
	//flag.Memes
}

func main() {
	flag.Parse()
	if flagListenMode {
		if flagRedundant || flagRedundantUpload || flagRedundantDownload {
			fmt.Println("In listen mode, the redundant flags have no effect. The client that makes the connection decides it.")
			return
		}
	}
	if flagListenMode {
		err := Server()
		if err != nil {
			panic(err)
		}
	} else {
		err := Client(flagAddress)
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
		go func() {
			sessionID := ClientReceivedSSHConnection(conn)
			err := SetupInterfaces(sessionID, serverAddr)
			if err != nil {
				fmt.Println("SEARCHFORME Error with", conn, err)
			}
		}()
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
		go func() {
			err := ServerReceivedClientConnection(conn)
			if err != nil {
				fmt.Println("Error with", conn, err)
			}
		}()
	}
}
