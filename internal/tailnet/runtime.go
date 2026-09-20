package tailnet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tsnet"
)

type State string

const (
	StatePending State = "pending"
	StateReady   State = "ready"
	StateError   State = "error"
)

type Status struct {
	State   State  `json:"state"`
	AuthURL string `json:"authUrl,omitempty"`
	Error   string `json:"error,omitempty"`
}

type backend interface {
	NetworkStatus(context.Context) (*ipnstate.Status, error)
	Start() error
	WaitReady(context.Context) error
	Dial(context.Context, string, string) (net.Conn, error)
	Listen(string, string) (net.Listener, error)
	Close() error
}

type Runtime struct {
	backend backend

	mu      sync.RWMutex
	status  Status
	started bool
}

type tsnetBackend struct {
	server *tsnet.Server
}

var loginURLPattern = regexp.MustCompile(`https://login\.tailscale\.com/[^\s]+`)

func New(dir, hostname string) *Runtime {
	runtime := &Runtime{status: Status{State: StatePending}}
	server := &tsnet.Server{
		Dir:      dir,
		Hostname: hostname,
		UserLogf: runtime.userLogf,
	}
	runtime.backend = &tsnetBackend{server: server}
	return runtime
}

func newRuntime(backend backend) *Runtime {
	return &Runtime{backend: backend, status: Status{State: StatePending}}
}

func (r *Runtime) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return errors.New("tailnet runtime already started")
	}
	r.started = true
	r.status.State = StatePending
	r.mu.Unlock()

	if err := r.backend.Start(); err != nil {
		r.setError(err)
		return err
	}
	go func() {
		err := r.backend.WaitReady(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			r.setError(err)
			return
		}
		r.mu.Lock()
		r.status = Status{State: StateReady}
		r.mu.Unlock()
	}()
	return nil
}

func (r *Runtime) Status() Status {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.status
}

func (r *Runtime) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	return r.backend.Dial(ctx, network, address)
}

func (r *Runtime) Close() error {
	return r.backend.Close()
}

func (r *Runtime) userLogf(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	match := loginURLPattern.FindString(message)
	if match == "" {
		return
	}
	match = strings.TrimRight(match, ".,);]}")
	parsed, err := url.Parse(match)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() != "login.tailscale.com" {
		return
	}
	r.mu.Lock()
	if r.status.State == StatePending {
		r.status.AuthURL = parsed.String()
	}
	r.mu.Unlock()
}

func (r *Runtime) setError(err error) {
	r.mu.Lock()
	r.status = Status{State: StateError, Error: err.Error()}
	r.mu.Unlock()
}

func (b *tsnetBackend) Start() error {
	return b.server.Start()
}

func (b *tsnetBackend) WaitReady(ctx context.Context) error {
	_, err := b.server.Up(ctx)
	return err
}

func (b *tsnetBackend) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	return b.server.Dial(ctx, network, address)
}

func (b *tsnetBackend) Close() error {
	return b.server.Close()
}

// Listen registers a node-local TCP or UDP listener without waiting for login.
// tsnet exposes incoming UDP flows as accepted datagram-preserving connections.
func (r *Runtime) Listen(network, address string) (net.Listener, error) {
	return r.backend.Listen(network, address)
}

func (b *tsnetBackend) Listen(network, address string) (net.Listener, error) {
	return b.server.Listen(network, address)
}
