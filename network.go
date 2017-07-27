package main

import (
	"errors"
	"fmt"
	mrand "math/rand"
	"time"

	"github.com/howardstark/fusion/protos"
)

const (
	BUF_SIZE     = 65536
	RAND_REORDER = false
)

func (sess *Session) sendPacket(sent *Sent) {
	if sess.redundant {
		fmt.Println("SENDING REDUNDANT")
		sess.sendOnAll(*sent.data)
		sent.date = time.Now().UnixNano()
		return
	}
	sess.lock.Lock()
	//TODO add optimizations like:
	//if sentOn is empty, just pick a connection at random
	//if there is 1 connection, don't use the random number generator, just go ahead and send it
	available := make([]Connection, 0)

	alreadyUsedMap := make(map[Connection]bool)
	for i := 0; i < len(sent.sentOn); i++ {
		alreadyUsedMap[sent.sentOn[i]] = true
	}

	for i := 0; i < len(sess.conns); i++ {
		_, ok := alreadyUsedMap[sess.conns[i]]
		if !ok {
			available = append(available, sess.conns[i])
		}
	}
	var connSelection Connection
	if len(available) == 0 {
		if len(sess.conns) == 0 {
			//this doesn't need to return an error
			//if there are no connections, it can just not send
			//it's still in outgoing
			//so when a conn is reestablished it'll be sent
			fmt.Println("OH NO UH IDK WHAT TO DO")
			return
		}
		ind := mrand.New(mrand.NewSource(time.Now().UnixNano())).Intn(len(sess.conns))
		_, ok := sess.conns[ind].(*TcpConnection)
		if ok {
			fmt.Println("", ind, sess.conns[ind].(*TcpConnection).conn.LocalAddr(), sess.conns[ind].(*TcpConnection).conn.RemoteAddr())
		} else {
			fmt.Println("sel not tcp")
		}
		connSelection = sess.conns[ind] // do this step in lock
	} else {
		ind := mrand.New(mrand.NewSource(time.Now().UnixNano())).Intn(len(available))
		_, ok := sess.conns[ind].(*TcpConnection)
		if ok {
			fmt.Println("Selected conn", available[ind].(*TcpConnection).conn.LocalAddr(), available[ind].(*TcpConnection).conn.RemoteAddr())
		} else {
			fmt.Println("selected not tcp")
		}
		connSelection = available[ind] // do this step in lock
	}

	sent.sentOn = append(sent.sentOn, connSelection) // TODO lock
	sent.date = time.Now().UnixNano()
	sess.lock.Unlock()

	connSelection.Write(*sent.data) // haha yes
	// do actual write outside of lock
}
func (sess *Session) sendOnAll(serialized []byte) {
	fmt.Println("Sending out packet to", len(sess.conns), "destinations")
	sess.lock.Lock() // lock is ok because we are starting goroutines to do the blocking io
	defer sess.lock.Unlock()
	for i := 0; i < len(sess.conns); i++ {
		//fmt.Println("Writing")
		go sess.conns[i].Write(serialized) // goroutine is fine because order doesn't matter
	}
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
		}
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
			time.Sleep(15 * time.Millisecond) //we only read less than 1/5 of the buffer, give ssh some time to chill
		}
	}
}

func (sess *Session) addConnAndListen(conn Connection) {
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

func connListen(sess *Session, conn Connection) error {
	fmt.Println("Beginning conn listen")
	for {
		//fmt.Println("Waiting for packet...")
		_, ok := conn.(*TcpConnection)
		if ok {
			conn.(*TcpConnection).conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		} else {
			for i := 0; i < 10; i++ {
				fmt.Println("\n\nTHIS ISNT A TCP CONNECTION PLEASE IMPLEMENT READ DEADLINES ITS net.Conn.SetReadDeadline PLSTHX\n\n")
			}
		}
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
			go sess.onReceiveData(conn, packet.GetData().GetSequenceID(), packet.GetData().GetContent())
		case *packets.Packet_Status:
			go sess.onReceiveStatus(packet.GetStatus().GetIncomingSeq(), packet.GetStatus().GetTimestamp(), packet.GetStatus().GetInflight())
		case *packets.Packet_Control:
			go sess.onReceiveControl(packet.GetControl().GetTimestamp(), packet.GetControl().GetRedundant())
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
