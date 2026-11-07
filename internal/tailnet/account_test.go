package tailnet

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"testing"

	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
)

func TestLogoutClearsAccountAndRequiresExplicitLogin(t *testing.T) {
	backend := newFakeBackend()
	runtime := newRuntime(backend)
	if err := runtime.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	close(backend.ready)
	waitForState(t, runtime, StateReady)

	current := &ipnstate.Status{BackendState: "Running", Self: &ipnstate.PeerStatus{UserID: 1},
		TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.1"), netip.MustParseAddr("fd7a:115c:a1e0::1")},
		User: map[tailcfg.UserID]tailcfg.UserProfile{
			1: {DisplayName: "Current User", LoginName: "current@example.com", ProfilePicURL: "https://avatars.example.com/current.png"},
			2: {DisplayName: "Other User", LoginName: "other@example.com", ProfilePicURL: "https://avatars.example.com/other.png"},
		}}
	runtime.applyNetworkStatus(current)
	if got := runtime.Status().Account; got == nil || got.Login != "current@example.com" || got.ProfilePicURL != "https://avatars.example.com/current.png" {
		t.Fatalf("account = %+v", got)
	}
	if local := runtime.Status().LocalDevice; local == nil || !local.Local || local.State != "unknown" ||
		!slices.Equal(local.Addresses, []string{"100.64.0.1", "fd7a:115c:a1e0::1"}) {
		t.Fatalf("local device while control plane is unavailable = %+v", local)
	}
	if err := runtime.Logout(t.Context()); err != nil {
		t.Fatal(err)
	}
	runtime.applyNetworkStatus(current) // A stale running snapshot must not reopen the panel.
	if got := runtime.Status(); got.State != StateSignedOut || got.Account != nil || got.AuthURL != "" || got.LocalDevice != nil {
		t.Fatalf("after logout = %+v", got)
	}
	if _, err := runtime.Login(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Login(t.Context()); err != nil || backend.loginCalls != 2 {
		t.Fatalf("must use interactive login on every click: calls=%d, err=%v", backend.loginCalls, err)
	}
	runtime.applyNetworkStatus(current)
	if got := runtime.Status(); got.State != StateReady || got.Account == nil || got.AuthURL != "" {
		t.Fatalf("after login = %+v", got)
	}
}

func TestLogoutCanRetryAfterLocalDisconnect(t *testing.T) {
	backend := newFakeBackend()
	runtime := newRuntime(backend)
	runtime.status.State = StateReady
	backend.logoutErr = errors.New("control plane timeout")
	if err := runtime.Logout(t.Context()); err == nil {
		t.Fatal("expected logout failure")
	}
	runtime.applyNetworkStatus(&ipnstate.Status{BackendState: "Stopped"})
	backend.logoutErr = nil
	if err := runtime.Logout(t.Context()); err != nil {
		t.Fatalf("retry logout: %v", err)
	}
	if backend.logoutRetries != 1 {
		t.Fatalf("backend logout retries = %d", backend.logoutRetries)
	}
	if got := runtime.Status().State; got != StateSignedOut {
		t.Fatalf("state after retry: %v", got)
	}
}

func (b *fakeBackend) Logout(_ context.Context, retry bool) error {
	if retry {
		b.logoutRetries++
	}
	return b.logoutErr
}
func (b *fakeBackend) Login(context.Context) (string, error) {
	b.loginCalls++
	return "https://login.tailscale.com/a/example", nil
}
