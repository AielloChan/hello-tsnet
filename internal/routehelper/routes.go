package routehelper

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"

	"tailscale.com/net/tsaddr"
)

const maxRouteAddresses = 8192

// Snapshot contains assigned node addresses only. Subnet routes and defaults
// cannot be expressed by the helper protocol.
type Snapshot struct {
	Local []netip.Addr
	Peers []netip.Addr
	Hosts []HostEntry
}

type peerUpdate struct {
	Peers []netip.Addr
	Hosts []HostEntry
}

type routeSet struct {
	installed map[netip.Addr]bool
}

func (r *routeSet) update(desired []netip.Addr, change func(string, netip.Addr) error) error {
	want := addrSet(desired)
	var result error
	for addr := range r.installed {
		if !want[addr] {
			if err := change("delete", addr); err != nil {
				result = errors.Join(result, fmt.Errorf("delete %s: %w", addr, err))
			} else {
				delete(r.installed, addr)
			}
		}
	}
	for _, addr := range desired {
		if !r.installed[addr] {
			if err := change("add", addr); err != nil {
				result = errors.Join(result, fmt.Errorf("add %s: %w", addr, err))
			} else {
				r.installed[addr] = true
			}
		}
	}
	return result
}

func (r *routeSet) addresses() []netip.Addr {
	result := make([]netip.Addr, 0, len(r.installed))
	for addr := range r.installed {
		result = append(result, addr)
	}
	slices.SortFunc(result, netip.Addr.Compare)
	return result
}

func validAddress(addr netip.Addr) bool {
	return addr.IsValid() && addr.Zone() == "" && !addr.Is4In6() && tsaddr.IsTailscaleIP(addr) &&
		addr != tsaddr.TailscaleServiceIP() && addr != tsaddr.TailscaleServiceIPv6()
}

func validateLocal(addrs []netip.Addr) error {
	if len(addrs) == 0 || len(addrs) > 2 {
		return errors.New("invalid local address count")
	}
	families := make(map[bool]bool, 2)
	for _, addr := range addrs {
		if !validAddress(addr) || families[addr.Is4()] {
			return errors.New("invalid local Tailnet address")
		}
		families[addr.Is4()] = true
	}
	return nil
}

func validatePeers(local, peers []netip.Addr) error {
	if len(peers) > maxRouteAddresses {
		return errors.New("too many peer addresses")
	}
	families := make(map[bool]bool, 2)
	seen := addrSet(local)
	for _, addr := range local {
		families[addr.Is4()] = true
	}
	for _, addr := range peers {
		if !validAddress(addr) || seen[addr] || !families[addr.Is4()] {
			return errors.New("invalid peer Tailnet address")
		}
		seen[addr] = true
	}
	return nil
}

// AssignedAddresses canonicalizes the address list exposed by tsnet.
func AssignedAddresses(addrs []netip.Addr) []netip.Addr {
	result := make([]netip.Addr, 0, len(addrs))
	for _, addr := range addrs {
		if validAddress(addr) {
			result = append(result, addr)
		}
	}
	slices.SortFunc(result, netip.Addr.Compare)
	return slices.Compact(result)
}

type Session interface {
	Update(context.Context, Snapshot) (Reply, error)
	Close()
}

type Reply struct {
	Routed     []netip.Addr
	Failed     int
	HostsError string `json:",omitempty"`
}

func addrSet(addrs []netip.Addr) map[netip.Addr]bool {
	out := make(map[netip.Addr]bool, len(addrs))
	for _, addr := range addrs {
		out[addr] = true
	}
	return out
}
