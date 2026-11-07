package tailnet

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"sync"
	"time"

	"hello-tsnet/internal/direct"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tsnet"
)

type State string

const (
	StatePending   State = "pending"
	StateReady     State = "ready"
	StateError     State = "error"
	StateSignedOut State = "signedOut"
)

type Status struct {
	State          State    `json:"state"`
	AuthURL        string   `json:"authUrl,omitempty"`
	Error          string   `json:"error,omitempty"`
	Access         string   `json:"access,omitempty"`
	RoutesReady    bool     `json:"-"`
	Account        *Account `json:"account,omitempty"`
	LocalAddresses []string `json:"localAddresses,omitempty"`
}

type backend interface {
	NetworkStatus(context.Context) (*ipnstate.Status, error)
	Ping(context.Context, netip.Addr) (time.Duration, error)
	Start() error
	WaitReady(context.Context) error
	Login(context.Context) (string, error)
	Logout(context.Context, bool) error
	Close() error
}

type Runtime struct {
	backend backend

	mu          sync.RWMutex
	status      Status
	started     bool
	available   bool
	accountMu   sync.Mutex
	logoutRetry bool
	cancel      context.CancelFunc
	done        chan struct{}
}

type tsnetBackend struct {
	server *tsnet.Server
	device *direct.Device
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
	ctx, r.cancel = context.WithCancel(ctx)
	r.done = make(chan struct{})
	r.mu.Unlock()

	if err := r.backend.Start(); err != nil {
		r.setError(err)
		close(r.done)
		return err
	}
	r.mu.Lock()
	r.available = true
	r.mu.Unlock()
	go func() {
		defer close(r.done)
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
		if b, ok := r.backend.(*tsnetBackend); ok {
			b.runRoutes(ctx, r.applyNetworkStatus, func(access string, ready bool) {
				r.mu.Lock()
				if r.status.State == StateReady {
					r.status.Access = access
					r.status.RoutesReady = ready
				}
				r.mu.Unlock()
			})
		}
	}()
	return nil
}

func (r *Runtime) Status() Status {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.status
}

func (r *Runtime) Close() error {
	r.mu.RLock()
	cancel, done := r.cancel, r.done
	r.mu.RUnlock()
	if cancel != nil {
		cancel()
		<-done
	}
	return r.backend.Close()
}

func (r *Runtime) userLogf(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	match := loginURLPattern.FindString(message)
	if match == "" {
		return
	}
	match = strings.TrimRight(match, ".,);]}")
	link := officialLoginURL(match)
	if link == "" {
		return
	}
	r.mu.Lock()
	if r.status.State == StatePending {
		r.status.AuthURL = link
	}
	r.mu.Unlock()
}

func (r *Runtime) setError(err error) {
	r.mu.Lock()
	r.status = Status{State: StateError, Error: err.Error()}
	r.mu.Unlock()
}

func (b *tsnetBackend) Start() error {
	device, err := direct.NewDevice()
	if err != nil {
		return err
	}
	b.device = device
	b.server.Tun = device
	return b.server.Start()
}

func (b *tsnetBackend) WaitReady(ctx context.Context) error {
	_, err := b.server.Up(ctx)
	return err
}

func (b *tsnetBackend) Close() error {
	if b.device != nil {
		_ = b.device.Close()
	}
	return b.server.Close()
}
