package forward

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"hello-tsnet/internal/config"
)

type stubDialer struct{}

func (stubDialer) Dial(context.Context, string, string) (net.Conn, error) {
	return nil, errors.New("not connected")
}

type blockingDialer struct{}

func (blockingDialer) Dial(ctx context.Context, _, _ string) (net.Conn, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type fakeListener struct {
	mu      sync.Mutex
	closed  bool
	started chan struct{}
	once    sync.Once
}

func newFakeListener() *fakeListener {
	return &fakeListener{started: make(chan struct{})}
}

func (l *fakeListener) Accept() (net.Conn, error) {
	l.once.Do(func() { close(l.started) })
	for {
		l.mu.Lock()
		closed := l.closed
		l.mu.Unlock()
		if closed {
			return nil, net.ErrClosed
		}
		time.Sleep(time.Millisecond)
	}
}

func (l *fakeListener) Close() error {
	l.mu.Lock()
	l.closed = true
	l.mu.Unlock()
	return nil
}

func (l *fakeListener) Addr() net.Addr { return fakeAddr("127.0.0.1:0") }

func (l *fakeListener) isClosed() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closed
}

type fakeAddr string

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return string(a) }

func TestPrepareDoesNotAcceptUntilCommit(t *testing.T) {
	listener := newFakeListener()
	manager := NewManager(stubDialer{}, func(network, address string) (net.Listener, error) {
		if network != "tcp" || address != "127.0.0.1:30001" {
			t.Fatalf("listen(%q, %q)", network, address)
		}
		return listener, nil
	})
	change, err := manager.Prepare([]config.Rule{testRule("one", 30001, "host:22")})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	select {
	case <-listener.started:
		t.Fatal("listener accepted before commit")
	case <-time.After(20 * time.Millisecond):
	}

	change.Commit()
	select {
	case <-listener.started:
	case <-time.After(time.Second):
		t.Fatal("listener did not accept after commit")
	}
}

func TestAbortClosesPreparedListeners(t *testing.T) {
	listener := newFakeListener()
	manager := NewManager(stubDialer{}, func(string, string) (net.Listener, error) {
		return listener, nil
	})
	change, err := manager.Prepare([]config.Rule{testRule("one", 30001, "host:22")})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	change.Abort()
	if !listener.isClosed() {
		t.Fatal("Abort() did not close prepared listener")
	}
	if got := manager.Rules(); len(got) != 0 {
		t.Fatalf("Rules() = %#v after abort", got)
	}
}

func TestPrepareFailureClosesAllStagedListenersAndPreservesRules(t *testing.T) {
	initial := newFakeListener()
	staged := newFakeListener()
	listenCalls := 0
	manager := NewManager(stubDialer{}, func(string, string) (net.Listener, error) {
		listenCalls++
		switch listenCalls {
		case 1:
			return initial, nil
		case 2:
			return staged, nil
		default:
			return nil, errors.New("port unavailable")
		}
	})
	first, err := manager.Prepare([]config.Rule{testRule("one", 30001, "host:22")})
	if err != nil {
		t.Fatalf("initial Prepare() error = %v", err)
	}
	first.Commit()

	_, err = manager.Prepare([]config.Rule{
		testRule("one", 30001, "host:22"),
		testRule("two", 30002, "host:80"),
		testRule("three", 30003, "host:443"),
	})
	if err == nil {
		t.Fatal("Prepare() error = nil, want listener failure")
	}
	if !staged.isClosed() {
		t.Fatal("failed Prepare() left staged listener open")
	}
	if initial.isClosed() {
		t.Fatal("failed Prepare() closed active listener")
	}
	got := manager.Rules()
	if len(got) != 1 || got[0].ID != "one" {
		t.Fatalf("Rules() = %#v, want original rule", got)
	}
}

func TestCommitReusesPortsAndClosesRemovedListeners(t *testing.T) {
	firstListener := newFakeListener()
	secondListener := newFakeListener()
	listenCalls := 0
	manager := NewManager(stubDialer{}, func(string, string) (net.Listener, error) {
		listenCalls++
		if listenCalls == 1 {
			return firstListener, nil
		}
		return secondListener, nil
	})
	change, err := manager.Prepare([]config.Rule{testRule("one", 30001, "host:22")})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	change.Commit()

	updated, err := manager.Prepare([]config.Rule{testRule("one", 30001, "other:2222")})
	if err != nil {
		t.Fatalf("update Prepare() error = %v", err)
	}
	updated.Commit()
	if listenCalls != 1 {
		t.Fatalf("target-only update created a listener; calls = %d", listenCalls)
	}
	if got := manager.Rules()[0].Target; got != "other:2222" {
		t.Fatalf("updated target = %q", got)
	}

	removed, err := manager.Prepare(nil)
	if err != nil {
		t.Fatalf("remove Prepare() error = %v", err)
	}
	removed.Commit()
	if !firstListener.isClosed() {
		t.Fatal("removed listener is still open")
	}
}

func TestDialIsBoundedPerConnection(t *testing.T) {
	listener := newFakeListener()
	manager := NewManager(blockingDialer{}, func(string, string) (net.Listener, error) {
		return listener, nil
	})
	manager.dialTimeout = 20 * time.Millisecond
	change, err := manager.Prepare([]config.Rule{testRule("one", 30001, "host:22")})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	change.Commit()

	client, server := net.Pipe()
	defer client.Close()
	done := make(chan struct{})
	go func() {
		manager.handleConn(30001, server)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handleConn() did not stop after the dial timeout")
	}
}

func testRule(id string, port int, target string) config.Rule {
	return config.Rule{ID: id, Name: id, LocalPort: port, Target: target}
}
