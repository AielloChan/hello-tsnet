package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"hello-tsnet/internal/config"
)

type Loader interface {
	Load() ([]config.Rule, error)
}

type Restorer interface {
	Restore([]config.Rule) error
}

type Runtime interface {
	Start(context.Context) error
	Close() error
}

type Manager interface {
	Close(context.Context)
}

type HTTPServer interface {
	Start(net.Listener) <-chan error
	Shutdown(context.Context) error
}

type App struct {
	InstancePath    string
	Address         string
	Listen          func(network, address string) (net.Listener, error)
	Loader          Loader
	Restorer        Restorer
	Runtime         Runtime
	Manager         Manager
	Server          HTTPServer
	ShutdownTimeout time.Duration
}

func (a *App) Run(ctx context.Context) error {
	if a.InstancePath != "" {
		lock, err := acquireInstance(a.InstancePath)
		if err != nil {
			return err
		}
		defer lock.Close()
	}
	listener, err := a.Listen("tcp", a.Address)
	if err != nil {
		return fmt.Errorf("listen for web UI: %w", err)
	}
	rules, err := a.Loader.Load()
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("load rules: %w", err)
	}
	if err := a.Restorer.Restore(rules); err != nil {
		_ = listener.Close()
		// Restoring a shared listener may have already started tsnet.
		timeout := a.ShutdownTimeout
		if timeout <= 0 { timeout = defaultShutdownTimeout }
		cleanupCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		a.Manager.Close(cleanupCtx)
		_ = a.Runtime.Close()
		return fmt.Errorf("restore rules: %w", err)
	}

	serverDone := a.Server.Start(listener)
	_ = a.Runtime.Start(ctx)

	var runErr error
	select {
	case <-ctx.Done():
	case err := <-serverDone:
		if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			runErr = fmt.Errorf("serve web UI: %w", err)
		}
	}

	timeout := a.ShutdownTimeout
	if timeout <= 0 {
		timeout = defaultShutdownTimeout
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	shutdownErr := a.Server.Shutdown(shutdownCtx)
	listenerErr := listener.Close()
	a.Manager.Close(shutdownCtx)
	runtimeErr := a.Runtime.Close()
	if errors.Is(listenerErr, net.ErrClosed) {
		listenerErr = nil
	}
	return errors.Join(runErr, shutdownErr, listenerErr, runtimeErr)
}

type HTTPServerAdapter struct {
	Server *http.Server
}

func (s *HTTPServerAdapter) Start(listener net.Listener) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- s.Server.Serve(listener)
		close(done)
	}()
	return done
}

func (s *HTTPServerAdapter) Shutdown(ctx context.Context) error {
	return s.Server.Shutdown(ctx)
}
