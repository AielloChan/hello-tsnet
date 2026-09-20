package forward

import (
	"context"
	"net"
	"sync"
	"time"
)

// Queues and the per-listener session limit bound memory while a peer is slow.
const udpQueueSize = 8
const udpWriteTimeout = 5 * time.Second

type udpRelay struct {
	manager *Manager
	key socketKey
	local net.PacketConn
	ctx context.Context
	cancel context.CancelFunc
	mu sync.Mutex
	sessions map[string]*udpSession
	closed bool
}

type udpSession struct {
	relay *udpRelay
	client net.Addr
	target string
	ctx context.Context
	cancel context.CancelFunc
	packets chan []byte
}

func newUDPRelay(manager *Manager, key socketKey, local net.PacketConn) *udpRelay {
	ctx, cancel := context.WithCancel(manager.ctx)
	return &udpRelay{manager: manager, key: key, local: local, ctx: ctx, cancel: cancel, sessions: make(map[string]*udpSession)}
}

func (r *udpRelay) run() {
	defer r.manager.wg.Done()
	defer r.close()
	buf := make([]byte, 65535)
	for {
		n, client, err := r.local.ReadFrom(buf)
		if err != nil { return }
		r.enqueue(client, buf[:n])
	}
}

func (r *udpRelay) enqueue(client net.Addr, packet []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.ctx.Err() != nil { return }
	target, ok := r.manager.state.Load().targets[r.key]
	if !ok { return }
	key := client.String()
	session := r.sessions[key]
	if session != nil && (session.target != target || session.ctx.Err() != nil) {
		session.cancel()
		delete(r.sessions, key)
		session = nil
	}
	if session == nil {
		if len(r.sessions) >= r.manager.udpSessionLimit { return }
		ctx, cancel := context.WithCancel(r.ctx)
		session = &udpSession{relay: r, client: client, target: target, ctx: ctx, cancel: cancel, packets: make(chan []byte, udpQueueSize)}
		r.sessions[key] = session
		r.manager.wg.Add(1)
		go session.run()
	}
	// UDP has no backpressure: drop overflow rather than block other clients.
	select {
	case session.packets <- append([]byte(nil), packet...):
	default:
	}
}

func (r *udpRelay) resetSessions() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, session := range r.sessions {
		session.cancel()
		delete(r.sessions, key)
	}
}

func (r *udpRelay) close() {
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		r.cancel()
		_ = r.local.Close()
	}
	r.mu.Unlock()
}

func (s *udpSession) current() bool {
	if s.ctx.Err() != nil { return false }
	target, ok := s.relay.manager.state.Load().targets[s.relay.key]
	return ok && target == s.target
}

func (s *udpSession) run() {
	r := s.relay
	defer r.manager.wg.Done()
	defer func() {
		s.cancel()
		r.mu.Lock()
		if r.sessions[s.client.String()] == s { delete(r.sessions, s.client.String()) }
		r.mu.Unlock()
	}()
	dialCtx, cancel := context.WithTimeout(s.ctx, r.manager.dialTimeout)
	remote, err := r.manager.dialer.Dial(dialCtx, "udp", s.target)
	cancel()
	if err != nil { return }
	stopClose := context.AfterFunc(s.ctx, func() { _ = remote.Close() })
	defer stopClose()
	readDone := make(chan struct{})
	defer func() { _ = remote.Close(); <-readDone }()
	remote.SetReadDeadline(time.Now().Add(r.manager.udpIdleTimeout))
	go func() {
		defer close(readDone)
		defer s.cancel()
		buf := make([]byte, 65535)
		for {
			n, err := remote.Read(buf)
			if err != nil || !s.current() { return }
			remote.SetReadDeadline(time.Now().Add(r.manager.udpIdleTimeout))
			// The loopback socket is shared, so its write deadline is always bounded.
			r.local.SetWriteDeadline(time.Now().Add(udpWriteTimeout))
			if _, err := r.local.WriteTo(buf[:n], s.client); err != nil { return }
		}
	}()
	for {
		select {
		case <-s.ctx.Done():
			return
		case packet := <-s.packets:
			if !s.current() { return }
			remote.SetReadDeadline(time.Now().Add(r.manager.udpIdleTimeout))
			remote.SetWriteDeadline(time.Now().Add(udpWriteTimeout))
			if _, err := remote.Write(packet); err != nil { return }
		}
	}
}
