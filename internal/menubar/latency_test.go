package menubar

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"hello-tsnet/internal/tailnet"
)

func TestLatencyOnlyProbesOnlineDevicesAndStops(t *testing.T) {
	started, stopped := make(chan string, 4), make(chan struct{}, 4)
	var reads atomic.Int32
	monitor := latencyMonitor{
		devices: func(context.Context) ([]tailnet.Device, error) {
			reads.Add(1)
			return []tailnet.Device{
				{ID: "online", State: "online", Addresses: []string{"100.64.0.2"}},
				{ID: "offline", State: "offline", Addresses: []string{"100.64.0.3"}},
				{ID: "expired", State: "expired", Addresses: []string{"100.64.0.4"}},
			}, nil
		},
		ping: func(ctx context.Context, address string) (time.Duration, error) {
			started <- address
			<-ctx.Done()
			stopped <- struct{}{}
			return 0, ctx.Err()
		},
		publish: func(uint64, latencyResult) { t.Error("canceled ping published") },
	}
	if reads.Load() != 0 { t.Fatal("monitor worked before activation") }
	first := monitor.start()
	defer monitor.stop()
	if monitor.start() != first { t.Fatal("activation must be idempotent") }
	select {
	case address := <-started:
		if address != "100.64.0.2" { t.Fatalf("unexpected peer: %s", address) }
	case <-time.After(time.Second): t.Fatal("opening did not start ping immediately")
	}
	monitor.stop()
	select {
	case <-stopped:
	case <-time.After(time.Second): t.Fatal("hiding did not cancel pending ping")
	}
	select {
	case address := <-started: t.Fatalf("ping after stop or offline ping: %s", address)
	case <-time.After(20*time.Millisecond):
	}
}

func TestLatencyRestartsWithFreshSessionAndResults(t *testing.T) {
	results := make(chan struct { session uint64; result latencyResult }, 8)
	monitor := latencyMonitor{
		devices: func(context.Context) ([]tailnet.Device, error) {
			return []tailnet.Device{{ID: "v6", State: "online", Addresses: []string{"fd7a:115c:a1e0::2"}}}, nil
		},
		ping: func(context.Context, string) (time.Duration, error) { return 12500*time.Microsecond, nil },
		publish: func(session uint64, result latencyResult) { results <- struct { session uint64; result latencyResult }{session, result} },
	}
	defer monitor.stop()
	first := monitor.start()
	check := func(session uint64) {
		t.Helper()
		select {
		case got := <-results:
			if got.session != session || got.result.ID != "v6" || got.result.Milliseconds == nil || *got.result.Milliseconds != 12.5 {
				t.Fatalf("unexpected result: %+v", got)
			}
		case <-time.After(time.Second): t.Fatal("missing ping result")
		}
	}
	check(first)
	monitor.stop()
	second := monitor.start()
	if first == second { t.Fatal("reopening must invalidate queued results") }
	check(second)
}

func TestLatencyBoundsConcurrency(t *testing.T) {
	started := make(chan struct{}, 100)
	monitor := latencyMonitor{
		devices: func(context.Context) ([]tailnet.Device, error) {
			var devices []tailnet.Device
			for i := 1; i <= 30; i++ {
				devices = append(devices, tailnet.Device{ID: fmt.Sprint(i), State: "online", Addresses: []string{fmt.Sprintf("100.64.0.%d", i)}})
			}
			return devices, nil
		},
		ping: func(ctx context.Context, _ string) (time.Duration, error) { started <- struct{}{}; <-ctx.Done(); return 0, ctx.Err() },
		publish: func(uint64, latencyResult) {},
	}
	monitor.start()
	defer monitor.stop()
	for i := 0; i < 8; i++ {
		select { case <-started: case <-time.After(time.Second): t.Fatal("workers did not start") }
	}
	select { case <-started: t.Fatal("more than eight concurrent probes"); case <-time.After(20*time.Millisecond): }
}

func TestLatencyStopSuppressesLateSuccess(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	results := make(chan uint64, 2)
	var calls atomic.Int32
	monitor := latencyMonitor{
		devices: func(context.Context) ([]tailnet.Device, error) {
			return []tailnet.Device{{ID: "peer", State: "online", Addresses: []string{"100.64.0.2"}}}, nil
		},
		ping: func(context.Context, string) (time.Duration, error) {
			if calls.Add(1) == 1 {
				close(started)
				<-release
			}
			return time.Millisecond, nil
		},
		publish: func(session uint64, _ latencyResult) { results <- session },
	}
	monitor.start()
	defer monitor.stop()
	select {
	case <-started:
	case <-time.After(time.Second): t.Fatal("first session did not start")
	}
	monitor.stop()
	second := monitor.start()
	close(release)
	select {
	case session := <-results:
		if session != second { t.Fatal("canceled session published late success") }
	case <-time.After(time.Second): t.Fatal("new session did not publish")
	}
	select {
	case <-results: t.Fatal("late success leaked into new session")
	case <-time.After(20*time.Millisecond):
	}
}

func TestLatencyReportsTimeoutAndContinues(t *testing.T) {
	results := make(chan latencyResult, 2)
	monitor := latencyMonitor{
		devices: func(context.Context) ([]tailnet.Device, error) {
			return []tailnet.Device{{ID: "peer", State: "online", Addresses: []string{"100.64.0.2"}}}, nil
		},
		ping: func(ctx context.Context, _ string) (time.Duration, error) { <-ctx.Done(); return 0, ctx.Err() },
		publish: func(_ uint64, result latencyResult) { results <- result },
	}
	monitor.start()
	defer monitor.stop()
	select {
	case result := <-results:
		if result.State != "timeout" || result.Milliseconds != nil { t.Fatalf("unexpected timeout: %+v", result) }
	case <-time.After(3*time.Second): t.Fatal("timeout was not bounded")
	}
}
