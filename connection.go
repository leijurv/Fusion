package main

import (
	"fmt"
	"io"
	"net"
	"sync"
)

type TcpConnection struct {
	iface   string
	conn    net.Conn
	outChan chan []byte
	running bool
	closed  error
	lock    sync.Mutex
}

func (conn *TcpConnection) Read(data []byte) (int, error) {
	a, b := conn.conn.Read(data)
	return a, b
}
func (conn *TcpConnection) ReadFull(data []byte) error {
	_, b := io.ReadFull(conn.conn, data)
	return b
}
func (conn *TcpConnection) Write(data []byte) error {
	conn.lock.Lock()
	defer conn.lock.Unlock()
	if conn.closed != nil {
		return conn.closed // if the writeloop has encountered an error, return it here
	}
	if !conn.running { // conn.closed == nil && !conn.running means it hasn't started yet
		conn.start()
	}
	conn.outChan <- data
	return nil
}
func (conn *TcpConnection) start() {
	conn.outChan = make(chan []byte, 4)
	conn.running = true
	go conn.writeloop()
}
func (conn *TcpConnection) writeloop() {
	for {
		data := <-conn.outChan
		a, err := conn.conn.Write(data)
		if err == nil && a != len(data) {
			panic("what the christ " + string(a) + " " + string(len(data)))
		}
		if err != nil || !conn.running {
			conn.lock.Lock()
			defer conn.lock.Unlock()
			conn.closed = err
			close(conn.outChan)
			conn.running = false
			fmt.Println("closing", conn.conn, "because", err)
			go conn.Close()
			return
		}
	}
}
func (conn *TcpConnection) Close() {
	conn.conn.Close()
	conn.lock.Lock()
	if conn.running {
		close(conn.outChan)
	}
	conn.running = false
	conn.lock.Unlock()
	select {
	case conn.outChan <- []byte("goodbye"):
	default:
	}
}
func (conn *TcpConnection) LocalAddr() net.Addr {
	return conn.conn.LocalAddr()
}

type Connection interface {
	Read(data []byte) (int, error)
	ReadFull(data []byte) error
	Write(data []byte) error
	Close()
	LocalAddr() net.Addr
}
