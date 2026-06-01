package main

import "net"

// chanListener is a channel-backed net.Listener for the FD mode control plane.
type chanListener struct {
	ch   chan net.Conn
	addr net.Addr
}

func newChanListener(ip net.IP, port int) *chanListener {
	return &chanListener{
		ch:   make(chan net.Conn, 16),
		addr: &net.TCPAddr{IP: ip, Port: port},
	}
}

func (cl *chanListener) deliver(conn net.Conn) {
	select {
	case cl.ch <- conn:
	default:
		conn.Close()
	}
}

func (cl *chanListener) Accept() (net.Conn, error) {
	conn, ok := <-cl.ch
	if !ok {
		return nil, net.ErrClosed
	}
	return conn, nil
}

func (cl *chanListener) Close() error {
	close(cl.ch)
	return nil
}

func (cl *chanListener) Addr() net.Addr {
	return cl.addr
}

