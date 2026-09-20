package desktop

import (
	"context"
	"errors"
	"testing"
	"time"
)

type runnerFunc func(context.Context) error

func (f runnerFunc) Run(ctx context.Context) error { return f(ctx) }

type testShell struct {
	onRun func(func())
	finished chan error
}

func (s *testShell) Run(quit func()) { s.onRun(quit) }
func (s *testShell) Finish(err error) { s.finished <- err }

func TestQuitWaitsForGatewayShutdown(t *testing.T) {
	stopped := make(chan struct{})
	shell := &testShell{finished: make(chan error, 1)}
	shell.onRun = func(quit func()) {
		quit()
		quit()
		select {
		case err := <-shell.finished:
			if err != nil { t.Errorf("Finish: %v", err) }
		case <-time.After(time.Second):
			t.Fatal("quit did not stop gateway")
		}
		select {
		case <-stopped:
		default: t.Error("shell finished before gateway stopped")
		}
	}
	err := Run(context.Background(), runnerFunc(func(ctx context.Context) error {
		<-ctx.Done()
		close(stopped)
		return nil
	}), shell)
	if err != nil { t.Fatal(err) }
}

func TestGatewayFailureReachesShellAndCaller(t *testing.T) {
	want := errors.New("port is occupied")
	shell := &testShell{finished: make(chan error, 1)}
	shell.onRun = func(func()) {
		if err := <-shell.finished; !errors.Is(err, want) { t.Errorf("Finish = %v", err) }
	}
	if err := Run(context.Background(), runnerFunc(func(context.Context) error { return want }), shell); !errors.Is(err, want) {
		t.Fatalf("Run = %v", err)
	}
}

func TestExternalCancellationStopsGateway(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	shell := &testShell{finished: make(chan error, 1)}
	shell.onRun = func(func()) { cancel(); <-shell.finished }
	if err := Run(ctx, runnerFunc(func(ctx context.Context) error { <-ctx.Done(); return nil }), shell); err != nil { t.Fatal(err) }
}
