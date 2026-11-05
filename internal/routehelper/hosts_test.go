package routehelper

import (
	"net/netip"
	"strings"
	"testing"
)

func TestManagedHostsPreservesExistingEntriesAndCleansUp(t *testing.T) {
	base := "# custom\n127.0.0.1 localhost\n192.0.2.10 Nas.Local. # user entry\n"
	v4 := netip.MustParseAddr("100.64.0.2")
	v6 := netip.MustParseAddr("fd7a:115c:a1e0::2")
	entries := []HostEntry{
		{Name: "nas.tailtest.ts.net", Address: v4},
		{Name: "mac.tailtest.ts.net", Address: v4},
		{Name: "mac.tailtest.ts.net", Address: v6},
		{Name: "mac.other.ts.net", Address: v4},
		{Name: "unrouted.tailtest.ts.net", Address: netip.MustParseAddr("100.64.0.3")},
	}
	routes := map[netip.Addr]bool{v4: true, v6: true}
	got, err := managedHosts(base, entries, routes)
	want := base + hostsBegin + "\n" +
		"100.64.0.2\tmac.other.ts.net\n" +
		"100.64.0.2\tmac.tailtest.ts.net\n" +
		"fd7a:115c:a1e0::2\tmac.tailtest.ts.net\n" +
		"100.64.0.2\tnas.tailtest.ts.net\n" + hostsEnd + "\n"
	if err != nil || got != want {
		t.Fatalf("managed hosts = %q, %v", got, err)
	}
	previous := base + hostsBegin + "\nfd7a:115c:a1e0::2\tdesk.tailtest.ts.net desk\n" + hostsEnd + "\n"
	updated, err := managedHosts(previous, []HostEntry{{Name: "desk.tailtest.ts.net", Address: v6}}, routes)
	if err != nil || updated != base+hostsBegin+"\nfd7a:115c:a1e0::2\tdesk.tailtest.ts.net desk.local\n"+hostsEnd+"\n" {
		t.Fatalf("replacement = %q, %v", updated, err)
	}
	cleared, err := managedHosts(updated, nil, nil)
	if err != nil || cleared != base {
		t.Fatalf("cleanup = %q, %v", cleared, err)
	}
	unterminated := strings.TrimSuffix(base, "\n")
	withBlock, err := managedHosts(unterminated, entries, routes)
	if err != nil {
		t.Fatal(err)
	}
	cleared, err = managedHosts(withBlock, nil, nil)
	if err != nil || cleared != unterminated {
		t.Fatalf("cleanup of unterminated input = %q, %v", cleared, err)
	}
}

func TestManagedHostsRejectsMalformedBlockAndDisallowsInjectedNames(t *testing.T) {
	address := netip.MustParseAddr("100.64.0.2")
	for _, invalid := range []string{"google.com", "x.ts.net", "hi.tailtest.ts.net\n127.0.0.1", "bad name.tailtest.ts.net"} {
		if validHostName(invalid) {
			t.Fatalf("allowed %q", invalid)
		}
	}
	if _, err := managedHosts(hostsBegin+"\n", []HostEntry{{Name: "mac.tailtest.ts.net", Address: address}}, nil); err == nil {
		t.Fatal("unclosed marker must prevent rewriting")
	}
}
