package main

import (
	"errors"
	"net"
	"time"

	"github.com/howardstark/fusion/protos"
	log "github.com/sirupsen/logrus"
)

func ServerReceivedClientConnection(conn net.Conn) error {
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	tcp := &Connection{conn: conn}
	packet, packetErr, _ := readProtoPacket(tcp)
	if packetErr != nil {
		log.Error("Could not read proto packet... ", packetErr)
		return packetErr
	}
	_, ok := packet.GetBody().(*packets.Packet_Init)
	if !ok {
		log.Error("Expected init packet, instead received: ", packet.GetBody())
		return errors.New("Expected init packet")
	}
	packetInit := packet.GetInit()
	if packetInit == nil {
		panic("protobuf guarantees this won't happen?")
	}
	id := SessionID(packetInit.GetSession())
	inter := packetInit.GetInterface()
	tcp.ifaceID = inter
	log.WithFields(log.Fields{
		"conn":  conn,
		"id":    id,
		"addr":  conn.RemoteAddr(),
		"iface": packetInit.GetInterface(),
	}).Debug("Server received connection")
	// TODO SetWriteDeadline for confirm?
	err := tcp.Write(marshal(&packets.Packet{ // use Connection.Write instead of net.Conn.Write to prime up the writeloop goroutine, start channels etc
		Body: &packets.Packet_Confirm{
			Confirm: &packets.Confirm{
				Session:   uint64(id),
				Interface: inter,
			},
		},
	}))
	if err != nil {
		// session hsan't been created yet; nothing to clean up
		// if we're unable to write the confirm, can just cleanly exit and pass up the requisite error
		return err
	}
	sess := getSession(SessionID(id))
	sess.onReceiveControl(packetInit.GetControl()) // not just for the first session. this allows client to change settings after initial.
	sess.lock.Lock()
	defer sess.lock.Unlock()
	if sess.sshConn == nil {
		log.WithField("id", id).Debug("Server is initializing new ssh conn")
		sshConn, err := net.Dial("tcp", flagServerDestination)
		if err != nil {
			log.WithError(err).Error("Localhost dial error")
			return err
		}
		sess.sshConn = &sshConn
		go sess.listenSSH()
	}
	log.Debug("Adding connection")
	go sess.addConnAndListen(tcp)
	return nil
}

func ClientCreateServerConnection(conn *Connection, id SessionID) error {
	conn.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	err := conn.Write(marshal(&packets.Packet{
		Body: &packets.Packet_Init{
			Init: &packets.Init{
				Session:   uint64(id),
				Interface: conn.ifaceID,
				Bandwidth: uint32(flagInterfaces.contents[conn.iface]),
				Control: &packets.Control{
					Timestamp: time.Now().UnixNano(),
					Redundant: flagRedundant || flagRedundantDownload,
				},
			},
		},
	}))
	if err != nil {
		return err
	}
	packet, packetErr, _ := readProtoPacket(conn)
	if packetErr != nil {
		log.WithError(packetErr).Error("Could not read proto packet...")
		return packetErr
	}
	_, ok := packet.GetBody().(*packets.Packet_Confirm)
	if !ok {
		log.Error("Expected confirm packet, instead received: ", packet.GetBody())
		return errors.New("Expected confirm packet")
	}
	if packet.GetConfirm().GetSession() != uint64(id) || packet.GetConfirm().GetInterface() != conn.ifaceID {
		err = errors.New("ID response mismatch")
		log.WithFields(log.Fields{
			"sess":     packet.GetConfirm().GetSession(),
			"id":       id,
			"iface":    packet.GetConfirm().GetInterface(),
			"iface-id": conn.ifaceID,
		}).Error(err)
		return err
	}
	//fmt.Println("ITS WORKING OKAY??")
	sessionsLock.Lock()
	defer sessionsLock.Unlock()
	sess, ok := sessions[id]
	if !ok {
		err = errors.New("Existing ssh conn for id '" + string(id) + "' not found...")
		log.Error(err)
		return err
	}
	log.WithFields(log.Fields{
		"id":   id,
		"conn": conn.conn,
	}).Info("Client creating new server conn")
	go sess.addConnAndListen(conn) // new goroutine because sessionslock
	return nil
}

func ClientReceivedSSHConnection(ssh net.Conn) SessionID {
	sess := newSession()
	log.WithFields(log.Fields{
		"ssh-conn": ssh,
		"id":       sess.sessionID,
	}).Info("Client received new ssh conn")
	sess.sshConn = &ssh
	go sess.listenSSH()
	return sess.sessionID
}
