package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"
)

func ServerReceivedClientConnection(conn net.Conn) error {
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var id uint64
	err := binary.Read(conn, binary.LittleEndian, &id)
	if err != nil {
		return err
	}
	fmt.Println("Server received connection", conn, " for session id", id, "from remote", conn.RemoteAddr(), "and local", conn.LocalAddr())
	err = binary.Write(conn, binary.BigEndian, id) //big endian prevents simple reflection
	if err != nil {
		return err
	}
	sess := getSession(SessionID(id))
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
	sess.addConnAndListen(&TcpConnection{conn: conn})
	return nil
}

func ClientCreateServerConnection(conn Connection, id SessionID) error {
	conn.(*TcpConnection).conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	err := binary.Write(conn.(*TcpConnection).conn, binary.LittleEndian, uint64(id)) //TODO
	if err != nil {
		return err
	}
	var verify uint64
	//TODO
	err = binary.Read(conn.(*TcpConnection).conn, binary.BigEndian, &verify) // verifies that proper 2-way communication is happening before adding to list of conns
	if err != nil {
		return err
	}
	if verify != uint64(id) {
		err = errors.New("ID response mismatch " + string(verify) + "  " + string(id))
		fmt.Println(err)
		return err
	}
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
