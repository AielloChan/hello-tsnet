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
type ListenPacketFunc func(network, address string) (net.PacketConn, error)

type socketKey struct {
	direction config.Direction
	network string
	port int
}

type Manager struct {
	dialer Dialer
	listen ListenFunc
	listenPacket ListenPacketFunc
	shareListen ListenFunc

	mu      sync.Mutex
	active  map[socketKey]*binding
	pending bool
	closed  bool
	state   atomic.Pointer[managerState]

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	dialTimeout time.Duration
	udpIdleTimeout time.Duration
	udpSessionLimit int
	shareConnectionLimit int

	connMu sync.Mutex
	conns  map[net.Conn]struct{}
}

type managerState struct {
	rules   []config.Rule
	targets map[socketKey]string
}

type binding struct {
	key socketKey
	ctx context.Context
	cancel context.CancelFunc
	slots chan struct{}
	port     int
	listener net.Listener
	udp      *udpRelay
}

type ChangeSet struct {
	manager *Manager
	desired *managerState
	staged  map[socketKey]*binding

	mu   sync.Mutex
	done bool
}

func NewManager(dialer Dialer, listen ListenFunc, listenPacket ListenPacketFunc, shareListen ListenFunc) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		dialer: dialer,
		listen: listen,
		listenPacket: listenPacket,
		shareListen: shareListen,
		active: make(map[socketKey]*binding),
		ctx:    ctx,
		cancel: cancel,
		conns:  make(map[net.Conn]struct{}),
		dialTimeout: 30 * time.Second,
		udpIdleTimeout: 60 * time.Second,
		udpSessionLimit: 128,
		shareConnectionLimit: 128,
	}
	m.state.Store(&managerState{rules: []config.Rule{}, targets: map[socketKey]string{}})
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
		targets: make(map[socketKey]string, len(rules)),
	}
	staged := make(map[socketKey]*binding)
	for _, rule := range rules {
		for _, network := range rule.Networks() {
			key := socketKey{direction: rule.EffectiveDirection(), network: network, port: rule.LocalPort}
			desired.targets[key] = rule.TargetAddress()
			if _, exists := m.active[key]; exists { continue }
			address := net.JoinHostPort("127.0.0.1", strconv.Itoa(rule.LocalPort))
			item := &binding{port: rule.LocalPort, key: key}
			var err error
			if key.direction == config.DirectionInbound {
				address = ":" + strconv.Itoa(rule.LocalPort)
				if m.shareListen == nil {
					err = errors.New("tailnet listener is unavailable")
				} else {
					item.listener, err = m.shareListen(network, address)
				}
				if err == nil {
					item.ctx, item.cancel = context.WithCancel(m.ctx)
					item.slots = make(chan struct{}, m.shareConnectionLimit)
				}
			} else if network == "tcp" {
				item.listener, err = m.listen(network, address)
			} else {
				var packet net.PacketConn
				packet, err = m.listenPacket(network, address)
				if err == nil { item.udp = newUDPRelay(m, key, packet) }
			}
			if err != nil {
				for _, prepared := range staged { prepared.close() }
				m.pending = false
				return nil, fmt.Errorf("listen on %s %s: %w", network, address, err)
			}
			staged[key] = item
		}
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
		item.close()
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
	if m.closed {
		for _, item := range c.staged { item.close() }
		m.pending = false
		m.mu.Unlock()
		return
	}
	previous := m.state.Load()
	m.state.Store(c.desired)
	toStart := make([]*binding, 0, len(c.staged))
	for port, item := range c.staged {
		m.active[port] = item
		toStart = append(toStart, item)
	}
	toClose := make([]*binding, 0)
	for port, item := range m.active {
		if target, keep := c.desired.targets[port]; keep {
			if item.udp != nil && previous.targets[port] != target { item.udp.resetSessions() }
			continue
		}
		delete(m.active, port)
		toClose = append(toClose, item)
	}
	m.pending = false
	// Register loops before Close can begin waiting on the group.
	m.wg.Add(len(toStart))
	m.mu.Unlock()

	for _, item := range toStart {
		if item.key.direction == config.DirectionInbound { go m.shareLoop(item) } else if item.udp != nil { go item.udp.run() } else { go m.acceptLoop(item) }
	}
	for _, item := range toClose {
		item.close()
	}
}

func (b *binding) close() {
	if b.cancel != nil { b.cancel() }
	if b.udp != nil { b.udp.close() } else { _ = b.listener.Close() }
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

	target, ok := m.state.Load().targets[socketKey{direction: config.DirectionOutbound, network: "tcp", port: port}]
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
			item.close()
		}
		m.active = make(map[socketKey]*binding)
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
