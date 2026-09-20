package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunRejectsDuplicateBeforeOpeningListeners(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instance.lock")
	first, err := acquireInstance(path)
	if err != nil { t.Fatal(err) }
	defer first.Close()
	application := &App{InstancePath: path}
	// All dependencies are nil: duplicate startup must stop before using them.
	if err := application.Run(context.Background()); err == nil { t.Fatal("duplicate Run succeeded") }
}

func TestInstanceLockRejectsDuplicateAndCanBeReacquired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instance.lock")
	first, err := acquireInstance(path)
	if err != nil { t.Fatal(err) }
	defer first.Close()
	if second, err := acquireInstance(path); err == nil {
		second.Close()
		t.Fatal("second instance acquired the same data directory")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 { t.Fatalf("lock permissions: %v, %v", info, err) }
	if err := first.Close(); err != nil { t.Fatal(err) }
	third, err := acquireInstance(path)
	if err != nil { t.Fatalf("lock was not released: %v", err) }
	third.Close()
}
