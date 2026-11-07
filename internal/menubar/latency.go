package menubar

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"time"

	"hello-tsnet/internal/tailnet"
)

type latencyResult struct {
	ID           string   `json:"id"`
	Address      string   `json:"address"`
	Milliseconds *float64 `json:"milliseconds,omitempty"`
	State        string   `json:"state,omitempty"`
}

// Each foreground visit owns a cancellable session. The native panel also
// checks its session ID, since a result may already be queued on the UI thread.
type latencyMonitor struct {
	devices func(context.Context) ([]tailnet.Device, error)
	ping    func(context.Context, string) (time.Duration, error)
	publish func(uint64, latencyResult)

	mu      sync.Mutex
	session uint64
	cancel  context.CancelFunc
}

func (m *latencyMonitor) start() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		return m.session
	}
	m.session++
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	go m.run(ctx, m.session)
	return m.session
}

func (m *latencyMonitor) stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
}

func (m *latencyMonitor) run(ctx context.Context, session uint64) {
	for ctx.Err() == nil {
		m.round(ctx, session)
		timer := time.NewTimer(5 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (m *latencyMonitor) round(ctx context.Context, session uint64) {
	readCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	devices, err := m.devices(readCtx)
	cancel()
	if err != nil || ctx.Err() != nil {
		return
	}
	jobs := make(chan tailnet.Device)
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for device := range jobs {
				if ctx.Err() != nil {
					return
				}
				address := pingAddress(device.Addresses)
				if address == "" {
					continue
				}
				probeCtx, stop := context.WithTimeout(ctx, 2*time.Second)
				duration, err := m.ping(probeCtx, address)
				result := latencyResult{ID: device.ID, Address: address}
				if err == nil {
					milliseconds := float64(duration) / float64(time.Millisecond)
					result.Milliseconds = &milliseconds
				} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
					result.State = "timeout"
				} else {
					result.State = "unreachable"
				}
				stop()
				if ctx.Err() == nil {
					m.publish(session, result)
				}
			}
		}()
	}
	for _, device := range devices {
		if device.State != "online" {
			continue
		}
		select {
		case jobs <- device:
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return
		}
	}
	close(jobs)
	workers.Wait()
}

func pingAddress(addresses []string) string {
	var ipv6 string
	for _, raw := range addresses {
		address, err := netip.ParseAddr(raw)
		if err != nil {
			continue
		}
		if address.Is4() {
			return address.String()
		}
		ipv6 = address.String()
	}
	return ipv6
}
