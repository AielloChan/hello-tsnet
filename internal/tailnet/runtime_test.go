package tailnet

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeBackend struct {
	startErr error
	readyErr error
	ready    chan struct{}
	started  chan struct{}
	closed   bool
	mu       sync.Mutex
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{ready: make(chan struct{}), started: make(chan struct{})}
}

func (b *fakeBackend) Start() error {
	select {
	case <-b.started:
	default:
		close(b.started)
	}
	return b.startErr
}

func (b *fakeBackend) WaitReady(ctx context.Context) error {
	select {
	case <-b.ready:
		return b.readyErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *fakeBackend) Dial(context.Context, string, string) (net.Conn, error) {
	return nil, errors.New("dial called")
}

func (b *fakeBackend) Close() error {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	return nil
}

func TestUserLogCapturesOnlyOfficialLoginURL(t *testing.T) {
	runtime := newRuntime(newFakeBackend())
	runtime.userLogf("visit %s", "https://evil.example/a/token")
	if got := runtime.Status().AuthURL; got != "" {
		t.Fatalf("captured untrusted URL %q", got)
	}

	want := "https://login.tailscale.com/a/example-token"
	runtime.userLogf("To start this tsnet server, go to: %s", want)
	if got := runtime.Status().AuthURL; got != want {
		t.Fatalf("AuthURL = %q, want %q", got, want)
	}
}

func TestStartTransitionsFromPendingToReadyAndClearsURL(t *testing.T) {
	backend := newFakeBackend()
	runtime := newRuntime(backend)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := runtime.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	runtime.userLogf("go to: %s", "https://login.tailscale.com/a/example")
	pending := runtime.Status()
	if pending.State != StatePending || pending.AuthURL == "" {
		t.Fatalf("pending status = %#v", pending)
	}

	close(backend.ready)
	waitForState(t, runtime, StateReady)
	ready := runtime.Status()
	if ready.AuthURL != "" || ready.Error != "" {
		t.Fatalf("ready status = %#v", ready)
	}
}

func TestStartFailureSetsErrorStatus(t *testing.T) {
	backend := newFakeBackend()
	backend.startErr = errors.New("cannot initialize")
	runtime := newRuntime(backend)

	err := runtime.Start(context.Background())
	if err == nil {
		t.Fatal("Start() error = nil")
	}
	status := runtime.Status()
	if status.State != StateError || !strings.Contains(status.Error, "cannot initialize") {
		t.Fatalf("status = %#v", status)
	}
}

func TestReadyFailureSetsErrorUnlessContextWasCancelled(t *testing.T) {
	backend := newFakeBackend()
	backend.readyErr = errors.New("control unavailable")
	runtime := newRuntime(backend)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := runtime.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	close(backend.ready)
	waitForState(t, runtime, StateError)
	if !strings.Contains(runtime.Status().Error, "control unavailable") {
		t.Fatalf("status = %#v", runtime.Status())
	}
}

func TestStatusJSONContainsNoAuthKey(t *testing.T) {
	runtime := newRuntime(newFakeBackend())
	runtime.userLogf("go to: %s", "https://login.tailscale.com/a/example")
	encoded, err := json.Marshal(runtime.Status())
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if strings.Contains(strings.ToLower(string(encoded)), "authkey") || strings.Contains(string(encoded), "tskey-") {
		t.Fatalf("status leaked an auth key: %s", encoded)
	}
}

func waitForState(t *testing.T, runtime *Runtime, want State) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if runtime.Status().State == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("state = %q, want %q", runtime.Status().State, want)
}
