package forward

import (
	"net"
	"time"
)

const udpWriteTimeout = 5 * time.Second

func proxyDatagrams(left, right net.Conn, idle time.Duration) {
	refresh := func() {
		deadline := time.Now().Add(idle)
		_ = left.SetReadDeadline(deadline)
		_ = right.SetReadDeadline(deadline)
	}
	refresh()
	copyPackets := func(dst, src net.Conn) {
		buffer := make([]byte, 65535)
		for {
			n, err := src.Read(buffer)
			if err != nil {
				return
			}
			refresh()
			_ = dst.SetWriteDeadline(time.Now().Add(udpWriteTimeout))
			if written, err := dst.Write(buffer[:n]); err != nil || written != n {
				return
			}
		}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		copyPackets(left, right)
		_ = left.Close()
		_ = right.Close()
	}()
	copyPackets(right, left)
	_ = left.Close()
	_ = right.Close()
	<-done
}
