package forward

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"hello-tsnet/internal/config"
)

func udpRule(t *testing.T, port int, target, protocol string) config.Rule {
	t.Helper()
	data, _ := json.Marshal(map[string]any{"id": "test", "name": "test", "localPort": port, "target": target, "protocol": protocol})
	var rule config.Rule
	if err := json.Unmarshal(data, &rule); err != nil { t.Fatal(err) }
	return rule
}

func closeManager(t *testing.T, manager *Manager) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	manager.Close(ctx)
}

func udpEcho(t *testing.T, prefix string) string {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { conn.Close() })
	go func() {
		buf := make([]byte, 65535)
		for {
			n, addr, err := conn.ReadFrom(buf)
			if err != nil { return }
			conn.WriteTo(append([]byte(prefix), buf[:n]...), addr)
		}
	}()
	return conn.LocalAddr().String()
}

type directUDPDialer struct { calls atomic.Int32 }
func (d *directUDPDialer) Dial(ctx context.Context, network, target string) (net.Conn, error) {
	d.calls.Add(1)
	return (&net.Dialer{}).DialContext(ctx, network, target)
}

func preparedUDP(t *testing.T, dialer Dialer) (*Manager, int) {
	t.Helper()
	packet, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil { t.Fatal(err) }
	port := packet.LocalAddr().(*net.UDPAddr).Port
	manager := NewManager(dialer, net.Listen, func(network, address string) (net.PacketConn, error) {
		if network != "udp" || address != net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) { t.Errorf("unexpected packet listen %s %s", network, address) }
		return packet, nil
	}, nil)
	t.Cleanup(func() { closeManager(t, manager); packet.Close() })
	return manager, port
}

func udpClient(t *testing.T, port int) net.Conn {
	t.Helper()
	conn, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { conn.Close() })
	return conn
}

func exchangeUDP(t *testing.T, conn net.Conn, payload, want []byte) {
	t.Helper()
	conn.SetDeadline(time.Now().Add(time.Second))
	if _, err := conn.Write(payload); err != nil { t.Fatal(err) }
	buf := make([]byte, 65535)
	n, err := conn.Read(buf)
	if err != nil { t.Fatal(err) }
	if !bytes.Equal(buf[:n], want) { t.Fatalf("datagram length=%d, want=%d (content mismatch)", n, len(want)) }
}

func TestUDPDatagramsAndClientsRemainSeparate(t *testing.T) {
	dialer := &directUDPDialer{}
	manager, port := preparedUDP(t, dialer)
	change, err := manager.Prepare([]config.Rule{udpRule(t, port, udpEcho(t, ""), "udp")})
	if err != nil { t.Fatal(err) }
	change.Commit()
	first, second := udpClient(t, port), udpClient(t, port)
	for _, payload := range [][]byte{[]byte("first"), {}, bytes.Repeat([]byte{0xab}, 8192), []byte("last")} {
		exchangeUDP(t, first, payload, payload)
		exchangeUDP(t, second, []byte("second"), []byte("second"))
	}
	if dialer.calls.Load() != 2 { t.Fatalf("dial calls = %d, want 2 per-client sessions", dialer.calls.Load()) }
}

func TestUDPIdleSessionIsRecreated(t *testing.T) {
	dialer := &directUDPDialer{}
	manager, port := preparedUDP(t, dialer)
	manager.udpIdleTimeout = 30 * time.Millisecond
	change, err := manager.Prepare([]config.Rule{udpRule(t, port, udpEcho(t, ""), "udp")})
	if err != nil { t.Fatal(err) }
	change.Commit()
	client := udpClient(t, port)
	exchangeUDP(t, client, []byte("one"), []byte("one"))
	time.Sleep(100 * time.Millisecond)
	exchangeUDP(t, client, []byte("two"), []byte("two"))
	if dialer.calls.Load() != 2 { t.Fatalf("idle session was not recycled: %d dials", dialer.calls.Load()) }
}

func TestUDPChangesTargetWithoutRebinding(t *testing.T) {
	manager, port := preparedUDP(t, &directUDPDialer{})
	rule := udpRule(t, port, udpEcho(t, "old:"), "udp")
	change, err := manager.Prepare([]config.Rule{rule})
	if err != nil { t.Fatal(err) }
	change.Commit()
	client := udpClient(t, port)
	exchangeUDP(t, client, []byte("one"), []byte("old:one"))
	rule.Target = udpEcho(t, "new:")
	change, err = manager.Prepare([]config.Rule{rule})
	if err != nil { t.Fatal(err) }
	change.Commit()
	exchangeUDP(t, client, []byte("two"), []byte("new:two"))
	change, err = manager.Prepare(nil)
	if err != nil { t.Fatal(err) }
	change.Commit()
	conn, err := net.ListenPacket("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil { t.Fatalf("deleted UDP port was not released: %v", err) }
	conn.Close()
}

func TestBothPreparationFailurePreservesTCP(t *testing.T) {
	listener := newFakeListener()
	manager := NewManager(stubDialer{}, func(string, string) (net.Listener, error) { return listener, nil },
		func(string, string) (net.PacketConn, error) { return nil, errors.New("UDP port occupied") }, nil)
	defer closeManager(t, manager)
	rule := udpRule(t, 30001, "host:22", "tcp")
	change, err := manager.Prepare([]config.Rule{rule})
	if err != nil { t.Fatal(err) }
	change.Commit()
	rule = udpRule(t, 30001, "host:22", "both")
	if _, err := manager.Prepare([]config.Rule{rule}); err == nil { t.Fatal("both must fail if UDP cannot bind") }
	if listener.isClosed() || len(manager.Rules()) != 1 { t.Fatal("UDP failure damaged working TCP rule") }
}

func TestUDPAbortNeverDialsAndReleasesSocket(t *testing.T) {
	dialer := &directUDPDialer{}
	manager, port := preparedUDP(t, dialer)
	change, err := manager.Prepare([]config.Rule{udpRule(t, port, udpEcho(t, ""), "udp")})
	if err != nil { t.Fatal(err) }
	client := udpClient(t, port)
	client.Write([]byte("staged"))
	change.Abort()
	if dialer.calls.Load() != 0 { t.Fatal("UDP dialed before commit") }
	conn, err := net.ListenPacket("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil { t.Fatalf("Abort did not release UDP port: %v", err) }
	conn.Close()
}

func TestBothFailureClosesNewTCPListener(t *testing.T) {
	listener := newFakeListener()
	manager := NewManager(stubDialer{}, func(string, string) (net.Listener, error) { return listener, nil },
		func(string, string) (net.PacketConn, error) { return nil, errors.New("occupied") }, nil)
	defer closeManager(t, manager)
	if _, err := manager.Prepare([]config.Rule{udpRule(t, 30001, "host:22", "both")}); err == nil {
		t.Fatal("both must fail if either protocol cannot bind")
	}
	if !listener.isClosed() || len(manager.Rules()) != 0 { t.Fatal("failed transaction leaked staged TCP listener or published a rule") }
}

func TestBothProtocolsForwardAndCanBeDisabledIndependently(t *testing.T) {
	targetTCP, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil { t.Fatal(err) }
	defer targetTCP.Close()
	targetUDP, err := net.ListenPacket("udp", targetTCP.Addr().String())
	if err != nil { t.Fatal(err) }
	defer targetUDP.Close()
	go func() {
		for {
			conn, err := targetTCP.Accept()
			if err != nil { return }
			go func() {
				defer conn.Close()
				buf := make([]byte, 100)
				n, err := conn.Read(buf)
				if err == nil { conn.Write(buf[:n]) }
			}()
		}
	}()
	go func() {
		buf := make([]byte, 100)
		for {
			n, addr, err := targetUDP.ReadFrom(buf)
			if err != nil { return }
			targetUDP.WriteTo(buf[:n], addr)
		}
	}()
	manager, port := preparedUDP(t, &directUDPDialer{})
	for _, mode := range []string{"both", "udp"} {
		change, err := manager.Prepare([]config.Rule{udpRule(t, port, targetTCP.Addr().String(), mode)})
		if err != nil { t.Fatal(err) }
		change.Commit()
		client := udpClient(t, port)
		exchangeUDP(t, client, []byte("udp"), []byte("udp"))
		conn, err := net.DialTimeout("tcp", client.RemoteAddr().String(), time.Second)
		if mode == "both" {
			if err != nil { t.Fatal(err) }
			exchangeUDP(t, conn, []byte("tcp"), []byte("tcp"))
			conn.Close()
		} else if err == nil { conn.Close(); t.Fatal("TCP listener still open after switching to UDP") }
	}
	change, err := manager.Prepare([]config.Rule{udpRule(t, port, targetTCP.Addr().String(), "tcp")})
	if err != nil { t.Fatal(err) }
	change.Commit()
	packet, err := net.ListenPacket("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil { t.Fatalf("UDP listener still open after switching to TCP: %v", err) }
	packet.Close()
}

type testDialFunc func(context.Context, string, string) (net.Conn, error)
func (f testDialFunc) Dial(ctx context.Context, network, address string) (net.Conn, error) { return f(ctx, network, address) }

func TestUDPBlockedClientDoesNotBlockOtherClientsAndShutdown(t *testing.T) {
	started := make(chan struct{})
	var calls atomic.Int32
	dialer := testDialFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	})
	manager, port := preparedUDP(t, dialer)
	change, err := manager.Prepare([]config.Rule{udpRule(t, port, udpEcho(t, ""), "udp")})
	if err != nil { t.Fatal(err) }
	change.Commit()
	udpClient(t, port).Write([]byte("slow"))
	select { case <-started: case <-time.After(time.Second): t.Fatal("dial did not begin") }
	exchangeUDP(t, udpClient(t, port), []byte("fast"), []byte("fast"))
	done := make(chan struct{})
	go func() { closeManager(t, manager); close(done) }()
	select { case <-done: case <-time.After(time.Second): t.Fatal("shutdown did not cancel pending UDP dial and active session") }
}

func TestUDPSessionLimitDoesNotDisturbExistingClient(t *testing.T) {
	dialer := &directUDPDialer{}
	manager, port := preparedUDP(t, dialer)
	manager.udpSessionLimit = 1
	change, err := manager.Prepare([]config.Rule{udpRule(t, port, udpEcho(t, ""), "udp")})
	if err != nil { t.Fatal(err) }
	change.Commit()
	first, second := udpClient(t, port), udpClient(t, port)
	exchangeUDP(t, first, []byte("one"), []byte("one"))
	second.SetDeadline(time.Now().Add(50 * time.Millisecond))
	second.Write([]byte("overflow"))
	if _, err := second.Read(make([]byte, 100)); err == nil { t.Fatal("accepted a session beyond the cap") }
	exchangeUDP(t, first, []byte("still works"), []byte("still works"))
	if dialer.calls.Load() != 1 { t.Fatalf("dialed beyond cap: %d", dialer.calls.Load()) }
}

func TestCommitAfterShutdownReleasesPreparedSockets(t *testing.T) {
	packet, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil { t.Fatal(err) }
	defer packet.Close()
	listener := newFakeListener()
	port := packet.LocalAddr().(*net.UDPAddr).Port
	manager := NewManager(stubDialer{}, func(string, string) (net.Listener, error) { return listener, nil },
		func(string, string) (net.PacketConn, error) { return packet, nil }, nil)
	change, err := manager.Prepare([]config.Rule{udpRule(t, port, "host:22", "both")})
	if err != nil { t.Fatal(err) }
	closeManager(t, manager)
	change.Commit()
	defer func() { listener.Close(); packet.Close(); closeManager(t, manager) }()
	if !listener.isClosed() { t.Fatal("commit after shutdown started a new listener") }
	if _, err := packet.WriteTo([]byte("closed"), packet.LocalAddr()); !errors.Is(err, net.ErrClosed) { t.Fatalf("staged UDP socket not closed: %v", err) }
}
