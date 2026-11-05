package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunIgnoresLegacyRules(t *testing.T) {
	base := t.TempDir()
	legacy := filepath.Join(base, "rules.json")
	if err := os.WriteFile(legacy, []byte("legacy content"), 0600); err != nil {
		t.Fatal(err)
	}
	application, err := New(Options{DataDir: base, Hostname: "test"})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &fakeRuntime{started: make(chan struct{}), closed: make(chan struct{})}
	application.Runtime = runtime
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- application.Run(ctx) }()
	select {
	case err := <-done:
		t.Fatalf("gateway failed to start: %v", err)
	case <-runtime.started:
	case <-time.After(2 * time.Second):
		t.Fatal("gateway did not start")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	<-runtime.closed
	if content, err := os.ReadFile(legacy); err != nil || string(content) != "legacy content" {
		t.Fatalf("legacy rules changed: %q, %v", content, err)
	}
}

type fakeRuntime struct {
	started chan struct{}
	closed  chan struct{}
}

func (r *fakeRuntime) Start(context.Context) error {
	close(r.started)
	return nil
}

func (r *fakeRuntime) Close() error {
	close(r.closed)
	return nil
}
