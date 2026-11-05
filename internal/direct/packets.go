package direct

import (
	"encoding/binary"
	"net/netip"
	"sync"
	"time"

	"tailscale.com/net/tsaddr"
)

type destination uint8

const (
	drop     destination = 0
	toNative destination = 1
	toProxy  destination = 2
)

const (
	tcpProtocol = 6
	udpProtocol = 17
	maxFlows    = 16384
)

type flow struct {
	src, dst     netip.Addr
	sport, dport uint16
	proto        uint8
}

func (f flow) reverse() flow {
	return flow{src: f.dst, dst: f.src, sport: f.dport, dport: f.sport, proto: f.proto}
}

type packet struct {
	flow
	fragment bool // A non-first fragment has no transport header.
	tcpFlags byte
}

func parsePacket(b []byte) (p packet, ok bool) {
	if len(b) == 0 {
		return p, false
	}
	var offset int
	switch b[0] >> 4 {
	case 4:
		if len(b) < 20 {
			return p, false
		}
		offset = int(b[0]&15) * 4
		if offset < 20 || offset > len(b) || int(binary.BigEndian.Uint16(b[2:])) != len(b) {
			return p, false
		}
		p.src = netip.AddrFrom4([4]byte(b[12:16]))
		p.dst = netip.AddrFrom4([4]byte(b[16:20]))
		p.proto = b[9]
		p.fragment = binary.BigEndian.Uint16(b[6:])&0x1fff != 0
	case 6:
		if len(b) < 40 || int(binary.BigEndian.Uint16(b[4:]))+40 != len(b) {
			return p, false
		}
		p.src = netip.AddrFrom16([16]byte(b[8:24]))
		p.dst = netip.AddrFrom16([16]byte(b[24:40]))
		p.proto, offset = b[6], 40
		for {
			switch p.proto {
			case 0, 60: // Hop-by-hop and destination options.
				if len(b)-offset < 2 {
					return p, false
				}
				p.proto, offset = b[offset], offset+(int(b[offset+1])+1)*8
			case 44:
				if len(b)-offset < 8 {
					return p, false
				}
				p.fragment = binary.BigEndian.Uint16(b[offset+2:])&0xfff8 != 0
				p.proto, offset = b[offset], offset+8
				if p.fragment {
					return p, true
				}
			case 43: // Source routing can change the validated destination.
				return p, false
			default:
				goto transport
			}
		}
	default:
		return p, false
	}
transport:
	if !p.fragment && (p.proto == tcpProtocol || p.proto == udpProtocol) {
		if len(b)-offset < 8 || (p.proto == tcpProtocol && len(b)-offset < 20) {
			return p, false
		}
		p.sport = binary.BigEndian.Uint16(b[offset:])
		p.dport = binary.BigEndian.Uint16(b[offset+2:])
		if p.proto == tcpProtocol {
			p.tcpFlags = b[offset+13]
		}
	}
	return p, true
}

type dispatcher struct {
	mu                 sync.Mutex
	local, nativePeers map[netip.Addr]bool
	flows              map[flow]time.Time
	screen             map[netip.Addr]time.Time
	nextSweep          time.Time
}

func newDispatcher() *dispatcher {
	return &dispatcher{
		flows:  make(map[flow]time.Time),
		screen: make(map[netip.Addr]time.Time),
	}
}

func addrSet(addrs []netip.Addr) map[netip.Addr]bool {
	out := make(map[netip.Addr]bool, len(addrs))
	for _, addr := range addrs {
		out[addr] = true
	}
	return out
}

func (d *dispatcher) setAddresses(local, routed []netip.Addr) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.local, d.nativePeers = addrSet(local), addrSet(routed)
	for f := range d.flows {
		if !d.local[f.src] || !d.nativePeers[f.dst] {
			delete(d.flows, f)
		}
	}
	for peer := range d.screen {
		if !d.nativePeers[peer] {
			delete(d.screen, peer)
		}
	}
}

func (d *dispatcher) outbound(raw []byte) bool {
	p, ok := parsePacket(raw)
	if !ok {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.local[p.src] || !d.nativePeers[p.dst] {
		return false
	}
	now := time.Now()
	if !p.fragment && (p.proto == tcpProtocol || p.proto == udpProtocol) {
		if !d.remember(p.flow, now) {
			return false
		}
		if p.proto == tcpProtocol && p.dport == 5900 {
			// Apple can send media before the client sends any UDP.
			d.screen[p.dst] = now.Add(5 * time.Minute)
		}
	}
	return true
}

func (d *dispatcher) proxyOutbound(raw []byte) bool {
	p, ok := parsePacket(raw)
	if !ok || !tsaddr.IsTailscaleIP(p.dst) {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.local[p.src]
}

func (d *dispatcher) inbound(raw []byte) destination {
	p, ok := parsePacket(raw)
	if !ok || !tsaddr.IsTailscaleIP(p.src) {
		return drop
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.local[p.dst] {
		return drop
	}
	if !d.nativePeers[p.src] {
		return toProxy
	}
	if p.fragment {
		// Only the stack that receives the first fragment can deliver the
		// datagram. This preserves out-of-order fragments on both paths.
		return toNative | toProxy
	}
	if p.proto != tcpProtocol && p.proto != udpProtocol {
		return toNative
	}
	if p.proto == tcpProtocol && p.tcpFlags&0x12 == 0x02 {
		return toProxy
	}
	now := time.Now()
	f := p.flow.reverse()
	if d.flows[f].After(now) {
		d.remember(f, now)
		return toNative
	}
	if p.proto == udpProtocol && screenPort(p.sport) && screenPort(p.dport) && d.screen[p.src].After(now) {
		d.remember(f, now)
		return toNative
	}
	return toProxy
}

func screenPort(port uint16) bool { return port >= 5900 && port <= 5902 }

func (d *dispatcher) remember(f flow, now time.Time) bool {
	if !now.Before(d.nextSweep) {
		d.nextSweep = now.Add(time.Second)
		for key, expiry := range d.flows {
			if !expiry.After(now) {
				delete(d.flows, key)
			}
		}
	}
	if _, ok := d.flows[f]; !ok && len(d.flows) >= maxFlows {
		return false
	}
	lifetime := 2 * time.Minute
	if f.proto == tcpProtocol {
		lifetime = 2 * time.Hour
	}
	d.flows[f] = now.Add(lifetime)
	return true
}
