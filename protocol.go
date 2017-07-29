package main

import (
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/howardstark/fusion/protos"
	log "github.com/sirupsen/logrus"
)

var packetDedupLock sync.Mutex
var (
	packetDedup = make(map[[32]byte]bool)
)

func shouldDedup(packet packets.Packet) bool {
	switch packet.GetBody().(type) {
	case *packets.Packet_Data:
		return false
	case *packets.Packet_Status:
		return true
	case *packets.Packet_Control:
		return true
	}
	panic("what is this packet")
}
func dedup(packet packets.Packet, rawPacket []byte) bool {
	if !shouldDedup(packet) {
		return false
	}
	hash := sha256.Sum256(rawPacket)
	packetDedupLock.Lock()
	defer packetDedupLock.Unlock()
	_, ok := packetDedup[hash]
	if ok {
		//fmt.Println("Ignoring already received packet with hash", hash)
		return true
	}
	packetDedup[hash] = true
	return false
}
func (sess *Session) wrap(data []byte) *OutgoingPacket {
	seq := sess.getOutgoingSeq()
	//fmt.Println("Wrapping packet with seq", seq)
	packet := packets.Packet{
		Body: &packets.Packet_Data{
			Data: &packets.Data{
				SequenceID: seq,
				Content:    data,
			},
		},
	}
	log.Debug("Packet constructed")
	marshalled := marshal(&packet)
	sess.outgoingLock.Lock()
	defer sess.outgoingLock.Unlock()
	out := &OutgoingPacket{
		seq:     seq,
		data:    &marshalled,
		session: sess,
		date:    time.Now().UnixNano(),
	}
	sess.outgoing[seq] = out
	return out
}
func marshal(packet *packets.Packet) []byte {
	packetData, packetErr := proto.Marshal(packet)
	if packetErr != nil {
		log.WithError(packetErr).Error("Marshal error")
		return nil
	}
	if len(packetData) >= 65535 {
		log.Error("Packet too large")
		return nil
	}
	packetLen := make([]byte, 2)
	binary.BigEndian.PutUint16(packetLen, uint16(len(packetData)))
	packetData = append(packetLen, packetData...)
	return packetData
}
func readProtoPacket(conn *Connection) (packets.Packet, error, []byte) {
	var packet packets.Packet
	packetLen := make([]byte, 2)
	lenErr := conn.ReadFull(packetLen)
	if lenErr != nil {
		return packet, lenErr, nil
	}
	l := binary.BigEndian.Uint16(packetLen)
	log.Debug("Reading packet of length ", l)
	packetData := make([]byte, l)
	dataErr := conn.ReadFull(packetData)
	if dataErr != nil {
		return packet, dataErr, packetData
	}
	unmarshErr := proto.Unmarshal(packetData, &packet)
	return packet, unmarshErr, packetData
}
