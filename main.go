package main

import (
	"flag"
	"fmt"
	"net"
	"time"
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
			err := SetupInterfaces(sessionID, serverAddr)
			if err != nil {
				fmt.Println("SEARCHFORME Error with", conn, err)
			}
		}()
	}
}

func SetupInterfaces(sessionID SessionID, serverAddr string) error {
	for {
		ifaces, ifaceErr := net.Interfaces()
		if ifaceErr != nil {
			return ifaceErr
		}
		tcpServerAddr, serverErr := net.ResolveTCPAddr("tcp", serverAddr)
		if serverErr != nil {
			return serverErr
		}
		for _, iface := range ifaces {
			session := getSession(sessionID)
			var active bool = false
			session.lock.Lock()
			for _, conn := range session.conns {
				fmt.Println(conn.iface)
				if iface.Name == conn.iface {
					active = true
					break
				}
			}
			session.lock.Unlock()
			if active {
				continue
			}
			addrs, addrErr := iface.Addrs()
			if addrErr != nil {
				return addrErr
			}
			for _, addr := range addrs {
				fmt.Println("ATTEMPTING", iface, addr)
				fmt.Println("serverAddr: ", serverAddr)
				ip, _, ipErr := net.ParseCIDR(addr.String())
				if ipErr != nil {
					fmt.Println(ipErr)
					continue
				}
				tcpLocalAddr, localErr := net.ResolveTCPAddr("tcp", ip.String()+":0")
				if ip.IsLinkLocalMulticast() {
					continue
				}
				if localErr != nil {
					fmt.Println(localErr)
					continue
				}
				fmt.Println("tcpLocalAddr: ", tcpLocalAddr)
				conn, err := net.DialTCP("tcp", tcpLocalAddr, tcpServerAddr)
				if err != nil {
					fmt.Println(err)
					continue
				}
				connection := &Connection{
					iface: iface.Name,
					conn:  conn,
				}
				ClientCreateServerConnection(connection, sessionID) //this just makes two connections over the same interface (for testing)
			}
		}
		time.Sleep(time.Second * 1)
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
