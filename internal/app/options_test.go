package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseOptionsUsesPrivateMacOSDefaults(t *testing.T) {
	options, err := ParseOptions(nil, "/example/Library/Application Support")
	if err != nil {
		t.Fatalf("ParseOptions() error = %v", err)
	}
	if options.DataDir != "/example/Library/Application Support/hello-tsnet" {
		t.Fatalf("DataDir = %q", options.DataDir)
	}
	if options.WebAddress != "127.0.0.1:6060" || options.Hostname != "hello-tsnet" {
		t.Fatalf("options = %#v", options)
	}
}

func TestParseOptionsAcceptsOverridesAndRejectsNonLoopback(t *testing.T) {
	options, err := ParseOptions([]string{"-data-dir", "/tmp/hello", "-listen", "127.0.0.1:7070", "-hostname", "my-gateway"}, "/unused")
	if err != nil {
		t.Fatalf("ParseOptions() error = %v", err)
	}
	if options.DataDir != "/tmp/hello" || options.WebAddress != "127.0.0.1:7070" || options.Hostname != "my-gateway" {
		t.Fatalf("options = %#v", options)
	}
	if _, err := ParseOptions([]string{"-listen", "0.0.0.0:6060"}, "/tmp"); err == nil {
		t.Fatal("ParseOptions() accepted a non-loopback address")
	}
}

func TestPreparePathsCreatesAndRepairsPrivatePermissions(t *testing.T) {
	base := filepath.Join(t.TempDir(), "hello-tsnet")
	state := filepath.Join(base, "tsnet")
	if err := os.MkdirAll(state, 0o777); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	rulesPath := filepath.Join(base, "rules.json")
	if err := os.WriteFile(rulesPath, []byte(`{"version":1,"rules":[]}`), 0o666); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chmod(base, 0o777); err != nil {
		t.Fatalf("Chmod(base) error = %v", err)
	}
	if err := os.Chmod(state, 0o777); err != nil {
		t.Fatalf("Chmod(state) error = %v", err)
	}
	if err := os.Chmod(rulesPath, 0o666); err != nil {
		t.Fatalf("Chmod(rules) error = %v", err)
	}

	paths, err := PreparePaths(base)
	if err != nil {
		t.Fatalf("PreparePaths() error = %v", err)
	}
	if paths.StateDir != state || paths.RulesFile != rulesPath {
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
	info, err := os.Stat(rulesPath)
	if err != nil {
		t.Fatalf("Stat(rules) error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("rules mode = %o, want 600", info.Mode().Perm())
	}
}
