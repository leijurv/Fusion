package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"github.com/howardstark/fusion/protos"
)

const (
	BUF_SIZE = 65536
)

type SessionID uint64

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
		sess = makeSession(id)
		sessions[id] = sess
	}
	return sess
}
func newSession() *Session {
	sessionsLock.Lock()
	defer sessionsLock.Unlock()
	ID := SessionID(5021) //generate session ID
	_, ok := sessions[ID]
	if ok {
		//omfg collision??
		fmt.Println("COLLISSIONOSNEUHSEIOT")
		return newSession() //recursion solves everything
	}
	sess := makeSession(ID)
	sessions[ID] = sess
	return sess
}
func makeSession(id SessionID) *Session {
	return &Session{
		sessionID: id, //this is the only field to init
		//dont make sshconn, we dont know if were server or client
	}
}

func ServerReceivedClientConnection(conn *net.Conn) error {
	var id int64
	err := binary.Read(*conn, binary.LittleEndian, &id)
	if err != nil {
		return err
	}
	sess := getSession(SessionID(id))
	sess.lock.Lock()
	defer sess.lock.Unlock()
	if sess.sshConn == nil {
		conn, err := net.Dial("tcp", "localhost:22")
		if err != nil {
			return err
		}
		sess.sshConn = &conn
		go sess.listenSSH()
	}
	return sess.addConnAndListen(conn)
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
		return errors.New("we dont have a ssh connection for this session id what are you even doing bro lol")
	}
	return sess.addConnAndListen(conn)
}

func ClientReceivedSSHConnection(ssh *net.Conn, serverAddr string) error {
	sess := newSession()
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
	buf := make([]byte, BUF_SIZE)
	for {
		n, err := (*sess.sshConn).Read(buf)
		if err != nil {
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
		_ = packet.Body
		serialized := []byte("this is the packet")
		sess.sendPacket(serialized)
	}
}
func (sess *Session) addConnAndListen(netconn *net.Conn) error {
	sess.lock.Lock()
	defer sess.lock.Unlock()
	conn := &Connection{
		conn: netconn,
	}
	sess.conns = append(sess.conns, conn)
	go connListen(sess, conn)
	return nil
}
func connListen(sess *Session, conn *Connection) error {
	return nil
}
