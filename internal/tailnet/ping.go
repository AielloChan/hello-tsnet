package tailnet

import (
	"context"
	"errors"
	"math"
	"net/netip"
	"time"

	"tailscale.com/net/tsaddr"
	"tailscale.com/tailcfg"
)

func (r *Runtime) Ping(ctx context.Context, address string) (time.Duration, error) {
	if r.Status().State != StateReady {
		return 0, ErrDevicesUnavailable
	}
	ip, err := netip.ParseAddr(address)
	if err != nil || !tsaddr.IsTailscaleIP(ip) {
		return 0, errors.New("invalid Tailnet ping address")
	}
	return r.backend.Ping(ctx, ip)
}

func (b *tsnetBackend) Ping(ctx context.Context, address netip.Addr) (time.Duration, error) {
	client, err := b.server.LocalClient()
	if err != nil {
		return 0, err
	}
	result, err := client.Ping(ctx, address, tailcfg.PingICMP)
	if err != nil {
		return 0, err
	}
	if result == nil || result.Err != "" || result.LatencySeconds < 0 || math.IsNaN(result.LatencySeconds) || math.IsInf(result.LatencySeconds, 0) {
		return 0, errors.New("Tailnet ping failed")
	}
	return time.Duration(result.LatencySeconds * float64(time.Second)), nil
}
