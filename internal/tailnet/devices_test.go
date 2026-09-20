package tailnet

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/key"
)

func TestDevicesOnlyExposeVisiblePeersAndPublicDisplayFields(t *testing.T) {
	seen := time.Date(2026, 9, 20, 1, 0, 0, 0, time.UTC)
	backend := newFakeBackend()
	backend.networkStatus = &ipnstate.Status{
		BackendState: "Running", AuthURL: "secret-auth-url",
		Self: &ipnstate.PeerStatus{Online: true, HostName: "this-node"},
		Peer: map[key.NodePublic]*ipnstate.PeerStatus{
			key.NewNode().Public(): {ID: "offline", HostName: "nas", TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.2")}, LastSeen: seen, InNetworkMap: true},
			key.NewNode().Public(): {ID: "online", HostName: "Mac mini", DNSName: "mac-mini.example.ts.net.", OS: "macOS", Online: true, InNetworkMap: true, TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.1")}, Addrs: []string{"private-endpoint"}},
			key.NewNode().Public(): {ID: "stale-engine-only", Online: true},
		},
	}
	runtime := newRuntime(backend)
	runtime.status.State = StateReady
	devices, err := runtime.Devices(context.Background())
	if err != nil { t.Fatal(err) }
	if len(devices) != 2 || devices[0].ID != "online" || devices[0].State != "online" || devices[0].DNSName != "mac-mini.example.ts.net" {
		t.Fatalf("devices = %#v", devices)
	}
	if devices[1].State != "offline" || devices[1].LastSeen == nil || !devices[1].LastSeen.Equal(seen) {
		t.Fatalf("offline device = %#v", devices[1])
	}
	encoded, err := json.Marshal(devices)
	if err != nil { t.Fatal(err) }
	for _, secret := range []string{"secret-auth-url", "private-endpoint", "PublicKey", "this-node"} {
		if strings.Contains(string(encoded), secret) { t.Fatalf("device response leaked %s", secret) }
	}
}

func TestDevicesUnavailableDoesNotPretendTheNetworkIsEmpty(t *testing.T) {
	backend := newFakeBackend()
	runtime := newRuntime(backend)
	if _, err := runtime.Devices(context.Background()); !errors.Is(err, ErrDevicesUnavailable) { t.Fatalf("pending error = %v", err) }
	if backend.statusCalls != 0 { t.Fatal("pending lookup implicitly started LocalClient") }
	runtime.status.State = StateReady
	for _, status := range []*ipnstate.Status{nil, {BackendState: "NeedsLogin"}, {BackendState: "Running", Self: &ipnstate.PeerStatus{Online: false}}} {
		backend.networkStatus = status
		if _, err := runtime.Devices(context.Background()); !errors.Is(err, ErrDevicesUnavailable) { t.Fatalf("unavailable error = %v", err) }
	}
	backend.networkStatus = &ipnstate.Status{BackendState: "Running", Self: &ipnstate.PeerStatus{Online: true}}
	devices, err := runtime.Devices(context.Background())
	if err != nil || devices == nil || len(devices) != 0 { t.Fatalf("known empty = %#v, %v", devices, err) }
	backend.statusErr = errors.New("internal credential detail")
	if _, err := runtime.Devices(context.Background()); !errors.Is(err, ErrDevicesUnavailable) || strings.Contains(err.Error(), "credential") { t.Fatalf("backend error = %v", err) }
}

func TestDevicesRetainIPv6AndExpiredState(t *testing.T) {
	backend := newFakeBackend()
	backend.networkStatus = &ipnstate.Status{BackendState: "Running", Self: &ipnstate.PeerStatus{Online: true}, Peer: map[key.NodePublic]*ipnstate.PeerStatus{
		key.NewNode().Public(): {ID: "ipv6", InNetworkMap: true, Expired: true, Online: true, TailscaleIPs: []netip.Addr{netip.MustParseAddr("fd7a:115c:a1e0::1")}},
	}}
	runtime := newRuntime(backend)
	runtime.status.State = StateReady
	devices, err := runtime.Devices(context.Background())
	if err != nil { t.Fatal(err) }
	if len(devices) != 1 || devices[0].State != "expired" || devices[0].Addresses[0] != "fd7a:115c:a1e0::1" { t.Fatalf("device = %#v", devices) }
}

func TestDevicesDoNotLabelMissingOnlineEvidenceAsOffline(t *testing.T) {
	backend := newFakeBackend()
	backend.networkStatus = &ipnstate.Status{BackendState: "Running", Self: &ipnstate.PeerStatus{Online: true}, Peer: map[key.NodePublic]*ipnstate.PeerStatus{
		key.NewNode().Public(): {ID: "unknown", InNetworkMap: true},
	}}
	runtime := newRuntime(backend)
	runtime.status.State = StateReady
	devices, err := runtime.Devices(context.Background())
	if err != nil { t.Fatal(err) }
	if devices[0].State != "unknown" { t.Fatalf("state without online/last-seen evidence = %q", devices[0].State) }
}
