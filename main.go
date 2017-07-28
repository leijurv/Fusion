package main

import (
	"flag"
	"fmt"
	"net"
)

type stringArrayVar []string

func (s *stringArrayVar) String() string {
	return "we probably don't need this?"
}

func (s *stringArrayVar) Set(value string) error {
	*s = append(*s, value)
	return nil
}

var flagListenMode bool
var flagAddress string
var flagIfacePoll int
var flagRedundant bool
var (
	flagUDPInterfaces = new(stringArrayVar)
)

func init() {
	flag.BoolVar(&flagRedundant, "r", false, "Send packets on every interface instead of just one? (Improves reliability)")
	flag.BoolVar(&flagListenMode, "l", false, "Should listen?")
	flag.StringVar(&flagAddress, "address", "localhost:5022", "Address of the server")
	flag.IntVar(&flagIfacePoll, "poll", 5, "How fast we should poll for new interfaces")
	flag.Var(flagUDPInterfaces, "udp-interface", "The name of the interface you wish to use as UDP instead of TCP (This is not recommended for a single interface, as retrying on UDP will not be attempted")
	flag.Var(flagUDPInterfaces, "ui", "Shorthand for the 'udp-interface' parameter. See 'udp-interface' for usage")
	//flag.Memes
}

func main() {
	flag.Parse()

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
