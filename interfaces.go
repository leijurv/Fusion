package main

import (
	"fmt"
	"net"
	"time"

	"github.com/howardstark/fusion/protos"
)

func SetupInterfaces(sessionID SessionID, serverAddr string) error {
	defer func() {
		fmt.Println("setupinterfaces killing", sessionID, "because scan failed")
		getSession(sessionID).kill() // when the scan is over, the session is over
	}()
	for {
		if !hasSession(sessionID) {
			fmt.Println("STOPPING SCAN")
			return nil
		}
		ifaces, ifaceErr := net.Interfaces()
		if ifaceErr != nil {
			fmt.Println("Stopping scan because iface err", ifaceErr)
			return ifaceErr
		}
		tcpServerAddr, serverErr := net.ResolveTCPAddr("tcp", serverAddr)
		if serverErr != nil {
			fmt.Println("Stopping scan because server err", serverErr)
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
				fmt.Println("Stopping scan because addr err", addrErr)
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
				fmt.Println("DIALING DIALING DIALING tcpLocalAddr: ", tcpLocalAddr)
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
				data := marshal(&packets.Packet{Body: &packets.Packet_Control{Control: &packets.Control{Timestamp: time.Now().UnixNano(), Redundant: flagRedundant || flagRedundantDownload}}})
				getSession(sessionID).redundant = flagRedundant || flagRedundantUpload
				go getSession(sessionID).sendOnAll(data)
			}
		}
		time.Sleep(time.Second * time.Duration(flagIfacePoll))
	}
}
