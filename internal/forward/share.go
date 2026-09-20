package forward

import (
	"context"
	"net"
	"time"
)

// tsnet listeners return a connection per TCP connection or UDP flow. Each
// shared flow dials only the rule's derived loopback endpoint on this machine.
func (m *Manager) shareLoop(item *binding) {
	defer m.wg.Done()
	for {
		conn, err := item.listener.Accept()
		if err != nil { return }
		select {
		case item.slots <- struct{}{}:
			m.wg.Add(1)
			go func() {
				defer m.wg.Done()
				defer func() { <-item.slots }()
				m.handleShared(item, conn)
			}()
		default:
			_ = conn.Close()
		}
	}
}

func (m *Manager) handleShared(item *binding, incoming net.Conn) {
	defer incoming.Close()
	stopIncoming := context.AfterFunc(item.ctx, func() { _ = incoming.Close() })
	defer stopIncoming()
	target, ok := m.state.Load().targets[item.key]
	if !ok || item.ctx.Err() != nil { return }
	dialCtx, cancel := context.WithTimeout(item.ctx, m.dialTimeout)
	defer cancel()
	local, err := (&net.Dialer{}).DialContext(dialCtx, item.key.network, target)
	if err != nil { return }
	defer local.Close()
	stopLocal := context.AfterFunc(item.ctx, func() { _ = local.Close() })
	defer stopLocal()
	if item.key.network == "udp" {
		proxyDatagrams(incoming, local, m.udpIdleTimeout)
	} else {
		proxyConn(incoming, local)
	}
}

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
			if err != nil { return }
			refresh()
			_ = dst.SetWriteDeadline(time.Now().Add(udpWriteTimeout))
			if written, err := dst.Write(buffer[:n]); err != nil || written != n { return }
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
