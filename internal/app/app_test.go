package app

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"hello-tsnet/internal/config"
)

type eventLog struct {
	mu     sync.Mutex
	events []string
}

func (l *eventLog) add(event string) {
	l.mu.Lock()
	l.events = append(l.events, event)
	l.mu.Unlock()
}

func (l *eventLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...)
}

type fakeLoader struct {
	log   *eventLog
	rules []config.Rule
	err   error
}

func (l *fakeLoader) Load() ([]config.Rule, error) {
	l.log.add("load")
	return append([]config.Rule(nil), l.rules...), l.err
}

type fakeRestorer struct {
	log    *eventLog
	err    error
	called bool
}

func (r *fakeRestorer) Restore([]config.Rule) error {
	r.log.add("restore")
	r.called = true
	return r.err
}

type fakeRuntime struct {
	log      *eventLog
	startErr error
	started  chan struct{}
}

func (r *fakeRuntime) Start(context.Context) error {
	r.log.add("runtime-start")
	close(r.started)
	return r.startErr
}

func (r *fakeRuntime) Close() error {
	r.log.add("runtime-close")
	return nil
}

type fakeManager struct {
	log      *eventLog
	hadLimit bool
}

func (m *fakeManager) Close(ctx context.Context) {
	m.log.add("manager-close")
	_, m.hadLimit = ctx.Deadline()
}

type fakeServer struct {
	log      *eventLog
	stopped  chan struct{}
	serveErr error
}

func (s *fakeServer) Start(net.Listener) <-chan error {
	s.log.add("http-serve")
	done := make(chan error, 1)
	go func() {
		<-s.stopped
		done <- s.serveErr
		close(done)
	}()
	return done
}

func (s *fakeServer) Shutdown(context.Context) error {
	s.log.add("http-shutdown")
	close(s.stopped)
	return nil
}

type fakeListener struct {
	log    *eventLog
	closed bool
}

func (l *fakeListener) Accept() (net.Conn, error) { return nil, net.ErrClosed }
func (l *fakeListener) Addr() net.Addr            { return fakeAddress("127.0.0.1:6060") }
func (l *fakeListener) Close() error {
	if !l.closed {
		l.log.add("listener-close")
		l.closed = true
	}
	return nil
}

type fakeAddress string

func (a fakeAddress) Network() string { return "tcp" }
func (a fakeAddress) String() string  { return string(a) }

func TestRunUsesRecoveryStartupAndBoundedShutdownOrder(t *testing.T) {
	log := &eventLog{}
	listener := &fakeListener{log: log}
	runtime := &fakeRuntime{log: log, started: make(chan struct{})}
	manager := &fakeManager{log: log}
	server := &fakeServer{log: log, stopped: make(chan struct{}), serveErr: net.ErrClosed}
	app := &App{
		Address: "127.0.0.1:6060",
		Listen: func(network, address string) (net.Listener, error) {
			log.add("listen")
			return listener, nil
		},
		Loader:          &fakeLoader{log: log},
		Restorer:        &fakeRestorer{log: log},
		Runtime:         runtime,
		Manager:         manager,
		Server:          server,
		ShutdownTimeout: 5 * time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()
	<-runtime.started
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	want := []string{"listen", "load", "restore", "http-serve", "runtime-start", "http-shutdown", "listener-close", "manager-close", "runtime-close"}
	got := log.snapshot()
	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events = %v, want %v", got, want)
		}
	}
	if !manager.hadLimit {
		t.Fatal("manager Close() did not receive a deadline")
	}
}

func TestRunLoadFailurePreservesFileAndStopsBeforeRestore(t *testing.T) {
	log := &eventLog{}
	listener := &fakeListener{log: log}
	restorer := &fakeRestorer{log: log}
	runtime := &fakeRuntime{log: log, started: make(chan struct{})}
	app := &App{
		Address: "127.0.0.1:6060",
		Listen: func(string, string) (net.Listener, error) {
			log.add("listen")
			return listener, nil
		},
		Loader:          &fakeLoader{log: log, err: errors.New("corrupt rules")},
		Restorer:        restorer,
		Runtime:         runtime,
		Manager:         &fakeManager{log: log},
		Server:          &fakeServer{log: log, stopped: make(chan struct{})},
		ShutdownTimeout: 5 * time.Second,
	}

	if err := app.Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil")
	}
	if restorer.called {
		t.Fatal("Restore() called after Load() failed")
	}
	select {
	case <-runtime.started:
		t.Fatal("runtime started after Load() failed")
	default:
	}
	if got := log.snapshot(); len(got) != 3 || got[0] != "listen" || got[1] != "load" || got[2] != "listener-close" {
		t.Fatalf("events = %v", got)
	}
}

func TestRunRuntimeStartFailureKeepsWebRunningUntilCancelled(t *testing.T) {
	log := &eventLog{}
	runtime := &fakeRuntime{log: log, startErr: errors.New("tsnet failed"), started: make(chan struct{})}
	server := &fakeServer{log: log, stopped: make(chan struct{}), serveErr: net.ErrClosed}
	app := &App{
		Address: "127.0.0.1:6060",
		Listen: func(string, string) (net.Listener, error) {
			return &fakeListener{log: log}, nil
		},
		Loader:          &fakeLoader{log: log},
		Restorer:        &fakeRestorer{log: log},
		Runtime:         runtime,
		Manager:         &fakeManager{log: log},
		Server:          server,
		ShutdownTimeout: 5 * time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()
	<-runtime.started
	select {
	case err := <-done:
		t.Fatalf("Run() returned after runtime Start failure: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run() shutdown error = %v", err)
	}
}
