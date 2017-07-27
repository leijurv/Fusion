package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
)

type Connection interface {
	Read(data []byte) (int, error)
	ReadFull(data []byte) error
	Write(data []byte) error
	WriteNonBlocking(data []byte) (bool, error)
	Close()
	LocalAddr() net.Addr
}

type TcpConnection struct {
	iface   string
	conn    net.Conn
	outChan chan []byte
	running bool
	closed  error
	lock    sync.Mutex
}

func (conn *TcpConnection) Read(data []byte) (int, error) {
	if conn.closed != nil {
		return 0, conn.closed
	}
	a, b := conn.conn.Read(data)
	return a, b
}
func (conn *TcpConnection) ReadFull(data []byte) error {
	if conn.closed != nil {
		return conn.closed
	}
	_, b := io.ReadFull(conn.conn, data)
	return b
}
func (conn *TcpConnection) WriteNonBlocking(data []byte) (bool, error) {
	conn.lock.Lock()
	defer conn.lock.Unlock()
	if conn.closed != nil {
		return false, conn.closed // if the writeloop has encountered an error, return it here
	}
	if !conn.running { // conn.closed == nil && !conn.running means it hasn't started yet
		conn.start()
	}
	select {
	case conn.outChan <- data:
		return true, nil
	default:
		return false, nil
	}
}
func (conn *TcpConnection) Write(data []byte) error {
	ok, err := conn.WriteNonBlocking(data)
	if err != nil {
		return err
	}
	if !ok {
		conn.outChan <- data // blocking write
		fmt.Println("Wrote with blocking =(")
	}
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
			if conn.running {
				close(conn.outChan)
			}
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
	defer conn.lock.Unlock()
	if conn.closed == nil {
		conn.closed = errors.New("close requested")
	}
	if conn.running {
		conn.running = false
		select {
		case conn.outChan <- []byte("goodbye"):
		default:
		}
		close(conn.outChan)
	}
}
func (conn *TcpConnection) LocalAddr() net.Addr {
	return conn.conn.LocalAddr()
}
