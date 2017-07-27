package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/howardstark/fusion/protos"
)

func ServerReceivedClientConnection(conn net.Conn) error {
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	tcp := &TcpConnection{conn: conn}
	packet, packetErr, _ := readProtoPacket(tcp)
	if packetErr != nil {
		fmt.Println("Read err", packetErr)
		return packetErr
	}
	_, ok := packet.GetBody().(*packets.Packet_Init)
	if !ok {
		fmt.Println("thats NOT a control packet")
		return errors.New("naughty")
	}
	id := SessionID(packet.GetInit().GetSession())
	fmt.Println("Server received connection", conn, " for session id", id, "from remote", conn.RemoteAddr(), "and local", conn.LocalAddr(), "and interface", packet.GetInit().GetInterface())
	err := binary.Write(conn, binary.LittleEndian, uint64(id))
	if err != nil {
		return err
	}
	sess := getSession(SessionID(id))
	sess.redundant = packet.GetInit().GetControl().GetRedundant()
	sess.lock.Lock()
	defer sess.lock.Unlock()
	if sess.sshConn == nil {
		fmt.Println("Server making new ssh connection for session id", id)
		sshConn, err := net.Dial("tcp", "localhost:22")
		if err != nil {
			fmt.Println("localhost dial err", err)
			return err
		}
		sess.sshConn = &sshConn
		go sess.listenSSH()
	}
	fmt.Println("Adding")
	go sess.addConnAndListen(tcp)
	return nil
}

func ClientCreateServerConnection(conn Connection, id SessionID) error {
	conn.(*TcpConnection).conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	err := conn.Write(marshal(&packets.Packet{
		Body: &packets.Packet_Init{
			Init: &packets.Init{
				Session:   uint64(id),
				Interface: 5021,
				Control: &packets.Control{
					Timestamp: time.Now().UnixNano(),
					Redundant: flagRedundant,
				},
			},
		},
	}))
	if err != nil {
		return err
	}
	var verify uint64
	err = binary.Read(conn.(*TcpConnection).conn, binary.LittleEndian, &verify) // verifies that proper 2-way communication is happening before adding to list of conns
	if err != nil {
		return err
	}
	if verify != uint64(id) {
		err = errors.New("ID response mismatch " + string(verify) + "  " + string(id))
		fmt.Println(err)
		return err
	}
	//fmt.Println("ITS WORKING OKAY??")
	sessionsLock.Lock()
	defer sessionsLock.Unlock()
	sess, ok := sessions[id]
	if !ok {
		err = errors.New("Existing ssh conn for id '" + string(id) + "' not found...")
		fmt.Println(err)
		return err
	}
	fmt.Println("Client creating new server conn for session id", id, "and", conn.(*TcpConnection).conn)
	go sess.addConnAndListen(conn) // new goroutine because sessionslock
	return nil
}

func ClientReceivedSSHConnection(ssh net.Conn) SessionID {
	sess := newSession()
	fmt.Println("Client received new ssh conn", ssh, "and gave it id ", sess.sessionID)
	sess.sshConn = &ssh
	go sess.listenSSH()
	return sess.sessionID
}
