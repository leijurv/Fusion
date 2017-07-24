package main

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/howardstark/fusion/protos"
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
func (sess *Session) wrap(data []byte) *Sent {
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
	fmt.Println("Constructed")
	marshalled := marshal(&packet)
	sess.outgoingLock.Lock()
	defer sess.outgoingLock.Unlock()
	sent := &Sent{
		seq:     seq,
		data:    &marshalled,
		session: sess,
		date:    time.Now().UnixNano(),
	}
	sess.outgoing[seq] = sent
	return sent
}
func marshal(packet *packets.Packet) []byte {
	packetData, packetErr := proto.Marshal(packet)
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
	return packetData
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
