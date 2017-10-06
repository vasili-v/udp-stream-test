package main

import "net"

type dispatcher struct {
	c   *net.UDPConn
	chs map[string]chan []byte
}

func newDispatcher(c *net.UDPConn) *dispatcher {
	return &dispatcher{
		c:   c,
		chs: make(map[string]chan []byte),
	}
}

func (d *dispatcher) dispatch(data []byte, src net.Addr) {
	msgs := make([]byte, len(data))
	copy(msgs, data)

	idx := src.String()
	ch, ok := d.chs[idx]
	if !ok {
		ch = make(chan []byte)
		d.chs[idx] = ch

		go handleSrc(d.c, ch, src)
	}

	ch <- msgs
}
