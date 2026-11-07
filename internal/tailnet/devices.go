package tailnet

import (
	"context"
	"errors"
	"net/netip"
	"sort"
	"strings"
	"time"

	"tailscale.com/ipn/ipnstate"
)

var ErrDevicesUnavailable = errors.New("设备状态暂不可用，请确认 Tailscale 已连接后刷新")

// Device is the display-only projection of a Tailnet device. Never expose the
// full LocalAPI response: it also contains authorization and network details.
type Device struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	DNSName   string     `json:"dnsName"`
	Addresses []string   `json:"addresses"`
	OS        string     `json:"os"`
	State     string     `json:"state"`
	Local     bool       `json:"local,omitempty"`
	LastSeen  *time.Time `json:"lastSeen,omitempty"`
}

func (r *Runtime) Devices(ctx context.Context) ([]Device, error) {
	// LocalClient implicitly starts tsnet. Do not call it during startup or
	// while awaiting authorization, and do not treat a cached map as live data.
	if r.Status().State != StateReady {
		return nil, ErrDevicesUnavailable
	}
	status, err := r.backend.NetworkStatus(ctx)
	if err != nil || status == nil || status.BackendState != "Running" || status.Self == nil || !status.Self.Online {
		return nil, ErrDevicesUnavailable
	}
	devices := make([]Device, 0, len(status.Peer))
	for _, peer := range status.Peer {
		if peer == nil || !peer.InNetworkMap {
			continue
		}
		devices = append(devices, displayDevice(peer, peer.TailscaleIPs))
	}
	sort.Slice(devices, func(i, j int) bool {
		if (devices[i].State == "online") != (devices[j].State == "online") {
			return devices[i].State == "online"
		}
		if devices[i].Name != devices[j].Name {
			return devices[i].Name < devices[j].Name
		}
		return devices[i].ID < devices[j].ID
	})
	return devices, nil
}

func displayDevice(peer *ipnstate.PeerStatus, addresses []netip.Addr) Device {
	device := Device{
		ID: string(peer.ID), Name: peer.HostName,
		DNSName: strings.TrimSuffix(peer.DNSName, "."),
		Addresses: []string{}, OS: peer.OS, State: "unknown",
	}
	for _, address := range addresses {
		if address.IsValid() {
			device.Addresses = append(device.Addresses, address.String())
		}
	}
	if device.Name == "" {
		device.Name = strings.Split(device.DNSName, ".")[0]
	}
	if device.Name == "" && len(device.Addresses) > 0 {
		device.Name = device.Addresses[0]
	}
	if device.Name == "" {
		device.Name = device.ID
	}
	if peer.Online {
		device.State = "online"
	} else if !peer.LastSeen.IsZero() {
		device.State = "offline"
		seen := peer.LastSeen
		device.LastSeen = &seen
	}
	if peer.Expired {
		device.State = "expired"
	}
	return device
}

func (b *tsnetBackend) NetworkStatus(ctx context.Context) (*ipnstate.Status, error) {
	client, err := b.server.LocalClient()
	if err != nil {
		return nil, err
	}
	return client.Status(ctx)
}
