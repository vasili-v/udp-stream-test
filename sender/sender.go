package sender

import (
	"fmt"
	"io"
	"net"
	"sync/atomic"
)

type Sender struct {
	w   *net.UDPConn
	dst net.Addr

	buf []byte
	off int
	rem int

	start int
	end   int

	total int
	stats []int
}

func NewSender(w *net.UDPConn, dst net.Addr, size, maxMsgCount int) *Sender {
	s := &Sender{
		w:   w,
		dst: dst,
		buf: make([]byte, size),
		rem: size,
	}

	if maxMsgCount > 0 {
		s.stats = make([]int, maxMsgCount)
	}

	return s
}

func (s *Sender) Send(msg []byte, count *uint64) (error, bool) {
	flushed := false

	if len(msg) > s.rem {
		err := s.Flush()
		if err != nil {
			return err, true
		}

		if len(msg) > s.rem {
			return s.drop(msg), true
		}

		flushed = true
	}

	s.put(msg)

	if s.rem <= 0 || count != nil && atomic.LoadUint64(count) <= uint64(s.end) {
		return s.Flush(), true
	}

	return nil, flushed
}

func (s *Sender) Flush() error {
	size := len(s.buf) - s.rem
	if size <= 0 {
		return nil
	}

	n, err := s.w.WriteTo(s.buf[:size], s.dst)
	if err != nil {
		return fmt.Errorf("(%d - %d) %s", s.start, s.end, err)
	}

	if n != size {
		return fmt.Errorf("(%d - %d) expected %d sent %d bytes", s.start, s.end, size, n)
	}

	s.off = 0
	s.rem = len(s.buf)

	s.count(s.end - s.start)
	s.start = s.end

	return nil
}

func (s *Sender) Stats(w io.Writer) {
	if len(s.stats) <= 0 {
		return
	}

	fmt.Fprintf(w, "sending stats for %s (total %d in %d):\n", s.dst, s.end, s.total)
	for i, c := range s.stats {
		if c > 0 {
			if i < len(s.stats)-1 {
				fmt.Fprintf(w, "\t%d: %d\n", i, c)
			} else {
				fmt.Fprintf(w, "\trest: %d\n", c)
			}
		}
	}
}

func (s *Sender) drop(msg []byte) error {
	n, err := s.w.WriteTo(msg, s.dst)
	if err != nil {
		return fmt.Errorf("%d %s", s.start, err)
	}

	if n != len(msg) {
		return fmt.Errorf("%d expected %d sent %d bytes", s.start, len(msg), n)
	}

	s.count(1)
	s.start++
	s.end++

	return nil
}

func (s *Sender) put(msg []byte) {
	n := copy(s.buf[s.off:], msg)
	s.off += n
	s.rem -= n
	s.end++
}

func (s *Sender) count(n int) {
	s.total++

	last := len(s.stats) - 1
	if n > 0 && n < last {
		s.stats[n]++
	} else if last >= 0 {
		s.stats[last]++
	}
}
