package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"

	"crypto/rand"
	"io"

	"bytes"

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
	sessionID   SessionID
	sshConn     *net.Conn
	conns       []*Connection
	lock        sync.Mutex
	outgoingSeq uint32
}

var sessionsLock sync.Mutex

var (
	sessions = make(map[SessionID]*Session)
)

func NewSessionID() SessionID {
	b := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return SessionID(420) // Sensible defaults amirite?
	}
	id, idErr := binary.ReadUvarint(bytes.NewReader(b))
	if idErr != nil {
		return SessionID(69)
	}
	return SessionID(id)
}

func getSession(id SessionID) *Session {
	sessionsLock.Lock()
	defer sessionsLock.Unlock()
	sess, ok := sessions[id]
	if !ok || sess == nil {
		sess = &Session{
			sessionID: id,
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
		sshConn, err := net.Dial("tcp", "localhost:1234")
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
		packet := packets.Packet{
			Body: &packets.Packet_Data{
				Data: &packets.Data{
					SequenceID: sess.getOutgoingSeq(),
					Content:    buf[:n],
				},
			},
		}
		fmt.Println("Constructed")
		packetData, packetErr := proto.Marshal(&packet)
		fmt.Println("Marshal")
		if packetErr != nil {
			fmt.Println("Marshal error", packetErr)
			return errors.New("Run.")
		}
		if len(packetData) > 4294967296 {
			return errors.New("Packet was too big")
		}
		packetLen := make([]byte, 4)
		binary.BigEndian.PutUint16(packetLen, uint16(len(packetData)))
		packetData = append(packetLen, packetData...)

		fmt.Println("Done marshal")
		sess.sendPacket(packetData)
	}
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

func (sess *Session) onReceiveData(sequenceID uint32, data []byte) {
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
		fmt.Println("Received packet", packet.GetBody())
		switch packet.GetBody().(type) {
		case *packets.Packet_Data:
			sess.onReceiveData(packet.GetData().GetSequenceID(), packet.GetData().Content)
		}
	}
}

func readProtoPacket(conn *Connection) (packets.Packet, error) {
	var packet packets.Packet
	packetLen := make([]byte, 2)
	_, lenErr := io.ReadFull(conn.conn, packetLen)
	if lenErr != nil {
		return packet, lenErr
	}
	l := binary.BigEndian.Uint16(packetLen)
	fmt.Println("Reading packet of length", l)
	packetData := make([]byte, l)
	_, dataErr := io.ReadFull(conn.conn, packetData)
	if dataErr != nil {
		return packet, dataErr
	}
	unmarshErr := proto.Unmarshal(packetData, &packet)
	return packet, unmarshErr
}
func Uvarint(buf []byte) (x uint64) {
	for i, b := range buf {
		x = x<<8 + uint64(b)
		if i == 7 {
			return
		}
	}
	return
}
