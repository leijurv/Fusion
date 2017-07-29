package main

import (
	"net"
	"time"

	"crypto/sha256"
	"encoding/binary"

	log "github.com/sirupsen/logrus"
)

func SetupInterfaces(sessionID SessionID, serverAddr string) error {
	defer func() {
		log.WithField("sess-id", sessionID).Warning("Killing SetupInterfaces because scan failed")
		getSession(sessionID).kill() // when the scan is over, the session is over
	}()
	for {
		if !hasSession(sessionID) {
			log.WithField("sess-id", sessionID).Debug("Stopping scan because session does not exist")
			return nil
		}
		ifaces, ifaceErr := net.Interfaces()
		if ifaceErr != nil {
			log.WithError(ifaceErr).Error("Stopping scan because iface err")
			return ifaceErr
		}
		tcpServerAddr, tcpServerErr := net.ResolveTCPAddr("tcp", serverAddr)
		if tcpServerErr != nil {
			log.WithError(tcpServerErr).Error("Stopping scan because server err")
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
			if len(flagInterfaces.contents) > 0 {
				_, ok := flagInterfaces.contents[iface.Name]
				if !ok {
					break
				}
			}
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
		log.WithError(addrErr).Error("Stopping scan because addr err")
		return addrErr
	}
	connection := buildConnectionFromAddrs(addrs, tcpServerAddr, iface, session.sessionID)
	if connection == nil {
		return nil
	}

	return nil
}

func buildConnectionFromAddrs(addrs []net.Addr, tcpServerAddr *net.TCPAddr, iface net.Interface, id SessionID) *Connection {
	for _, addr := range addrs {
		log.WithFields(log.Fields{
			"iface": iface.Name,
			"addr":  addr,
		}).Debug("Attempting connection")
		ip, _, ipErr := net.ParseCIDR(addr.String())
		if ipErr != nil {
			log.WithError(ipErr).Error("Could not parse CIDR")
			continue
		}
		if ip.IsLinkLocalMulticast() {
			continue
		}
		tcpLocalAddr, localErr := net.ResolveTCPAddr("tcp", ip.String()+":0")
		if localErr != nil {
			log.WithError(localErr).Warning("Could not resolve tcp address")
			continue
		}
		log.WithField("local-addr", tcpLocalAddr).Debug("Dialing TCP conn...")
		conn, err := net.DialTCP("tcp", tcpLocalAddr, tcpServerAddr)
		if err != nil {
			log.Error("TCP Dial error", err)
			continue
		}
		if iface.Name == "" {
			log.Error("Empty interface name")
			continue
		}
		interimID := sha256.Sum256([]byte(iface.Name))
		ifaceID := binary.BigEndian.Uint64(interimID[:8])
		connection := &Connection{
			iface:   iface.Name,
			ifaceID: ifaceID,
			conn:    conn,
		}
		connErr := ClientCreateServerConnection(connection, id)
		if connErr != nil {
			log.WithFields(log.Fields{
				"conn":    connection,
				"sess-id": id,
			}).WithError(connErr).Error("Could not create conn...")
			continue
		}
		return connection
	}
	return nil
}
