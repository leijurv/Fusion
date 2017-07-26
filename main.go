package main

import (
	"flag"
	"fmt"
	"net"
	"time"

	"github.com/howardstark/fusion/protos"
)

var flagListenMode bool
var flagAddress string
var flagIfacePoll int
var flagRedundant bool

func init() {
	flag.BoolVar(&flagRedundant, "r", false, "Send packets on every interface instead of just one? Improves reliability.")
	flag.BoolVar(&flagListenMode, "l", false, "Should listen?")
	flag.StringVar(&flagAddress, "address", "localhost:5022", "Address of the server")
	flag.IntVar(&flagIfacePoll, "poll", 5, "How fast we should poll for new interfaces")

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
		if !hasSession(sessionID) {
			fmt.Println("STOPPING SCAN")
			return nil
		}
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
				if iface.Name == conn.(*TcpConnection).iface {
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
				connection := &TcpConnection{
					iface: iface.Name,
					conn:  conn,
				}
				fmt.Println(ClientCreateServerConnection(connection, sessionID))
				data := marshal(&packets.Packet{Body: &packets.Packet_Control{Control: &packets.Control{Timestamp: time.Now().UnixNano(), Redundant: flagRedundant}}})
				getSession(sessionID).redundant = flagRedundant
				go getSession(sessionID).sendOnAll(data)
			}
		}
		time.Sleep(time.Second * time.Duration(flagIfacePoll))
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
