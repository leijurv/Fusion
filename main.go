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
		go func() {
			sessionID := ClientReceivedSSHConnection(conn)
			err := SetupClient(sessionID, serverAddr)
			if err != nil {
				fmt.Println("Error with", conn, err)
			}
		}()
	}
}

func SetupClient(sessionID SessionID, serverAddr string) error {
	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		return err
	}
	ClientCreateServerConnection(conn, sessionID) // would be DANK if it made connections over every available interface. No.
	ifaces, ifaceErr := net.Interfaces()
	if ifaceErr != nil {
		return ifaceErr
	}
	for _, iface := range ifaces {
		addrs, addrErr := iface.Addrs()
		if addrErr != nil {
			return addrErr
		}
		for _, addr := range addrs {
			if addr.Network()[:3] != "tcp" {
				continue
			}
			serverAddr, serverErr := net.ResolveTCPAddr("tcp", serverAddr)
			if serverErr != nil {
				return serverErr
			}
			fmt.Println("serverAddr: ", serverAddr)
			localAddr, localErr := net.ResolveTCPAddr("tcp", addr.String())
			if localErr != nil {
				return localErr
			}
			fmt.Println("localAddr: ", localAddr)
			conn, err = net.DialTCP("tcp", localAddr, serverAddr)
			if err != nil {
				return err
			}
			ClientCreateServerConnection(conn, sessionID) //this just makes two connections over the same interface (for testing)
		}
	}
	return nil
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
