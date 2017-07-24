package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/howardstark/fusion/protos"
	"io"
	"crypto/rand"
)

const (
	BUF_SIZE = 65536
)

type SessionID uint64

func NewSessionID() SessionID {
	b := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return SessionID(420) // Sensible defaults amirite?
	}
	return SessionID(b)
}

type Connection struct {
	conn                       *net.Conn
	lastSuccessfulDataTransfer uint64 //idk more fields here
}

type Session struct {
	sessionID   SessionID
	sshConn     *net.Conn
	conns       []*Connection
	lock        sync.Mutex
	outgoingSeq uint32
}

var sessions map[SessionID]*Session
var sessionsLock sync.Mutex

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
		fmt.Println("Session id collision detected at id: ", ID)
		return newSession() //recursion solves everything
	}
	sess := &Session{
		sessionID: ID,
	}
	sessions[ID] = sess
	return sess
}

func ServerReceivedClientConnection(conn *net.Conn) error {
	var id int64
	err := binary.Read(*conn, binary.LittleEndian, &id)
	if err != nil {
		return err
	}
	fmt.Println("Server received connection", conn, " for session id", id, " and ", *conn)
	sess := getSession(SessionID(id))
	sess.lock.Lock()
	defer sess.lock.Unlock()
	if sess.sshConn == nil {
		fmt.Println("Server making new ssh connection for session id", id)
		conn, err := net.Dial("tcp", "localhost:22")
		if err != nil {
			return err
		}
		sess.sshConn = &conn
		go sess.listenSSH()
	}
	sess.addConnAndListen(conn)
	return nil
}

func ClientCreateServerConnection(conn *net.Conn, id SessionID) error {
	err := binary.Write(*conn, binary.LittleEndian, uint64(id))
	if err != nil {
		return err
	}
	sessionsLock.Lock()
	defer sessionsLock.Unlock()
	sess, ok := sessions[id]
	if !ok {
		return errors.New("Existing ssh conn for id '" + string(id) + "' not found...")
	}
	fmt.Println("Client creating new server conn for session id", id, "and", conn, "and", *conn)
	sess.addConnAndListen(conn)
	return nil
}

func ClientReceivedSSHConnection(ssh *net.Conn, serverAddr string) error {
	sess := newSession()
	fmt.Println("Client received new ssh conn", ssh, "and", *ssh, " and gave it id ", sess.sessionID)
	sess.sshConn = ssh

	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		return err
	}

	ClientCreateServerConnection(&conn, sess.sessionID)

	go sess.listenSSH()
	return nil
}

func (sess *Session) getOutgoingSeq() uint32 {
	sess.lock.Lock()
	defer sess.lock.Unlock()
	seq := sess.outgoingSeq
	sess.outgoingSeq++
	return seq
}

func (sess *Session) sendPacket(serialized []byte) {
	for i := 0; i < len(sess.conns); i++ {
		(*sess.conns[i].conn).Write(serialized)
	}
}

func (sess *Session) listenSSH() error {
	for {
		buf := make([]byte, BUF_SIZE)
		n, err := (*sess.sshConn).Read(buf)
		if err != nil {
			// TODO kill everything... if the ssh connection dies everything should die
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
		packetData, packetErr := proto.Marshal(packet)
		if packetErr != nil {
			return errors.New("Run.")
		}
		sess.sendPacket(packetData)
	}
}
func (sess *Session) addConnAndListen(netconn *net.Conn) {
	sess.lock.Lock()
	defer sess.lock.Unlock()
	conn := &Connection{
		conn: netconn,
	}
	sess.conns = append(sess.conns, conn)
	go connListen(sess, conn)
}

func (sess *Session) onReceiveData(sequenceID uint32, data []byte) {
	fmt.Println("Sending",len(data),"bytes to ssh")
	(*sess.sshConn).Write(data)
}

func connListen(sess *Session, conn *Connection) error {
	for {
		packet, packetErr := readProtoPacket(conn)
		if packetErr != nil {
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
	packetLen := make([]byte, 2)
	_, lenErr := io.ReadFull(*conn.conn, packetLen)
	if lenErr != nil {
		return packet, lenErr
	}
	packetData := make([]byte, binary.BigEndian.Uint16(packetLen))
	_, dataErr := io.ReadFull(conn.conn, packetData)
	if dataErr != nil {
		return packet, dataErr
	}
	unmarshErr := proto.Unmarshal(packetData, &packet)
	return packet, unmarshErr
}
