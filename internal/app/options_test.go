package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseOptionsUsesPrivateMacOSDefaults(t *testing.T) {
	userConfigDir := filepath.Join(t.TempDir(), "Library", "Application Support")
	options, err := ParseOptions(nil, userConfigDir)
	if err != nil {
		t.Fatalf("ParseOptions() error = %v", err)
	}
	if options.DataDir != filepath.Join(userConfigDir, "hello-tsnet") {
		t.Fatalf("DataDir = %q", options.DataDir)
	}
	if options.Hostname != "hello-tsnet" {
		t.Fatalf("options = %#v", options)
	}
}

func TestParseOptionsAcceptsOverridesAndRejectsLegacyListener(t *testing.T) {
	options, err := ParseOptions([]string{"-data-dir", "/tmp/hello", "-hostname", "my-gateway"}, "/unused")
	if err != nil {
		t.Fatalf("ParseOptions() error = %v", err)
	}
	if options.DataDir != "/tmp/hello" || options.Hostname != "my-gateway" {
		t.Fatalf("options = %#v", options)
	}
	if _, err := ParseOptions([]string{"-listen", "127.0.0.1:6060"}, "/tmp"); err == nil {
		t.Fatal("ParseOptions() accepted the retired management listener")
	}
}

func TestPreparePathsCreatesAndRepairsPrivatePermissions(t *testing.T) {
	base := filepath.Join(t.TempDir(), "hello-tsnet")
	state := filepath.Join(base, "tsnet")
	if err := os.MkdirAll(state, 0o777); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Chmod(base, 0o777); err != nil {
		t.Fatalf("Chmod(base) error = %v", err)
	}
	if err := os.Chmod(state, 0o777); err != nil {
		t.Fatalf("Chmod(state) error = %v", err)
	}

	paths, err := PreparePaths(base)
	if err != nil {
		t.Fatalf("PreparePaths() error = %v", err)
	}
	if paths.StateDir != state {
		t.Fatalf("paths = %#v", paths)
	}
	for _, path := range []string{base, state} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat(%q) error = %v", path, err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("%s mode = %o, want 700", path, info.Mode().Perm())
		}
	}
}
