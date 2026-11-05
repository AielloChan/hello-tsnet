package tailnet

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"time"

	"hello-tsnet/internal/routehelper"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
)

func (b *tsnetBackend) runRoutes(ctx context.Context, networkStatus func(*ipnstate.Status), report func(string, bool)) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	client, err := b.server.LocalClient()
	if err != nil {
		report("无法读取 Tailnet 设备清单", false)
		return
	}
	changes := make(chan struct{}, 1)
	notify := func() {
		select {
		case changes <- struct{}{}:
		default:
		}
	}
	go func() {
		for ctx.Err() == nil {
			watcher, err := client.WatchIPNBus(ctx, ipn.NotifyInitialStatus|ipn.NotifyInitialState|ipn.NotifyPeerPatches|ipn.NotifyNoNetMap)
			if err == nil {
				for {
					event, err := watcher.Next()
					if err != nil {
						break
					}
					if event.InitialStatus != nil || event.State != nil || event.SelfChange != nil ||
						len(event.PeersChanged) > 0 || len(event.PeersRemoved) > 0 {
						notify()
					}
				}
				_ = watcher.Close()
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}()
	b.device.Sync(ctx, func(ctx context.Context) (routehelper.Snapshot, error) {
		status, err := client.Status(ctx)
		if err != nil {
			return routehelper.Snapshot{}, err
		}
		networkStatus(status)
		return routeSnapshot(status)
	}, changes, report)
}

func routeSnapshot(status *ipnstate.Status) (routehelper.Snapshot, error) {
	var snapshot routehelper.Snapshot
	if status == nil {
		return snapshot, errors.New("missing Tailnet status")
	}
	switch status.BackendState {
	case "NeedsLogin", "NeedsMachineAuth", "Stopped", "NoState":
		return snapshot, nil
	case "Running":
	default:
		return snapshot, errors.New("Tailnet status is changing")
	}
	if status.Self == nil || len(status.TailscaleIPs) == 0 {
		return snapshot, errors.New("missing Tailnet identity")
	}
	snapshot.Local = routehelper.AssignedAddresses(status.TailscaleIPs)
	for _, peer := range status.Peer {
		if peer != nil && peer.InNetworkMap && !peer.Expired {
			name := routehelper.CanonicalHostName(peer.DNSName)
			for _, addr := range routehelper.AssignedAddresses(peer.TailscaleIPs) {
				for _, local := range snapshot.Local {
					if addr.Is4() == local.Is4() && addr != local {
						snapshot.Peers = append(snapshot.Peers, addr)
						if name != "" {
							snapshot.Hosts = append(snapshot.Hosts, routehelper.HostEntry{Name: name, Address: addr})
						}
						break
					}
				}
			}
		}
	}
	slices.SortFunc(snapshot.Peers, netip.Addr.Compare)
	snapshot.Peers = slices.Compact(snapshot.Peers)
	return snapshot, nil
}
