package main
import (
	"net"
	"sync"
)

const(
	BUF_SIZE = 65536
)

type SessionID uint64

struct Connection{
	conn *net.Conn
	lastSuccessfulDataTransfer uint64//idk more fields here
}

struct Session{
	sessionID SessionID
	sshConn *net.Conn
	conns []Connection
	lock sync.Mutex
}

var sessions map[SessionID]*Session
var sessionsLock sync.Mutex
func getSession(id SessionID) *Session{
	sessionsLock.Lock()
	defer sessionsLock.Unlock()
	sess, ok := sessions[id]
	if !ok || sess==nil{
		sess = makeSession(id)
		sessions[id]=sess
	}
	return sess
}
func makeSession(id SessionID) *Session{
	return &Session{
		sessionID: id,
		//etc
	}
}
func (sess *Session) listenSSH() error{
	buf := make([]byte, BUF_SIZE)
	for{
		n, err:=sess.sshConn.read(buf)
		if err!=nil{
			return err
		}
		//make protobuf packet
		//pick from sess.conns
		//send
	}
}