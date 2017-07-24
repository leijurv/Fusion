package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/howardstark/fusion/protos"
)

type SessionID uint64

type Session struct {
	lock        sync.Mutex // todo rwmutex
	sessionID   SessionID
	sshConn     *net.Conn
	conns       []*Connection
	outgoingSeq uint32
	//
	incomingLock       sync.Mutex // this lock is for inflight, incomingSeq, and highestReceivedSeq
	incomingSeq        uint32
	inflight           map[uint32]*[]byte
	highestReceivedSeq uint32
	//
	outgoingLock sync.Mutex // this lock is ONLY for the outgoing map
	outgoing     map[uint32]*[]byte
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
			outgoing:  make(map[uint32]*[]byte),
		}
		sessions[id] = sess
		go sess.timer()
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
		inflight:  make(map[uint32]*[]byte),
		outgoing:  make(map[uint32]*[]byte),
	}
	sessions[ID] = sess
	go sess.timer()
	return sess
}
func (sess *Session) getOutgoingSeq() uint32 {
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
	sess.sessionID = SessionID(0) // kill timer
}
func (sess *Session) timer() {
	id := sess.sessionID
	ticksWithoutConns := 0
	for uint64(sess.sessionID) != 0 {
		time.Sleep(1 * time.Second)
		sess.tick()
		fmt.Println("Ticking with", len(sess.conns), "conns and tick count", ticksWithoutConns)
		if len(sess.conns) == 0 {
			ticksWithoutConns++
		} else {
			ticksWithoutConns = 0
		}
		if ticksWithoutConns > 60 {
			fmt.Println("Killing session", sess.sessionID, "because", ticksWithoutConns, "ticks without any connections")
			sess.kill()
		}
	}
	fmt.Println("Timer exiting for", id)
}
func (sess *Session) tick() {
	sess.lock.Lock()
	defer sess.lock.Unlock()
	sess.incomingLock.Lock()
	defer sess.incomingLock.Unlock()
	timestamp := time.Now().UnixNano()
	keys := make([]uint32, len(sess.inflight))
	i := 0
	for k := range sess.inflight {
		keys[i] = k
		i++
	}
	fmt.Println("Sending tick", timestamp, keys, sess.incomingSeq)
	data := marshal(&packets.Packet{Body: &packets.Packet_Status{Status: &packets.Status{Timestamp: timestamp, IncomingSeq: sess.incomingSeq, Inflight: keys}}})
	go sess.sendOnAll(data)
}
func (sess *Session) removeConn(conn *Connection) {
	sess.lock.Lock()
	defer sess.lock.Unlock()
	for i := 0; i < len(sess.conns); i++ {
		if sess.conns[i] == conn {
			sess.conns = append(sess.conns[:i], sess.conns[i+1:]...)
			fmt.Println("Sucesssfully removed connection index", i)
			return
		}
	}
	fmt.Println(conn, "not present in", sess.conns)
	panic("conn could not be removed")
}
func (sess *Session) checkInflight() {
	for {
		data, ok := sess.inflight[sess.incomingSeq]
		if !ok {
			fmt.Println("Still waiting on", sess.incomingSeq)
			return
		}
		sess.writeSSH(*data)
		delete(sess.inflight, sess.incomingSeq)
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
func (sess *Session) onReceiveStatus(incomingSeq uint32, timestamp int64, inflight []uint32) {
	fmt.Println("Received status", incomingSeq, timestamp, inflight)
	sess.outgoingLock.Lock()
	defer sess.outgoingLock.Unlock()
	keys := make([]uint32, len(sess.outgoing)) // make a copy of the keys beacuse we're gonna modify the map
	i := 0
	for k := range sess.outgoing {
		keys[i] = k
		i++
	}
	fmt.Println("Current outgoing keys:", keys)
	//only do a prune once we receive a status because that's the only time we get new info that lets us prune
	for i = 0; i < len(keys); i++ {
		if keys[i] < incomingSeq {
			//remove everything in sess.outgoing with key less than incomingSeq
			//because they've just told us that they've received and processed everything < incomingSeq.
			delete(sess.outgoing, keys[i])
		}
	}

	for j := 0; j < len(inflight); j++ { //also remove everything in inflight from sess.outgoing, because that's what they have received but not processed
		_, ok := sess.outgoing[inflight[j]]
		if ok {
			fmt.Println("Inflight prune", inflight[j])
		}
		delete(sess.outgoing, inflight[j])
	}

	//MAYBE resend the gaps of inflight
}