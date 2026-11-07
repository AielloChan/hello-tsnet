package tailnet

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"
)

type pingBackend struct {
	*fakeBackend
	ping func(context.Context, netip.Addr) (time.Duration, error)
}

func (b *pingBackend) Ping(ctx context.Context, address netip.Addr) (time.Duration, error) {
	return b.ping(ctx, address)
}

func TestPingRequiresReadyTailnetAddress(t *testing.T) {
	calls := 0
	backend := &pingBackend{fakeBackend: newFakeBackend(), ping: func(ctx context.Context, address netip.Addr) (time.Duration, error) {
		calls++
		if address.String() != "100.64.0.2" { t.Fatalf("unexpected destination: %s", address) }
		return 12*time.Millisecond, ctx.Err()
	}}
	runtime := newRuntime(backend)
	if _, err := runtime.Ping(context.Background(), "100.64.0.2"); err == nil { t.Fatal("ping while signed out") }
	runtime.status.State = StateReady
	for _, address := range []string{"", "example.com", "127.0.0.1", "192.168.1.1"} {
		if _, err := runtime.Ping(context.Background(), address); err == nil { t.Fatalf("accepted %q", address) }
	}
	if calls != 0 { t.Fatal("invalid ping reached backend") }
	if got, err := runtime.Ping(context.Background(), "100.64.0.2"); err != nil || got != 12*time.Millisecond { t.Fatalf("ping = %v, %v", got, err) }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runtime.Ping(ctx, "100.64.0.2"); !errors.Is(err, context.Canceled) { t.Fatalf("cancellation lost: %v", err) }
}
