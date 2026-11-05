package direct

import (
	"encoding/binary"
	"net/netip"
	"testing"
)

func udp4(src, dst string, sourcePort, destPort uint16, fragment uint16) []byte {
	p := make([]byte, 28)
	p[0], p[9] = 0x45, 17
	binary.BigEndian.PutUint16(p[2:], uint16(len(p)))
	binary.BigEndian.PutUint16(p[6:], fragment)
	copy(p[12:], netip.MustParseAddr(src).AsSlice())
	copy(p[16:], netip.MustParseAddr(dst).AsSlice())
	binary.BigEndian.PutUint16(p[20:], sourcePort)
	binary.BigEndian.PutUint16(p[22:], destPort)
	binary.BigEndian.PutUint16(p[24:], 8)
	return p
}

func TestPacketRoutingKeepsRepliesNativeAndForwardsNewSessions(t *testing.T) {
	const local, peer = "100.64.0.1", "100.64.0.2"
	d := newDispatcher()
	d.setAddresses([]netip.Addr{netip.MustParseAddr(local)}, []netip.Addr{netip.MustParseAddr(peer)})
	if got := d.inbound(udp4(peer, local, 8000, 43000, 0)); got != toProxy {
		t.Fatalf("new remote flow = %v", got)
	}
	if !d.outbound(udp4(local, peer, 43000, 8000, 0)) {
		t.Fatal("local flow denied")
	}
	if got := d.inbound(udp4(peer, local, 8000, 43000, 0)); got != toNative {
		t.Fatalf("native reply = %v", got)
	}
	d.setAddresses([]netip.Addr{netip.MustParseAddr(local)}, nil)
	if got := d.inbound(udp4(peer, local, 8000, 43000, 0)); got != toProxy {
		t.Fatalf("reply without a native route = %v", got)
	}
	if got := d.inbound(udp4(peer, "100.64.0.3", 8000, 43000, 0)); got != drop {
		t.Fatalf("foreign destination = %v", got)
	}
}

func TestFragmentsCanReachBothStacksBeforeFlowIsKnown(t *testing.T) {
	d := newDispatcher()
	d.setAddresses([]netip.Addr{netip.MustParseAddr("100.64.0.1")}, []netip.Addr{netip.MustParseAddr("100.64.0.2")})
	if got := d.inbound(udp4("100.64.0.2", "100.64.0.1", 0, 0, 1)); got != toNative|toProxy {
		t.Fatalf("orphan fragment = %v", got)
	}
}

func TestIncomingTCPSYNDoesNotReuseNativeReplyClassification(t *testing.T) {
	local := netip.MustParseAddr("100.64.0.1")
	peer := netip.MustParseAddr("100.64.0.2")
	d := newDispatcher()
	d.setAddresses([]netip.Addr{local}, []netip.Addr{peer})
	makeTCP := func(src, dst netip.Addr, sport, dport uint16, flags byte) []byte {
		p := append(udp4(src.String(), dst.String(), sport, dport, 0)[:24], make([]byte, 16)...)
		p[9] = tcpProtocol
		binary.BigEndian.PutUint16(p[2:], 40)
		p[32], p[33] = 0x50, flags
		return p
	}
	if !d.outbound(makeTCP(local, peer, 43000, 5900, 0x02)) {
		t.Fatal("native connection denied")
	}
	if got := d.inbound(makeTCP(peer, local, 5900, 43000, 0x12)); got != toNative {
		t.Fatalf("SYN-ACK = %v", got)
	}
	if got := d.inbound(udp4(peer.String(), local.String(), 5901, 5901, 0)); got != toNative {
		t.Fatalf("first Screen Sharing media datagram = %v", got)
	}
	if got := d.inbound(makeTCP(peer, local, 5900, 43000, 0x02)); got != toProxy {
		t.Fatalf("new TCP SYN = %v", got)
	}
}
