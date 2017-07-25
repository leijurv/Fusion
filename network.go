package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	mrand "math/rand"
	"net"
	"time"

	"github.com/howardstark/fusion/protos"
)

const (
	BUF_SIZE     = 65536
	RAND_REORDER = false
)

type Connection struct {
	iface                      string
	conn                       net.Conn
	lastSuccessfulDataTransfer uint64 //idk more fields here
}

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
	go func() {
		fmt.Println("AAAAAAAAAAAAAAAAAAAAAAAAHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHh")
		sess.addConnAndListen(&Connection{conn: conn})
	}()
	return nil
}

func ClientCreateServerConnection(conn *Connection, id SessionID) error {
	conn.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	err := binary.Write(conn.conn, binary.LittleEndian, uint64(id))
	if err != nil {
		return err
	}
	var verify uint64
	err = binary.Read(conn.conn, binary.BigEndian, &verify) // verifies that proper 2-way communication is happening before adding to list of conns
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
	fmt.Println("Client creating new server conn for session id", id, "and", conn.conn)
	sess.addConnAndListen(conn)
	return nil
}

func ClientReceivedSSHConnection(ssh net.Conn) SessionID {
	sess := newSession()
	fmt.Println("Client received new ssh conn", ssh, "and gave it id ", sess.sessionID)
	sess.sshConn = &ssh
	go sess.listenSSH()
	return sess.sessionID
}

func (sess *Session) sendPacket(sent *Sent) {
	if sess.redundant {
		fmt.Println("SENDING REDUNDANT")
		sess.sendOnAll(*sent.data)
		sent.date = time.Now().UnixNano()
		return
	}
	sess.lock.Lock()
	defer sess.lock.Unlock()
	available := make([]*Connection, 0)

	alreadyUsedMap := make(map[*Connection]bool)
	for i := 0; i < len(sent.sentOn); i++ {
		alreadyUsedMap[sent.sentOn[i]] = true
	}

	for i := 0; i < len(sess.conns); i++ {
		_, ok := alreadyUsedMap[sess.conns[i]]
		if !ok {
			available = append(available, sess.conns[i])
		}
	}
	var connSelection *Connection
	if len(available) == 0 {
		if len(sess.conns) == 0 {
			fmt.Println("OH NO UH IDK WHAT TO DO")
			return
		}
		ind := mrand.New(mrand.NewSource(time.Now().UnixNano())).Intn(len(sess.conns))
		fmt.Println("", ind, sess.conns[ind].conn.LocalAddr(), sess.conns[ind].conn.RemoteAddr())
		connSelection = sess.conns[ind] // do this step in lock
	} else {
		ind := mrand.New(mrand.NewSource(time.Now().UnixNano())).Intn(len(available))
		fmt.Println("Selected conn", available[ind].conn.LocalAddr(), available[ind].conn.RemoteAddr())
		connSelection = available[ind] // do this step in lock
	}

	sent.sentOn = append(sent.sentOn, connSelection)
	c := connSelection.conn
	sent.date = time.Now().UnixNano()
	go c.Write(*sent.data) // haha yes
	// do actual write outside of lock
}
func (sess *Session) sendOnAll(serialized []byte) {
	fmt.Println("Sending out packet to", len(sess.conns), "destinations")
	sess.lock.Lock() // lock is ok because we are starting goroutines to do the blocking io
	defer sess.lock.Unlock()
	for i := 0; i < len(sess.conns); i++ {
		//fmt.Println("Writing")
		go sess.conns[i].conn.Write(serialized) // goroutine is fine because order doesn't matter
	}
}

func (sess *Session) listenSSH() error {
	for {
		buf := make([]byte, BUF_SIZE)
		n, err := (*sess.sshConn).Read(buf)
		if err != nil {
			fmt.Println("SSH read err, killing session", sess.sessionID, err)
			go sess.kill()
			return err
		}
		if len(sess.conns) == 0 {
			fmt.Println("listenssh waiting for connections...  ", sess.sessionID)
		}
		for len(sess.conns) == 0 { // no point in reading from ssh if the data has nowhere to go
			time.Sleep(10 * time.Millisecond) // block until nonempty
			if uint64(sess.sessionID) == 0 {
				fmt.Println("listenssh dying because session killed")
				return nil
			}
		}
		fmt.Println("Read", n, "bytes from ssh")
		if !RAND_REORDER {
			sess.sendPacket(sess.wrap(buf[:n]))
		} else {
			buf = buf[:n]
			if len(buf) < 10 {
				fmt.Println(len(buf), "too small to split")
				sess.sendPacket(sess.wrap(buf))
				continue
			}

			parts := 5
			partSize := len(buf) / parts

			fmt.Println(len(buf), "gonna split")
			packets := make([]*Sent, parts+1)
			totalSize := 0
			for i := 0; i < parts; i++ {
				tmp := buf[i*partSize : (i+1)*partSize]
				totalSize += len(tmp)
				packets[i] = sess.wrap(tmp)
			}
			tmp := buf[parts*partSize:]
			packets[parts] = sess.wrap(tmp)
			totalSize += len(tmp)
			if len(buf) != totalSize {
				fmt.Println("Expected len ", len(buf), "got", totalSize, "packetslen", len(packets))
				panic("") // somehow the splitter and reorderer ended up with the wrong number of total bytes
			}

			rSrc := mrand.New(mrand.NewSource(time.Now().UnixNano()))
			perm := rSrc.Perm(len(packets))
			for i := 0; i < len(perm); i++ {
				sess.sendPacket(packets[perm[i]])
			}
		}
		if n < BUF_SIZE/5 {
			time.Sleep(10 * time.Millisecond) //we only read less than 1/5 of the buffer, give ssh some time to chill
		}
	}
}

func (sess *Session) addConnAndListen(conn *Connection) {
	sess.lock.Lock()
	defer sess.lock.Unlock()
	sess.conns = append(sess.conns, conn)
	go func() {
		err := connListen(sess, conn)
		if err != nil {
			fmt.Println("conn listen err", err)
			sess.removeConn(conn)
		}
	}()
}

func (sess *Session) writeSSH(data []byte) {
	fmt.Println("Sending", len(data), "bytes to ssh")
	(*sess.sshConn).Write(data)
}

func connListen(sess *Session, conn *Connection) error {
	fmt.Println("Beginning conn listen")
	for {
		//fmt.Println("Waiting for packet...")
		conn.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		packet, packetErr, rawPacket := readProtoPacket(conn)
		//fmt.Println("Got packet...")
		if packetErr != nil {
			fmt.Println("Read err", packetErr)
			return packetErr
		}
		if dedup(packet, rawPacket) {
			continue
		}
		switch packet.GetBody().(type) {
		case *packets.Packet_Data:
			go sess.onReceiveData(packet.GetData().GetSequenceID(), packet.GetData().GetContent())
		case *packets.Packet_Status:
			go sess.onReceiveStatus(packet.GetStatus().GetIncomingSeq(), packet.GetStatus().GetTimestamp(), packet.GetStatus().GetInflight())
		case *packets.Packet_Control:
			go sess.onReceiveControl(packet.GetControl().GetTimestamp(), packet.GetControl().GetRedundant())
		}
	}
	panic("conn listen exited loop without returning err")
}
