package main

import (
	"errors"
	mrand "math/rand"
	"sync"
	"time"

	"github.com/howardstark/fusion/protos"
	log "github.com/sirupsen/logrus"
)

const (
	//BUF_SIZE is not determined by how fast or how often SSH sends out data, or how big. The connection to ssh is a localhost socket, which is fast as can be, and is not an issue.
	//It also is not determined by TCP packet size. TCP packet max size is anywhere from 800 to 1400 to 1500 to 9000 to 65535. The OS is very very good and fast at splitting up writes into all the requisite TCP packets.
	//the real slow part is incoming and outgoing, and keeping track of all this data, all the goroutines, outgoing channels, inflight maps, etc. We want to have big chunks. At 10mbit, 64kb chunks result in ~20 per second. That's completely doable. Smaller chunks like the TCP max size on different networks would be really pushing it.
	BUF_SIZE = 65535 - 11 - 1 // encoding with protobuf into the Packet_Data results in 11 bytes added. when BUF_SIZE is 65536, we get packets up to 65547 in length.
	//by limiting the post-wrapping length to less than 65535, we can encode the protobuf packet length into 2 bytes instead of 4.

	RAND_REORDER = false // TODO cli option
)

type OutgoingPacket struct {
	seq     uint32
	data    *[]byte
	sentOn  []*Connection
	date    int64
	session *Session
	lock    sync.Mutex
}

func (sess *Session) sendPacket(out *OutgoingPacket) {
	if sess.redundant {
		sess.sendPacketCustom(out, true) // if we are sending redundant, send to everywhere I can non-blockingly
		return
	}
	sess.sendPacketCustom(out, false) // by default, only send to one non-blocking
}

func (sess *Session) sendPacketCustom(out *OutgoingPacket, multipleNonBlocking bool) {
	if sess.redundant && !multipleNonBlocking { // TODO i mean is it this function's job to catch this? just warn
		log.Error("Possible bug or race condition =DDDD")
		multipleNonBlocking = true
	}
	sess.lock.Lock()
	nsc := len(sess.conns)
	if nsc == 1 {
		c := sess.conns[0]
		out.sentOn = append(out.sentOn, c) // TODO lock
		out.date = time.Now().UnixNano()
		sess.lock.Unlock() // no blocking io in lock
		c.Write(*out.data)
		return
	}
	var avail []*Connection
	if len(out.sentOn) != 0 {
		avail = make([]*Connection, 0, len(sess.conns)) //don't do "len(sess.conns)-len(out.sentOn)" because that could become negative (dupes in sentOn, connections dropping out, etc)

		alreadyUsedMap := make(map[*Connection]bool)
		for i := 0; i < len(out.sentOn); i++ {
			alreadyUsedMap[out.sentOn[i]] = true
		}
		for i := 0; i < len(sess.conns); i++ {
			_, ok := alreadyUsedMap[sess.conns[i]]
			if !ok {
				avail = append(avail, sess.conns[i])
			}
		}
	}
	if len(avail) == 0 {
		avail = sess.conns // if we've already tried them all, try them all again!
	}
	if len(avail) == 0 { // hmm its *still* empty?
		//this doesn't need to return an error
		//if there are no connections, it can just not send
		//it's still in outgoing
		//so when a conn is reestablished it'll be sent
		log.Warning("No open connections. Will send when conn is reestablished.")
		sess.lock.Unlock()
		return
	}
	var connSelection *Connection
	writeBlocking := false
	if len(avail) == 1 {
		connSelection = avail[0]
		//if there is only one connection, just write blocking to it. no point in trying non-blocking, then falling back to blocking if this is the only option.
		writeBlocking = true
	} else {
		rSrc := mrand.New(mrand.NewSource(time.Now().UnixNano()))
		perm := rSrc.Perm(len(avail))
		wrote := false
		connSelection = avail[rSrc.Intn(len(avail))]
		for i := 0; i < len(avail); i++ {
			c := avail[perm[i]]
			ok, _ := c.WriteNonBlocking(*out.data)
			if ok {
				if wrote { // we already did one write, dont just pretend like it didn't happen, broadcast it to the world =DDDDDDDDDD
					log.WithFields(log.Fields{
						"conn":  connSelection.conn,
						"iface": connSelection.iface,
						"addr":  connSelection.conn.RemoteAddr(),
					}).Debug("Write to connection")
				}
				wrote = true
				connSelection = c
				if !multipleNonBlocking {
					break
				}
			}
		}
		if !wrote {
			log.Debug("Failed to write. Picking new interface at random.")
			writeBlocking = true
		}
	}
	log.WithFields(log.Fields{
		"conn":  connSelection.conn,
		"iface": connSelection.iface,
		"addr":  connSelection.conn.RemoteAddr(),
	}).Debug("Write to connection")
	out.sentOn = append(out.sentOn, connSelection) // TODO lock
	out.date = time.Now().UnixNano()

	if writeBlocking {
		log.Debug("No connections were non-blocking. Falling back to random blocking.")
		if len(avail) == 1 {
			sess.lock.Unlock()             // since avail is 1, we can do this blocking write outside blockingSendSelector
			connSelection.Write(*out.data) // haha yes
			// do actual write outside of lock
			return
		}
		//multiple connections available, but none of them willing
		//wait until one of them is

		sess.blockingSendSelector.Lock() // this shouldn't cause deadlock because blockingSendSelector is only used within sendPacketCustom
		sess.lock.Unlock()               // unlock this afterwards so no one else can blocking send with us
		defer sess.blockingSendSelector.Unlock()
		// deadlock is impossible here because no one can wait for blockingsendselector *without already having* the normal lock

		for {
			if len(sess.conns) != nsc { // something has changed
				return
			}
			if uint64(sess.sessionID) == 0 { // session is killed
				return
			}
			rSrc := mrand.New(mrand.NewSource(time.Now().UnixNano())) // we know len(avail) is not 1
			perm := rSrc.Perm(len(avail))
			for i := 0; i < len(avail); i++ {
				ok, _ := avail[perm[i]].WriteNonBlocking(*out.data)
				if ok {
					return
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
	} else {
		sess.lock.Unlock()
	}
}

func (sess *Session) sendStatus(serialized []byte) { // this func is only for sending your status. it starts a ton of goroutines and returns once at least one conn has written.
	log.Debug("Sending out packet to ", len(sess.conns), " destinations")
	sess.lock.Lock() // lock is ok because we are starting goroutines to do the blocking io
	count := len(sess.conns)
	if count == 1 {
		c := sess.conns[0]
		sess.lock.Unlock() // no blocking io in lock
		c.Write(serialized)
		return
	}
	if count == 0 {
		log.Debug("0 destination status, exiting")
		sess.lock.Unlock()
		return
	}
	done := make(chan error)
	for i := 0; i < count; i++ {
		//fmt.Println("Writing")
		c := sess.conns[i]
		go func(conn *Connection, serialized []byte) {
			done <- conn.Write(serialized) // goroutine is fine because order doesn't matter
		}(c, serialized)
	}
	sess.lock.Unlock() // no blocking io in lock
	ind := 0
	for {
		a := <-done
		ind++
		if a == nil { // one of them succeeded yay
			break
		}
		if ind == count {
			log.Warning("Writing on all interfaces failed")
			break
		}
	}
	go func() { // clean up after ourselves
		for i := ind; i < count; i++ {
			<-done
		}
		close(done)
	}()
}

func (sess *Session) listenSSH() {
	contextLog := log.WithFields(log.Fields{
		"sess-id": sess.sessionID,
	})
	defer sess.kill() // guarantee
	for {
		if uint64(sess.sessionID) == 0 {
			contextLog.Warning("ListenSSH dying because session was killed.")
			return
		}
		buf := make([]byte, BUF_SIZE)
		n, err := (*sess.sshConn).Read(buf)
		if err != nil {
			contextLog.WithError(err).Error("SSH read err. Killing session.")
			go sess.kill()
			return
		}
		buf = buf[:n]
		if len(sess.conns) == 0 {
			contextLog.Debug("ListenSSH waiting for connections...")
		} //TODO is it ok to do len(sess.conns) without lock? i think it's fine but not certain... we aren't iterating, reading, or modifying so probably
		for len(sess.conns) == 0 { // no point in reading from ssh if the data has nowhere to go
			time.Sleep(10 * time.Millisecond) // block until nonempty
			if uint64(sess.sessionID) == 0 {
				contextLog.Debug("ListenSSH dying with read data because session was killed.")
				return
			}
		}
		contextLog.Debug("Read ", n, "bytes from ssh")
		if RAND_REORDER {
			randomize(buf, sess)
		} else {
			sess.sendPacket(sess.wrap(buf))
		}
		if n < BUF_SIZE/5 { //TODO /5 and 15* should be consts
			time.Sleep(5 * time.Millisecond) //we only read less than 1/5 of the buffer, give ssh some time to chill
		} //changed my mind this is a bad idea
		//changed my mind this is a good idea
	}
}

func (sess *Session) addConnAndListen(conn *Connection) {
	sess.lock.Lock()
	defer sess.lock.Unlock()
	sess.conns = append(sess.conns, conn)
	go func() {
		defer sess.removeConn(conn)
		err := connListen(sess, conn)
		log.WithError(err).Error("Conn listen err")
	}()
}

func (sess *Session) writeSSH(data []byte) {
	log.Debug("Sending ", len(data), "bytes to ssh")
	n, err := (*sess.sshConn).Write(data)
	if n != len(data) && err == nil {
		panic("whatt") // TODO: Whatt
	}
	if err != nil {
		log.WithField("sess-id", sess.sessionID).WithError(err).Error("SSH write err. Killing session.")
		go sess.kill()
	}
}

func connListen(sess *Session, conn *Connection) error {
	log.Info("Beginning to listen to connection")
	for {
		//fmt.Println("Waiting for packet...")
		conn.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		packet, packetErr, rawPacket := readProtoPacket(conn)
		//fmt.Println("Got packet...")
		if packetErr != nil {
			log.WithError(packetErr).Error("Could not read packet")
			return packetErr
		}

		if dedup(packet, rawPacket) {
			continue
		}
		switch packet.GetBody().(type) { //TODO why on earth are these all go
		case *packets.Packet_Data:
			go sess.onReceiveData(conn, packet.GetData())
		case *packets.Packet_Status:
			go sess.onReceiveStatus(packet.GetStatus())
		case *packets.Packet_Control:
			go sess.onReceiveControl(packet.GetControl())
		default:
			err := errors.New("Unexpected packet type?")
			log.WithFields(log.Fields{
				"packet": packet,
				"body":   packet.GetBody(),
			}).WithError(err).Error("Unexpected packet type")
			return err
		}
	}
	panic("conn listen exited loop without returning err")
}
