package forward

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"hello-tsnet/internal/config"
)

type Dialer interface {
	Dial(ctx context.Context, network, address string) (net.Conn, error)
}

type ListenFunc func(network, address string) (net.Listener, error)

type Manager struct {
	dialer Dialer
	listen ListenFunc

	mu      sync.Mutex
	active  map[int]*binding
	pending bool
	closed  bool
	state   atomic.Pointer[managerState]

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	dialTimeout time.Duration

	connMu sync.Mutex
	conns  map[net.Conn]struct{}
}

type managerState struct {
	rules   []config.Rule
	targets map[int]string
}

type binding struct {
	port     int
	listener net.Listener
}

type ChangeSet struct {
	manager *Manager
	desired *managerState
	staged  map[int]*binding

	mu   sync.Mutex
	done bool
}

func NewManager(dialer Dialer, listen ListenFunc) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		dialer: dialer,
		listen: listen,
		active: make(map[int]*binding),
		ctx:    ctx,
		cancel: cancel,
		conns:  make(map[net.Conn]struct{}),
		dialTimeout: 30 * time.Second,
	}
	m.state.Store(&managerState{rules: []config.Rule{}, targets: map[int]string{}})
	return m
}

func (m *Manager) Prepare(rules []config.Rule) (*ChangeSet, error) {
	if err := config.ValidateRules(rules); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, net.ErrClosed
	}
	if m.pending {
		return nil, errors.New("another rule change is already prepared")
	}
	m.pending = true

	desired := &managerState{
		rules:   append([]config.Rule(nil), rules...),
		targets: make(map[int]string, len(rules)),
	}
	staged := make(map[int]*binding)
	for _, rule := range rules {
		desired.targets[rule.LocalPort] = rule.Target
		if _, exists := m.active[rule.LocalPort]; exists {
			continue
		}
		address := net.JoinHostPort("127.0.0.1", strconv.Itoa(rule.LocalPort))
		listener, err := m.listen("tcp", address)
		if err != nil {
			for _, item := range staged {
				_ = item.listener.Close()
			}
			m.pending = false
			return nil, fmt.Errorf("listen on %s: %w", address, err)
		}
		staged[rule.LocalPort] = &binding{port: rule.LocalPort, listener: listener}
	}

	return &ChangeSet{manager: m, desired: desired, staged: staged}, nil
}

func (m *Manager) Rules() []config.Rule {
	state := m.state.Load()
	return append([]config.Rule(nil), state.rules...)
}

func (c *ChangeSet) Abort() {
	c.mu.Lock()
	if c.done {
		c.mu.Unlock()
		return
	}
	c.done = true
	c.mu.Unlock()

	for _, item := range c.staged {
		_ = item.listener.Close()
	}
	c.manager.mu.Lock()
	c.manager.pending = false
	c.manager.mu.Unlock()
}

func (c *ChangeSet) Commit() {
	c.mu.Lock()
	if c.done {
		c.mu.Unlock()
		return
	}
	c.done = true
	c.mu.Unlock()

	m := c.manager
	m.mu.Lock()
	m.state.Store(c.desired)
	toStart := make([]*binding, 0, len(c.staged))
	for port, item := range c.staged {
		m.active[port] = item
		toStart = append(toStart, item)
	}
	toClose := make([]*binding, 0)
	for port, item := range m.active {
		if _, keep := c.desired.targets[port]; keep {
			continue
		}
		delete(m.active, port)
		toClose = append(toClose, item)
	}
	m.pending = false
	m.mu.Unlock()

	for _, item := range toStart {
		m.wg.Add(1)
		go m.acceptLoop(item)
	}
	for _, item := range toClose {
		_ = item.listener.Close()
	}
}

func (m *Manager) acceptLoop(item *binding) {
	defer m.wg.Done()
	for {
		conn, err := item.listener.Accept()
		if err != nil {
			return
		}
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.handleConn(item.port, conn)
		}()
	}
}

func (m *Manager) handleConn(port int, local net.Conn) {
	m.track(local)
	defer func() {
		m.untrack(local)
		_ = local.Close()
	}()

	target, ok := m.state.Load().targets[port]
	if !ok {
		return
	}
	dialCtx, cancel := context.WithTimeout(m.ctx, m.dialTimeout)
	defer cancel()
	remote, err := m.dialer.Dial(dialCtx, "tcp", target)
	if err != nil {
		return
	}
	m.track(remote)
	defer func() {
		m.untrack(remote)
		_ = remote.Close()
	}()
	proxyConn(local, remote)
}

func (m *Manager) track(conn net.Conn) {
	m.connMu.Lock()
	m.conns[conn] = struct{}{}
	m.connMu.Unlock()
}

func (m *Manager) untrack(conn net.Conn) {
	m.connMu.Lock()
	delete(m.conns, conn)
	m.connMu.Unlock()
}

func (m *Manager) Close(ctx context.Context) {
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		m.cancel()
		for _, item := range m.active {
			_ = item.listener.Close()
		}
		m.active = make(map[int]*binding)
	}
	m.mu.Unlock()

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return
	case <-ctx.Done():
		m.connMu.Lock()
		for conn := range m.conns {
			_ = conn.Close()
		}
		m.connMu.Unlock()
		<-done
	}
}
