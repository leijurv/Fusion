package main

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/howardstark/fusion/protos"
)

const (
	BUF_SIZE = 65536
)

type SessionID uint64

type Connection struct {
	conn                       net.Conn
	lastSuccessfulDataTransfer uint64 //idk more fields here
}

type Session struct {
	sessionID          SessionID
	sshConn            *net.Conn
	conns              []*Connection
	lock               sync.Mutex
	outgoingSeq        uint32
	incomingSeq        uint32
	inflight           map[uint32]*[]byte
	incomingLock       sync.Mutex
	highestReceivedSeq uint32
}

var sessionsLock sync.Mutex

var (
	sessions = make(map[SessionID]*Session)
)

func NewSessionID() SessionID {
	b := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		fmt.Println("Defaulting nonrandom session id 1", err)
		return SessionID(420) // Sensible defaults amirite?
	}
	return SessionID(binary.LittleEndian.Uint64(b))
}

func getSession(id SessionID) *Session {
	sessionsLock.Lock()
	defer sessionsLock.Unlock()
	sess, ok := sessions[id]
	if !ok || sess == nil {
		sess = &Session{
			sessionID: id,
			inflight:  make(map[uint32]*[]byte),
		}
		sessions[id] = sess
	}
	return sess
}

func newSession() *Session {
	sessionsLock.Lock()
	defer sessionsLock.Unlock()
	ID := NewSessionID() //generate session ID
	_, ok := sessions[ID]
	if ok {
		//omfg collision??
		// TODO: Check for collisions since they are much more likely given the nature of our random number generator
		fmt.Println("Session id collision detected at id: ", ID)
		return newSession() //recursion solves everything
	}
	sess := &Session{
		sessionID: ID,
		inflight:  make(map[uint32]*[]byte),
	}
	sessions[ID] = sess
	return sess
}

func ServerReceivedClientConnection(conn net.Conn) error {
	var id int64
	err := binary.Read(conn, binary.LittleEndian, &id)
	if err != nil {
		return err
	}
	fmt.Println("Server received connection", conn, " for session id", id)
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
	go sess.addConnAndListen(conn)
	return nil
}

func ClientCreateServerConnection(conn net.Conn, id SessionID) error {
	err := binary.Write(conn, binary.LittleEndian, uint64(id))
	if err != nil {
		return err
	}
	sessionsLock.Lock()
	defer sessionsLock.Unlock()
	sess, ok := sessions[id]
	if !ok {
		return errors.New("Existing ssh conn for id '" + string(id) + "' not found...")
	}
	fmt.Println("Client creating new server conn for session id", id, "and", conn)
	sess.addConnAndListen(conn)
	return nil
}

func ClientReceivedSSHConnection(ssh net.Conn, serverAddr string) error {
	sess := newSession()
	fmt.Println("Client received new ssh conn", ssh, "and gave it id ", sess.sessionID)
	sess.sshConn = &ssh

	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		return err
	}

	ClientCreateServerConnection(conn, sess.sessionID)

	go sess.listenSSH()
	return nil
}

func (sess *Session) getOutgoingSeq() uint32 { // actually only one thread will be touching outgoingSeq, but still good practice =)))))
	sess.lock.Lock()
	defer sess.lock.Unlock()
	seq := sess.outgoingSeq
	sess.outgoingSeq++
	return seq
}
func (sess *Session) kill() {
	sess.lock.Lock()
	defer sess.lock.Unlock()
	if sess.sshConn != nil {
		(*sess.sshConn).Close()
	}
	for i := 0; i < len(sess.conns); i++ {
		sess.conns[i].conn.Close()
	}
	sessionsLock.Lock()
	defer sessionsLock.Unlock()
	delete(sessions, sess.sessionID)
}
func (sess *Session) sendPacket(serialized []byte) {
	fmt.Println("Sending out packet", len(sess.conns))
	for i := 0; i < len(sess.conns); i++ {
		fmt.Println("Writing")
		sess.conns[i].conn.Write(serialized)
	}
}

func (sess *Session) listenSSH() error {
	for {
		buf := make([]byte, BUF_SIZE)
		n, err := (*sess.sshConn).Read(buf)
		if err != nil {
			fmt.Println("Read errrr", err)
			go sess.kill()
			return err
		}
		fmt.Println("Read", n, "bytes from ssh")
		buf = buf[:n]
		if len(buf) < 10 {
			fmt.Println(len(buf), "too small to split")
			sess.sendPacket(sess.wrap(buf))
			continue
		}

		parts := 5
		partSize := len(buf) / parts

		fmt.Println(len(buf), "gonna split")
		packets := make([][]byte, parts+1)
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
			panic("")
		}

		for len(packets) > 0 {
			i := uint64(NewSessionID()) % uint64(len(packets))
			sess.sendPacket(packets[i])
			packets = append(packets[:i], packets[i+1:]...)
		}
	}
}

func (sess *Session) wrap(data []byte) []byte {
	seq := sess.getOutgoingSeq()
	fmt.Println("Wrapping packet with seq", seq)
	packet := packets.Packet{
		Body: &packets.Packet_Data{
			Data: &packets.Data{
				SequenceID: seq,
				Content:    data,
			},
		},
	}
	fmt.Println("Constructed")
	packetData, packetErr := proto.Marshal(&packet)
	fmt.Println("Marshal")
	if packetErr != nil {
		fmt.Println("Marshal error", packetErr)
		return nil
	}
	if len(packetData) > 4294967296 {
		fmt.Println(errors.New("Packet was too big"))
		return nil
	}
	packetLen := make([]byte, 4)
	binary.LittleEndian.PutUint32(packetLen, uint32(len(packetData)))
	packetData = append(packetLen, packetData...)
	fmt.Println("Done marshal")
	return packetData
}
func (sess *Session) addConnAndListen(netconn net.Conn) {
	sess.lock.Lock()
	defer sess.lock.Unlock()
	conn := &Connection{
		conn: netconn,
	}
	sess.conns = append(sess.conns, conn)
	go connListen(sess, conn)
}
func (sess *Session) checkInflight() {
	for {
		seq := sess.incomingSeq
		data, ok := sess.inflight[seq]
		if !ok {
			fmt.Println("Still waiting on", seq)
			return
		}
		sess.writeSSH(*data)
		delete(sess.inflight, seq)
		sess.incomingSeq++
	}
}
func (sess *Session) onReceiveData(sequenceID uint32, data []byte) {
	sess.incomingLock.Lock()
	defer sess.incomingLock.Unlock()
	if sequenceID > sess.highestReceivedSeq {
		sess.highestReceivedSeq = sequenceID
	}
	if sess.incomingSeq == sequenceID {
		fmt.Println("Ordered seq matches good, now waiting on", sess.incomingSeq+1)
		sess.writeSSH(data)
		sess.incomingSeq++
		sess.checkInflight()
		return
	}
	if sess.incomingSeq > sequenceID {
		fmt.Println("Dupe somehow? expecting", sess.incomingSeq, "got", sequenceID)
		return
	}
	fmt.Println("Out of order, expecting", sess.incomingSeq, "got", sequenceID)
	sess.inflight[sequenceID] = &data
	sess.checkInflight()
}
func (sess *Session) writeSSH(data []byte) {
	fmt.Println("Sending", len(data), "bytes to ssh")
	(*sess.sshConn).Write(data)
}

func connListen(sess *Session, conn *Connection) error {
	fmt.Println("Beginning conn listen")
	for {
		fmt.Println("Waiting for packet...")
		packet, packetErr := readProtoPacket(conn)
		fmt.Println("Got packet...")
		if packetErr != nil {
			fmt.Println("Read err", packetErr)
			return packetErr
		}
		switch packet.GetBody().(type) {
		case *packets.Packet_Data:
			sess.onReceiveData(packet.GetData().GetSequenceID(), packet.GetData().Content)
		}
	}
}

func readProtoPacket(conn *Connection) (packets.Packet, error) {
	var packet packets.Packet
	packetLen := make([]byte, 4)
	_, lenErr := io.ReadFull(conn.conn, packetLen)
	if lenErr != nil {
		return packet, lenErr
	}
	l := binary.LittleEndian.Uint32(packetLen)
	fmt.Println("Reading packet of length", l)
	packetData := make([]byte, l)
	_, dataErr := io.ReadFull(conn.conn, packetData)
	if dataErr != nil {
		return packet, dataErr
	}
	unmarshErr := proto.Unmarshal(packetData, &packet)
	return packet, unmarshErr
}