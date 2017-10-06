package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/vasili-v/udp-stream-test/sender"
)

type pair struct {
	req []byte

	sent time.Time
	recv *time.Time
	dup  int
}

func main() {
	pairs := newPairs(total, msgSize)
	waits := newPairs(total, 0)

	udpAddr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		panic(fmt.Errorf("resolving address: %s", err))
	}

	fmt.Fprintf(os.Stderr, "sending to %s\n", udpAddr)
	c, err := net.ListenUDP("udp", nil)
	if err != nil {
		panic(fmt.Errorf("dialing error: %s", err))
	}
	defer c.Close()

	miss := 0
	ch := make(chan int)

	th := make(chan int, limit)
	count := len(pairs)
	go func() {
		defer close(ch)

		for {
			buf := make([]byte, 64*1024)
			n, src, err := c.ReadFrom(buf)
			if n > 0 {
				data := buf[:n]
				for len(data) > 0 {
					msg, n, err := extractMsg(data)
					if err != nil {
						if src == nil {
							fmt.Fprintf(os.Stderr, "reading error: %s\n", err)
						} else {
							fmt.Fprintf(os.Stderr, "reading from %s error: %s\n", src, err)
						}
						return
					}

					data = data[n:]
					<-th

					id := binary.BigEndian.Uint32(msg)
					if id < uint32(len(pairs)) {
						p := pairs[id]
						if p.recv == nil {
							t := time.Now()
							p.recv = &t
							count--
							if count <= 0 {
								return
							}
						} else {
							p.dup++
						}
					} else {
						miss++
					}
				}
			}

			if err != nil {
				panic(fmt.Errorf("reading from %s error: %s", src, err))
			}
		}
	}()

	wcount := 0
	snd := sender.NewSender(c, udpAddr, bufSize, bufSize/msgSize+2)
	for _, p := range pairs {
		select {
		default:
			waits[wcount].sent = time.Now()
			th <- 0
			t := time.Now()
			waits[wcount].recv = &t
			wcount++

		case th <- 0:
		}

		p.sent = time.Now()
		err, ok := snd.Send(p.req, nil)
		if err != nil {
			panic(fmt.Errorf("sending error: %s", err))
		}

		if ok {
			time.Sleep(cooldown)
		}
	}

	if err := snd.Flush(); err != nil {
		panic(fmt.Errorf("sending error: %s", err))
	}

	if wcount > 0 {
		waits = waits[:wcount]
	}

	snd.Stats(os.Stderr)

	if count > 0 {
		fmt.Fprintf(os.Stderr, "waiting for %d responses\n", count)
	}

	select {
	case <-ch:
	case <-time.After(timeout):
	}

	if count > 0 {
		fmt.Fprintf(os.Stderr, "got %d responses\n", len(pairs)-count)
	} else {
		fmt.Fprintf(os.Stderr, "got all %d responses\n", len(pairs))
	}

	if miss > 0 {
		panic(fmt.Errorf("got %d messages with invalid ids", miss))
	}

	dup := 0
	for _, p := range pairs {
		dup += p.dup
	}

	if dup > 0 {
		panic(fmt.Errorf("got %d duplicates", dup))
	}

	dump(pairs, "")
	if len(waitsDump) > 0 && wcount > 0 {
		dump(waits, waitsDump)
	}
}

func newPairs(n, size int) []*pair {
	out := make([]*pair, n)

	if size > 0 {
		fmt.Fprintf(os.Stderr, "making messages to send:\n")
	}

	for i := range out {
		if size > 0 {
			buf := make([]byte, size)
			binary.BigEndian.PutUint16(buf, uint16(len(buf)-2))
			binary.BigEndian.PutUint32(buf[2:], uint32(i))
			for i := 6; i < len(buf); i++ {
				buf[i] = 0xaa
			}

			if i < 3 {
				fmt.Fprintf(os.Stderr, "\t%d: % x\n", i, buf)
			} else if i == 3 {
				fmt.Fprintf(os.Stderr, "\t%d: ...\n", i)
			}

			out[i] = &pair{req: buf}
		} else {
			out[i] = &pair{}
		}
	}

	return out
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
