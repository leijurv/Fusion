package main

import (
	"net"
	"sync"
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
	sessionID SessionID
	sshConn   *net.Conn
	conns     []Connection
	lock      sync.Mutex
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
func makeSession(id SessionID) *Session {
	return &Session{
		sessionID: id, //this is the only field to init
		//dont make sshconn, we dont know if were server or client
	}
}

func ServerReceivedClientConnection(conn *net.Conn) { //dont pass in ID, we'll read it
	//read ID
	//get session
	//set ssh conn to a new one to localhost:22 if it's nil
	//add this connection
	//start listening
}

func ClientCreateServerConnection(conn *net.Conn, id SessionID) {
	//conn.writeUint64(id)
	//
}
func (sess *Session) listenSSH() error {
	buf := make([]byte, BUF_SIZE)
	for {
		n, err := sess.sshConn.read(buf)
		if err != nil {
			return err
		}
		//make protobuf packet
		//pick from sess.conns
		//send
	}
}
func (conn *Connection) listen() error {
	return nil
}
