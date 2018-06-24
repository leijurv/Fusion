package main

import (
	"crypto/rand"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"time"

	"github.com/howardstark/fusion/protos"
	log "github.com/sirupsen/logrus"
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
	outgoing     map[uint32]*OutgoingPacket
	//
	blockingSendSelector sync.Mutex
	//
	redundant bool
}

var (
	sessionsLock sync.Mutex
	sessions     = make(map[SessionID]*Session)
)

func NewSessionID() SessionID {
	b := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		panic("unable to generate random session id")
	}
	return SessionID(binary.BigEndian.Uint64(b))
}

func hasSession(id SessionID) bool {
	sessionsLock.Lock()
	defer sessionsLock.Unlock()
	_, ok := sessions[id]
	return ok
}

func getSession(id SessionID) *Session {
	sessionsLock.Lock()
	defer sessionsLock.Unlock()
	sess, ok := sessions[id]
	if !ok || sess == nil {
		sess = secretUncheckedMakeSession(id)
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
		log.WithField("sess-id", ID).Panic("Session id collision detected")
		//return newSession() //recursion solves everything
	}
	return secretUncheckedMakeSession(ID)
}

func secretUncheckedMakeSession(id SessionID) *Session {
	sess := &Session{
		sessionID: id,
		inflight:  make(map[uint32]*[]byte),
		outgoing:  make(map[uint32]*OutgoingPacket),
	}
	sessions[id] = sess
	go sess.timer()
	return sess
}

func (sess *Session) incrementOutgoingSeq() uint32 {
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
		sess.conns[i].Close()
	}
	sessionsLock.Lock()
	defer sessionsLock.Unlock()
	delete(sessions, sess.sessionID)
	sess.sessionID = SessionID(0) // setting session id to zero marks as killed (see next function), and kills timer, etc
}
func (sess *Session) isKilled() bool {
	return uint64(sess.sessionID) == 0
}

func (sess *Session) timer() {
	defer sess.kill() // guarantee
	id := sess.sessionID
	ticksWithoutConns := 0
	for !sess.isKilled() {
		time.Sleep(1 * time.Second)
		sess.tick()
		log.WithFields(log.Fields{
			"active-conns": len(sess.conns),
			"tick-count":   ticksWithoutConns,
		}).Debug("Ticking")
		if len(sess.conns) == 0 {
			ticksWithoutConns++
		} else {
			ticksWithoutConns = 0
		}
		if ticksWithoutConns > 60 {
			log.WithFields(log.Fields{
				"sess-id":    sess.sessionID,
				"tick-count": ticksWithoutConns,
			}).Info("Killing session. No conns.")
			sess.kill()
		}
	}
	log.WithField("sess-id", id).Debug("Timer exiting")
}

func (sess *Session) tick() {
	data := marshal(sess.StatusPacket())
	sess.sendStatus(data) // not in new goroutine, should block.
}

func (sess *Session) StatusPacket() *packets.Packet {
	sess.lock.Lock() // TODO do we need this lock or is just incomingLock sufficient
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
	log.WithFields(log.Fields{
		"timestamp": timestamp,
		"keys":      keys,
		"inc-seq":   sess.incomingSeq,
	}).Debug("Sending tick")

	return &packets.Packet{
		Body: &packets.Packet_Status{
			Status: &packets.Status{
				Timestamp:   timestamp,
				IncomingSeq: sess.incomingSeq,
				Inflight:    keys,
			},
		},
	}
}

func (sess *Session) removeConn(conn *Connection) {
	sess.lock.Lock()
	defer sess.lock.Unlock()
	for i := 0; i < len(sess.conns); i++ {
		if sess.conns[i] == conn {
			sess.conns = append(sess.conns[:i], sess.conns[i+1:]...)
			log.Debug("Successfully removed connection index ", i)
			return
		}
	}
	log.Debug(conn, " not present in ", sess.conns)
	panic("conn could not be removed")
}

func (sess *Session) checkInflight() { // *sheds tear* it's... beautiful
	for {
		data, ok := sess.inflight[sess.incomingSeq]
		if !ok {
			return
		}
		sess.writeSSH(*data)
		delete(sess.inflight, sess.incomingSeq)
		sess.incomingSeq++
	}
}

func (sess *Session) onReceiveData(from *Connection, packet *packets.Data) {
	sequenceID := packet.GetSequenceID()
	data := packet.GetContent()
	sess.incomingLock.Lock()
	defer sess.incomingLock.Unlock()
	if sequenceID > sess.highestReceivedSeq {
		sess.highestReceivedSeq = sequenceID
	}
	if sess.incomingSeq == sequenceID { // woohoo everything is arriving in the right order yay
		log.WithField("futureSeq", sess.incomingSeq+1).Debug("Sequence is in order. 👍")
		sess.writeSSH(data)
		sess.incomingSeq++
		sess.checkInflight()
		return
	}
	if sess.incomingSeq > sequenceID {
		log.WithFields(log.Fields{
			"expecting": sess.incomingSeq,
			"received":  sequenceID,
			"from":      from.LocalAddr(),
		}).Debug("Received dupe.")
		return
	}
	log.WithFields(log.Fields{
		"expecting": sess.incomingSeq,
		"received":  sequenceID,
		"from":      from.LocalAddr(),
	}).Debug("Out of order.")
	sess.inflight[sequenceID] = &data
	sess.checkInflight() // technically this call should never result in anything happening...? if sess.incomingSeq < sequenceID then sess.inflight[sess.incomingSeq] will always be nil...
}

func anyActive(out *OutgoingPacket, sess *Session) bool {
	out.lock.Lock()
	defer out.lock.Unlock()
	sess.lock.Lock()
	defer sess.lock.Unlock()
	for _, conn := range out.sentOn {
		if !func() bool {
			for i := 0; i < len(sess.conns); i++ {
				if sess.conns[i] == conn {
					return false
				}
			}
			return true
		}() {
			return true
		}
	}
	return false
}

func (sess *Session) onReceiveControl(packet *packets.Control) {
	sess.redundant = packet.GetRedundant()
}

func (sess *Session) onReceiveStatus(packet *packets.Status) {
	incomingSeq := packet.GetIncomingSeq()
	timestamp := packet.GetTimestamp()
	inflight := packet.GetInflight()

	log.WithFields(log.Fields{
		"incomingSeq": incomingSeq,
		"timestamp":   timestamp,
		"inflight":    inflight,
	}).Debug("Received status")

	maxReceived := uint32(0) // i would make this -1 but it's a uint lol
	foundAny := false
	inflightMap := make(map[uint32]bool)
	for i := 0; i < len(inflight); i++ {
		inflightMap[inflight[i]] = true
		if inflight[i] > maxReceived {
			maxReceived = inflight[i]
			foundAny = true
		}
	}

	sess.outgoingLock.Lock()
	defer sess.outgoingLock.Unlock()

	keys := make([]uint32, len(sess.outgoing)) // make a copy of the keys beacuse we're gonna modify the map
	index := 0
	for k := range sess.outgoing {
		keys[index] = k
		index++
	}

	log.WithField("keys", keys).Debug("Current outgoing keys")

	//only do a prune once we receive a status because that's the only time we get new info that lets us prune
	for i := 0; i < len(keys); i++ {
		if keys[i] < incomingSeq {
			//remove everything in sess.outgoing with key less than incomingSeq
			//because they've just told us that they've received and processed everything < incomingSeq.
			delete(sess.outgoing, keys[i])
		}
	}

	for j := 0; j < len(inflight); j++ { //also remove everything in inflight from sess.outgoing, because that's what they have received but not processed
		_, ok := sess.outgoing[inflight[j]]
		if ok {
			log.Debug("Inflight prune ", inflight[j])
			delete(sess.outgoing, inflight[j])
		}
	}
	if !foundAny { // maxReceived isn't really 0, they actually haven't even received packed 0, so can't run any of this just yet
		return
	}
	for seq := maxReceived; seq >= incomingSeq; seq-- {
		_, ok := inflightMap[seq] // this isn't sess.inflight its a different map; don't need that lock
		if ok {
			continue
		}
		//seq is a gap in what they have received
		//this means one connection is going faster than another
		outPacket, ok := sess.outgoing[seq]
		if !ok || outPacket == nil {
			log.WithFields(log.Fields{
				"seq":     seq,
				"sess-id": sess.sessionID,
			}).Error("Attempting to fetch already-pruned packet, you slimy bandit. Kicking.")
			go sess.kill()
			return
		}

		diff := time.Now().UnixNano() - outPacket.date
		//outPacket.lock.Lock()
		stillActive := anyActive(outPacket, sess)
		//stillActive = false means it almost certainly isn't still in transit; the connection is just closed
		log.WithFields(log.Fields{
			"time-diff": diff / 1000000,
			"seq":       seq,
			"active":    stillActive,
		}).Debug("Time diff (ms)")
		if !stillActive {
			//this will only ever resend if there's an existing connection that it hasn't been sent on yet
			//there would be no point otherwise, because of tcp ordering
			//therefore, this should never cause sendPacketCustom to resort to resetting conn array...
			outPacket.date = time.Now().UnixNano() // wait a bit before doing this again
			sess.sendPacketCustom(outPacket, true) // send it over every non-blocking conn we can. and if that doesn't work, over one blocking one
			//the ", true" will only have an effect if there are 3 connections or more by the way. since it's already failed on one conn...
			//TODO is it ok for status packet processing to block on resending a packet? methinks no... we ARE holding outgoingLock...
			//and yet, holding outgoing lock will prevent anything else from sending anything
			//delicious.
		}
		//outPacket.lock.Unlock() // ok to do this in the lock; resending due to a dropped connection happens once per packet per dropped connection
		// something like resending if no acknowledgement after 20 secs would not go in the lock because that'll mess everything up

	}
}
