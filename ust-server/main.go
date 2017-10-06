package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vasili-v/udp-stream-test/sender"
)

func handleSrc(c *net.UDPConn, in chan []byte, src net.Addr) {
	fmt.Printf("got first message from %s\n", src)
	out := make(chan []byte, limit)

	var received uint64
	go readSrc(c, in, out, &received, src)
	writeSrcBuf(c, out, &received, src)
}

func readSrc(c *net.UDPConn, in, out chan []byte, count *uint64, src net.Addr) {
	var wg sync.WaitGroup
	defer func() {
		wg.Wait()
		close(out)
	}()

	th := make(chan int, limit)
	for data := range in {
		for len(data) > 0 {
			msg, n, err := extractMsg(data)
			if err != nil {
				fmt.Printf("reading from %s error: %s\n", src, err)
				break
			}

			data = data[n:]

			wg.Add(1)
			atomic.AddUint64(count, 1)

			th <- 0
			go handleMsg(msg, out, func() {
				wg.Done()
				<-th
			})
		}
	}
}

func writeSrcBuf(c *net.UDPConn, out chan []byte, count *uint64, dst net.Addr) {
	s := sender.NewSender(c, dst, bufSize, bufSize/6+2)
	for msg := range out {
		err, _ := s.Send(msg, count)
		if err != nil {
			panic(fmt.Errorf("sending to %s error: %s", dst, err))
		}
	}
}

func extractMsg(data []byte) ([]byte, int, error) {
	if len(data) < 2 {
		return nil, 0, fmt.Errorf("expected at least 2 bytes got %d", len(data))
	}

	size := int(binary.BigEndian.Uint16(data[:2]))

	data = data[2:]
	if len(data) < size {
		return nil, 0, fmt.Errorf("expected at least %d bytes got %d\n", size, len(data))
	}

	msg := make([]byte, size)
	copy(msg, data)

	return msg, size + 2, nil
}

func handleMsg(req []byte, out chan []byte, f func()) {
	defer f()

	size := len(req)
	if size < 4 {
		fmt.Printf("expected at least 4 bytes in request but got %d\n", size)
		return
	}

	time.Sleep(2 * time.Microsecond)

	id := binary.BigEndian.Uint32(req)

	res := make([]byte, size+2)
	binary.BigEndian.PutUint16(res, uint16(size))
	binary.BigEndian.PutUint32(res[2:], id)
	for i := 6; i < len(res); i++ {
		res[i] = 0x55
	}

	out <- res
}

func main() {
	udpAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		panic(fmt.Errorf("resolving address error: %s", err))
	}

	c, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		panic(fmt.Errorf("opening port error: %s", err))
	}
	defer c.Close()

	d := newDispatcher(c)

	buf := make([]byte, 64*1024)
	for {
		n, src, err := c.ReadFrom(buf)
		if n > 0 {
			d.dispatch(buf[:n], src)
		}

		if err != nil {
			panic(fmt.Errorf("reading error: %s", err))
		}
	}
}
