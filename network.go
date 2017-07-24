package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	mrand "math/rand"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/howardstark/fusion/protos"
)

const (
	BUF_SIZE     = 65536
	RAND_REORDER = true
)

type SessionID uint64

type Connection struct {
	conn                       net.Conn
	lastSuccessfulDataTransfer uint64 //idk more fields here
}

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
var packetDedupLock sync.Mutex
var (
	packetDedup = make(map[[32]byte]bool)
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
		// TODO: Check for collisions since they are much more likely given the nature of our random number generator
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

func ServerReceivedClientConnection(conn net.Conn) error {
	var id uint64
	err := binary.Read(conn, binary.LittleEndian, &id)
	if err != nil {
		return err
	}
	fmt.Println("Server received connection", conn, " for session id", id)
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
	go sess.addConnAndListen(conn)
	return nil
}

func ClientCreateServerConnection(conn net.Conn, id SessionID) error {
	err := binary.Write(conn, binary.LittleEndian, uint64(id))
	if err != nil {
		return err
	}
	var verify uint64
	err = binary.Read(conn, binary.BigEndian, &verify) // verifies that proper 2-way communication is happening before adding to list of conns
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
	fmt.Println("Client creating new server conn for session id", id, "and", conn)
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
func (sess *Session) sendPacket(serialized []byte) {
	sess.lock.Lock()
	defer sess.lock.Unlock()
	ind := mrand.New(mrand.NewSource(time.Now().UnixNano())).Intn(len(sess.conns))
	fmt.Println("Selected conn index", ind)
	connSelection := sess.conns[ind].conn // do this step in lock
	go connSelection.Write(serialized)    // haha yes
	// do actual write outside of lock
}
func (sess *Session) sendOnAll(serialized []byte) {
	fmt.Println("Sending out packet", len(sess.conns))
	sess.lock.Lock() // lock is ok because we are starting goroutines to do the blocking io
	defer sess.lock.Unlock()
	for i := 0; i < len(sess.conns); i++ {
		fmt.Println("Writing")
		go sess.conns[i].conn.Write(serialized) // goroutine is fine because order doesn't matter
	}
}

func (sess *Session) listenSSH() error {
	for {
		for len(sess.conns) == 0 { // no point in reading from ssh if the data has nowhere to go
			time.Sleep(10 * time.Millisecond) // block until nonempty
			if uint64(sess.sessionID) == 0 {
				fmt.Println("listenssh dying because session killed")
				return nil
			}
		}
		buf := make([]byte, BUF_SIZE)
		n, err := (*sess.sshConn).Read(buf)
		if err != nil {
			fmt.Println("SSH read err, killing session", sess.sessionID, err)
			go sess.kill()
			return err
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

			rSrc := mrand.New(mrand.NewSource(time.Now().UnixNano()))
			perm := rSrc.Perm(len(packets))
			for i := 0; i < len(perm); i++ {
				sess.sendPacket(packets[perm[i]])
			}
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
	marshalled := marshal(&packet)
	sess.outgoingLock.Lock()
	defer sess.outgoingLock.Unlock()
	sess.outgoing[seq] = &marshalled
	return marshalled
}
func marshal(packet *packets.Packet) []byte {
	packetData, packetErr := proto.Marshal(packet)
	fmt.Println("Marshal")
	if packetErr != nil {
		fmt.Println("Marshal error", packetErr)
		return nil
	}
	if len(packetData) >= 4294967296 {
		fmt.Println(errors.New("Packet was too big"))
		return nil
	}
	packetLen := make([]byte, 4)
	binary.LittleEndian.PutUint32(packetLen, uint32(len(packetData)))
	packetData = append(packetLen, packetData...)
	fmt.Println("Done marshal")
	return packetData
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
func (sess *Session) addConnAndListen(netconn net.Conn) {
	sess.lock.Lock()
	defer sess.lock.Unlock()
	conn := &Connection{
		conn: netconn,
	}
	sess.conns = append(sess.conns, conn)
	go func() {
		err := connListen(sess, conn)
		if err != nil {
			fmt.Println("conn listen err", err)
			sess.removeConn(conn)
		}
	}()
}
func (sess *Session) removeConn(conn *Connection) {
	sess.lock.Lock()
	defer sess.lock.Unlock()
	for i := 0; i < len(sess.conns); i++ {
		if sess.conns[i] == conn {
			sess.conns = append(sess.conns[:i], sess.conns[i+1:]...)
			fmt.Println("Sucesssfully removed index", i)
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
func (sess *Session) writeSSH(data []byte) {
	fmt.Println("Sending", len(data), "bytes to ssh")
	(*sess.sshConn).Write(data)
}
func (sess *Session) onReceiveStatus(incomingSeq uint32, timestamp int64, inflight []uint32) {
	fmt.Println("Received status", incomingSeq, timestamp, inflight)
	//remove everything in sess.outgoing with key less than incomingSeq
	//because they've just told us that they've received and processed everything < incomingSeq, and they now expect incomingSeq next

	//also remove everything in inflight from sess.outgoing, because that's what they have received but not processed

	//MAYBE resend the gaps of inflight
}
func shouldDedup(packet packets.Packet) bool {
	switch packet.GetBody().(type) {
	case *packets.Packet_Data:
		return false
	case *packets.Packet_Status:
		return true
	}
	panic("what is this packet")
}
func dedup(packet packets.Packet, rawPacket []byte) bool {
	if !shouldDedup(packet) {
		return false
	}
	packetDedupLock.Lock()
	defer packetDedupLock.Unlock()
	hash := sha256.Sum256(rawPacket)
	_, ok := packetDedup[hash]
	if ok {
		fmt.Println("Ignoring already received packet with hash", hash)
		return true
	}
	packetDedup[hash] = true
	return false
}
func connListen(sess *Session, conn *Connection) error {
	fmt.Println("Beginning conn listen")
	for {
		fmt.Println("Waiting for packet...")
		packet, packetErr, rawPacket := readProtoPacket(conn)
		fmt.Println("Got packet...")
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
		}
	}
	panic("conn listen exited loop without returning err")
}

func readProtoPacket(conn *Connection) (packets.Packet, error, []byte) {
	var packet packets.Packet
	packetLen := make([]byte, 4)
	_, lenErr := io.ReadFull(conn.conn, packetLen)
	if lenErr != nil {
		return packet, lenErr, nil
	}
	l := binary.LittleEndian.Uint32(packetLen)
	fmt.Println("Reading packet of length", l)
	packetData := make([]byte, l)
	_, dataErr := io.ReadFull(conn.conn, packetData)
	if dataErr != nil {
		return packet, dataErr, packetData
	}
	unmarshErr := proto.Unmarshal(packetData, &packet)
	return packet, unmarshErr, packetData
}
