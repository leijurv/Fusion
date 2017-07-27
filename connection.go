package main

import (
	"io"
	"net"
)

type TcpConnection struct {
	iface                      string
	conn                       net.Conn
	lastSuccessfulDataTransfer uint64 //idk more fields here
}

func (conn *TcpConnection) Read(data []byte) (int, error) {
	a, b := conn.conn.Read(data)
	return a, b
}
func (conn *TcpConnection) ReadFull(data []byte) error {
	_, b := io.ReadFull(conn.conn, data)
	return b
}
func (conn *TcpConnection) Write(data []byte) error { //TODO lock?
	a, b := conn.conn.Write(data)
	if a != len(data) {
		panic("what the christ")
	}
	return b
}
func (conn *TcpConnection) Close() {
	conn.conn.Close()
}

type Connection interface {
	Read(data []byte) (int, error)
	ReadFull(data []byte) error
	Write(data []byte) error
	Close()
}