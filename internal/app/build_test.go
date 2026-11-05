package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewCreatesPrivateDataDirectoriesWithoutStartingRuntime(t *testing.T) {
	base := filepath.Join(t.TempDir(), "hello-tsnet")
	application, err := New(Options{
		DataDir:  base,
		Hostname: "hello-tsnet-test",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if application == nil {
		t.Fatal("New() returned nil application")
	}
	for _, path := range []string{base, filepath.Join(base, "tsnet")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat(%q) error = %v", path, err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("%s mode = %o, want 700", path, info.Mode().Perm())
		}
	}
}
