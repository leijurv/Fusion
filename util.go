package main

import (
	mrand "math/rand"
	"time"

	log "github.com/sirupsen/logrus"
)

func randomize(buf []byte, sess *Session) {
	if len(buf) < 10 {
		log.WithField("buf-len", len(buf)).Debug("Buffer too small to split")
		sess.sendPacket(sess.wrap(buf))
		return
	}

	parts := 5
	partSize := len(buf) / parts
	log.WithField("buf-len", len(buf)).Debug("Splitting buffer")
	packets := make([]*OutgoingPacket, parts+1)
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
		log.WithField("packets-len", len(packets)).Debug("Expected len ", len(buf), "got", totalSize)
		panic("") // somehow the splitter and reorderer ended up with the wrong number of total bytes
	}

	rSrc := mrand.New(mrand.NewSource(time.Now().UnixNano()))
	perm := rSrc.Perm(len(packets))
	for i := 0; i < len(perm); i++ {
		sess.sendPacket(packets[perm[i]])
	}
}
