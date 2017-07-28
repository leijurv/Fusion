package main

import (
	"errors"
	"fmt"
	mrand "math/rand"
	"time"

	"github.com/howardstark/fusion/protos"
)

const (
	//BUF_SIZE is not determined by how fast or how often SSH sends out data, or how big. The connection to ssh is a localhost socket, which is fast as can be, and is not an issue.
	//It also is not determined by TCP packet size. TCP packet max size is anywhere from 800 to 1400 to 1500 to 9000 to 65535. The OS is very very good and fast at splitting up writes into all the requisite TCP packets
	//the real slow part is incoming and outgoing, and keeping track of all this data, all the goroutines and outgoing channels, etc. We want to have big chunks. At 10mbit, 64kb chunks result in ~20 per second. That's completely doable. Smaller chunks like the TCP max size on different networks would be really pushing it
	BUF_SIZE = 65535 - 11 - 1 // encoding with protobuf into the Packet_Data results in 11 bytes added. when BUF_SIZE is 65536, we get packets up to 65547 in length.
	//by limiting the post-wrapping length to less than 65535, we can encode the protobuf packet length into 2 bytes instead of 4.

	RAND_REORDER = false // TODO cli option
)

func (sess *Session) sendPacket(out *OutgoingPacket) {
	if sess.redundant {
		fmt.Println("SENDING REDUNDANT")
		sess.sendOnAll(*out.data)
		out.date = time.Now().UnixNano()
		return
	}
	sess.lock.Lock()
	if len(sess.conns) == 1 {
		c := sess.conns[0]
		out.sentOn = append(out.sentOn, c) // TODO lock
		out.date = time.Now().UnixNano()
		sess.lock.Unlock() // no blocking io in lock
		c.Write(*out.data)
		return
	}
	//TODO add optimizations like:
	//if sentOn is empty, just pick a connection at random
	//if there is 1 connection, don't use the random number generator, just go ahead and send it
	avail := make([]*Connection, 0)

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
	if len(avail) == 0 {
		if len(sess.conns) == 0 {
			//this doesn't need to return an error
			//if there are no connections, it can just not send
			//it's still in outgoing
			//so when a conn is reestablished it'll be sent
			fmt.Println("OH NO UH IDK WHAT TO DO")
			return
		}
		avail = sess.conns // if we've already tried them all, try them all again!
	}
	rSrc := mrand.New(mrand.NewSource(time.Now().UnixNano()))
	perm := rSrc.Perm(len(avail))
	wrote := false
	connSelection := avail[rSrc.Intn(len(avail))]
	for i := 0; i < len(avail); i++ {
		c := avail[perm[i]]
		ok, _ := c.WriteNonBlocking(*out.data)
		if ok {
			wrote = true
			connSelection = c
			break
		}
	}
	if !wrote {
		fmt.Println("Failed, picking at random")
	}
	fmt.Println("Selected", wrote, "conn", connSelection.conn.LocalAddr(), connSelection.conn.RemoteAddr())
	out.sentOn = append(out.sentOn, connSelection) // TODO lock
	out.date = time.Now().UnixNano()
	sess.lock.Unlock() // no blocking io in lock

	if !wrote {
		fmt.Println("No connections were non blocking, falling back to random blocking")
		connSelection.Write(*out.data) // haha yes
		// do actual write outside of lock
	}
}
func (sess *Session) sendOnAll(serialized []byte) {
	fmt.Println("Sending out packet to", len(sess.conns), "destinations")
	sess.lock.Lock() // lock is ok because we are starting goroutines to do the blocking io
	count := len(sess.conns)
	if count == 1 {
		c := sess.conns[0]
		sess.lock.Unlock() // no blocking io in lock
		c.Write(serialized)
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
			fmt.Println("OH NO THEY ALL FAILED")
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

func (sess *Session) listenSSH() error {
	defer sess.kill() // guarantee
	for {
		if uint64(sess.sessionID) == 0 {
			fmt.Println("listenssh dying because session killed")
			return nil
		}
		buf := make([]byte, BUF_SIZE)
		n, err := (*sess.sshConn).Read(buf)
		if err != nil {
			fmt.Println("SSH read err, killing session", sess.sessionID, err)
			go sess.kill()
			return err
		}
		buf = buf[:n]
		if len(sess.conns) == 0 {
			fmt.Println("listenssh waiting for connections...  ", sess.sessionID)
		} //TODO is it ok to do len(sess.conns) without lock? i think it's fine but not certain... we aren't iterating, reading, or modifying so probably
		for len(sess.conns) == 0 { // no point in reading from ssh if the data has nowhere to go
			time.Sleep(10 * time.Millisecond) // block until nonempty
			if uint64(sess.sessionID) == 0 {
				fmt.Println("listenssh dying because session killed (waiting subloop)")
				return nil
			}
		}
		fmt.Println("Read", n, "bytes from ssh")
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
		fmt.Println("conn listen err", err)
	}()
}

func (sess *Session) writeSSH(data []byte) {
	fmt.Println("Sending", len(data), "bytes to ssh")
	n, err := (*sess.sshConn).Write(data)
	if n != len(data) && err == nil {
		panic("whatt")
	}
	if err != nil {
		fmt.Println("SSH write err", err, "killing session", sess.sessionID)
		go sess.kill()
	}
}

func connListen(sess *Session, conn *Connection) error {
	fmt.Println("Beginning conn listen")
	for {
		//fmt.Println("Waiting for packet...")
		conn.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		packet, packetErr, rawPacket := readProtoPacket(conn)
		//fmt.Println("Got packet...")
		if packetErr != nil {
			fmt.Println("Read err", packetErr)
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
		case *packets.Packet_Init:
			err := errors.New("Init packet after init")
			fmt.Println(err, packet, packet.GetBody())
			return err
		default:
			err := errors.New("Unknown packet type?")
			fmt.Println(err, packet, packet.GetBody())
			return err
		}
	}
	panic("conn listen exited loop without returning err")
}
