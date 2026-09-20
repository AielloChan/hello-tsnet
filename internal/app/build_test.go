package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"hello-tsnet/internal/tailnet"
	"hello-tsnet/internal/config"
	"hello-tsnet/internal/web"
)

func TestNewCreatesPrivateDataDirectoriesWithoutStartingRuntime(t *testing.T) {
	base := filepath.Join(t.TempDir(), "hello-tsnet")
	application, err := New(Options{
		DataDir:         base,
		WebAddress:      "127.0.0.1:6061",
		Hostname:        "hello-tsnet-test",
		ShutdownTimeout: time.Second,
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

func TestWebStatusMapsTailnetStates(t *testing.T) {
	tests := []struct {
		input tailnet.Status
		want  web.Status
	}{
		{input: tailnet.Status{State: tailnet.StatePending, AuthURL: "https://login.tailscale.com/a/example"}, want: web.Status{State: web.StatePending, AuthURL: "https://login.tailscale.com/a/example"}},
		{input: tailnet.Status{State: tailnet.StateReady}, want: web.Status{State: web.StateReady}},
		{input: tailnet.Status{State: tailnet.StateError, Error: "failed"}, want: web.Status{State: web.StateError, Error: "failed"}},
	}
	for _, tt := range tests {
		if got := webStatus(tt.input); got != tt.want {
			t.Fatalf("webStatus(%#v) = %#v, want %#v", tt.input, got, tt.want)
		}
	}
}

func TestSharingCannotExposeManagementPort(t *testing.T) {
	for _, port := range []int{6060, 6061} {
		preparer := managerPreparer{managementPort: port}
		_, err := preparer.Prepare([]config.Rule{{ID:"admin", Name:"admin", LocalPort:port, Protocol:config.ProtocolBoth, Direction:config.DirectionInbound}})
		if err == nil { t.Fatal("management port sharing accepted") }
	}
}
