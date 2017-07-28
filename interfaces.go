package main

import (
	"fmt"
	"net"
	"time"

	"crypto/sha256"
	"encoding/binary"

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
		tcpServerAddr, tcpServerErr := net.ResolveTCPAddr("tcp", serverAddr)
		if tcpServerErr != nil {
			fmt.Println("Stopping scan because server err", tcpServerErr)
			return tcpServerErr
		}
		////var udpServerAddr *net.UDPAddr
		//var udpServerErr error
		//if len(*flagUDPInterfaces) > 0 {
		//	udpServerAddr, udpServerErr = net.ResolveUDPAddr("udp", serverAddr)
		//	if udpServerErr != nil {
		//		return udpServerErr
		//	}
		//}
		session := getSession(sessionID)
		for _, iface := range ifaces {
			connErr := startConnectionFromIface(session, iface, tcpServerAddr)
			if connErr != nil {
				return connErr
			}
		}
		time.Sleep(time.Second * time.Duration(flagIfacePoll))
	}
}

func startConnectionFromIface(session *Session, iface net.Interface, tcpServerAddr *net.TCPAddr) error {
	session.lock.Lock()
	for _, conn := range session.conns {
		if iface.Name == conn.GetInterfaceName() {
			session.lock.Unlock()
			return nil
		}
	}
	session.lock.Unlock()
	addrs, addrErr := iface.Addrs()
	if addrErr != nil {
		fmt.Println("Stopping scan because addr err", addrErr)
		return addrErr
	}
	connection := buildConnectionFromAddrs(addrs, tcpServerAddr, iface)
	if connection == nil {
		return nil
	}
	fmt.Println(ClientCreateServerConnection(connection, session.sessionID))
	data := marshal(&packets.Packet{Body: &packets.Packet_Control{Control: &packets.Control{Timestamp: time.Now().UnixNano(), Redundant: flagRedundant}}})
	getSession(session.sessionID).redundant = flagRedundant
	go getSession(session.sessionID).sendOnAll(data)
	return nil
}

func buildConnectionFromAddrs(addrs []net.Addr, tcpServerAddr *net.TCPAddr, iface net.Interface) *Connection {
	for _, addr := range addrs {
		fmt.Println("ATTEMPTING", iface, addr)
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
		interimID := sha256.Sum256([]byte(iface.Name))
		ifaceID := binary.BigEndian.Uint64(interimID[:8])
		return &Connection{
			iface:   iface.Name,
			ifaceID: ifaceID,
			conn:    conn,
		}
	}
	return nil
}
