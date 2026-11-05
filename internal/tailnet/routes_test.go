package tailnet

import (
	"net/netip"
	"slices"
	"testing"

	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/key"
)

func TestRouteSnapshotUsesAssignedAddressesFromVisibleDevices(t *testing.T) {
	self := netip.MustParseAddr("100.64.0.1")
	self6 := netip.MustParseAddr("fd7a:115c:a1e0::1")
	peer := netip.MustParseAddr("100.64.0.2")
	peer6 := netip.MustParseAddr("fd7a:115c:a1e0::2")
	subnet := netip.MustParseAddr("192.168.1.2")
	status := &ipnstate.Status{
		BackendState: "Running",
		TailscaleIPs: []netip.Addr{self6, self},
		Self:         &ipnstate.PeerStatus{Online: true},
		Peer: map[key.NodePublic]*ipnstate.PeerStatus{
			key.NewNode().Public(): {InNetworkMap: true, DNSName: "Mac-Mini.tailtest.ts.net.", TailscaleIPs: []netip.Addr{peer, peer6, subnet}},
			key.NewNode().Public(): {InNetworkMap: true, Expired: true, TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.3")}},
			key.NewNode().Public(): {TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.4")}},
		},
	}
	snapshot, err := routeSnapshot(status)
	if err != nil || !slices.Equal(snapshot.Local, []netip.Addr{self, self6}) ||
		!slices.Equal(snapshot.Peers, []netip.Addr{peer, peer6}) {
		t.Fatalf("snapshot = %+v, %v", snapshot, err)
	}
	if len(snapshot.Hosts) != 2 || snapshot.Hosts[0].Name != "mac-mini.tailtest.ts.net" ||
		snapshot.Hosts[0].Address != peer || snapshot.Hosts[1].Address != peer6 {
		t.Fatalf("hosts = %+v", snapshot.Hosts)
	}
	status.BackendState = "NeedsLogin"
	snapshot, err = routeSnapshot(status)
	if err != nil || len(snapshot.Local) > 0 || len(snapshot.Peers) > 0 {
		t.Fatalf("logged out snapshot = %+v, %v", snapshot, err)
	}
}
