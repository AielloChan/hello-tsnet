package direct

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"os"
	"sync"

	"github.com/tailscale/wireguard-go/tun"

	"hello-tsnet/internal/forward"
)

const mtu = 1280

type outgoing struct {
	raw    []byte
	native *os.File
}

// Device combines native traffic with responses from the same-port proxy.
// Replacing or removing the utun descriptor does not restart the Tailnet node.
type Device struct {
	ctx      context.Context
	cancel   context.CancelFunc
	proxy    *forward.Gateway
	dispatch *dispatcher
	queue    chan outgoing
	events   chan tun.Event
	mu       sync.RWMutex
	file     *os.File
	once     sync.Once
}

var _ tun.Device = (*Device)(nil)

func NewDevice() (*Device, error) {
	gateway, err := forward.NewGateway()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &Device{
		ctx: ctx, cancel: cancel, proxy: gateway, dispatch: newDispatcher(),
		queue: make(chan outgoing, 256), events: make(chan tun.Event, 1),
	}
	d.events <- tun.EventUp
	go func() {
		for {
			raw := gateway.Read(ctx)
			if raw == nil || !d.enqueue(outgoing{raw: raw}) {
				return
			}
		}
	}()
	return d, nil
}

func (d *Device) enqueue(p outgoing) bool {
	select {
	case d.queue <- p:
		return true
	case <-d.ctx.Done():
		return false
	}
}

func (d *Device) setFile(file *os.File) {
	d.mu.Lock()
	if file != nil && d.ctx.Err() != nil {
		d.mu.Unlock()
		_ = file.Close()
		return
	}
	previous := d.file
	d.file = file
	d.mu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}
	if file != nil {
		go d.readNative(file)
	}
}

func (d *Device) readNative(file *os.File) {
	buffer := make([]byte, 65539)
	for {
		n, err := file.Read(buffer)
		if err != nil {
			return
		}
		if n <= 4 || (binary.BigEndian.Uint32(buffer[:4]) != 2 && binary.BigEndian.Uint32(buffer[:4]) != 30) {
			continue
		}
		if !d.enqueue(outgoing{raw: append([]byte(nil), buffer[4:n]...), native: file}) {
			return
		}
	}
}

func (d *Device) File() *os.File           { return nil }
func (d *Device) Name() (string, error)    { return "hello-tsnet", nil }
func (d *Device) MTU() (int, error)        { return mtu, nil }
func (d *Device) BatchSize() int           { return 1 }
func (d *Device) Events() <-chan tun.Event { return d.events }

func (d *Device) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	if len(bufs) == 0 || len(sizes) == 0 || offset < 0 || offset > len(bufs[0]) {
		return 0, io.ErrShortBuffer
	}
	for {
		select {
		case <-d.ctx.Done():
			return 0, net.ErrClosed
		case p := <-d.queue:
			if p.native != nil {
				d.mu.RLock()
				valid := d.file == p.native && d.dispatch.outbound(p.raw)
				d.mu.RUnlock()
				if !valid {
					continue
				}
			} else if !d.dispatch.proxyOutbound(p.raw) {
				continue
			}
			if len(p.raw) > len(bufs[0])-offset {
				continue
			}
			copy(bufs[0][offset:], p.raw)
			sizes[0] = len(p.raw)
			return 1, nil
		}
	}
}

func (d *Device) Write(bufs [][]byte, offset int) (int, error) {
	for i, buf := range bufs {
		if offset < 0 || offset > len(buf) {
			return i, io.ErrShortBuffer
		}
		if d.ctx.Err() != nil {
			return i, net.ErrClosed
		}
		raw := buf[offset:]
		target := d.dispatch.inbound(raw)
		if target&toProxy != 0 {
			d.proxy.Inject(raw)
		}
		if target&toNative != 0 {
			d.mu.RLock()
			file := d.file
			d.mu.RUnlock()
			if file != nil {
				writeNative(file, buf, offset)
			}
		}
	}
	return len(bufs), nil
}

func writeNative(file *os.File, buf []byte, offset int) {
	raw := buf[offset:]
	family := uint32(2) // Darwin AF_INET
	if raw[0]>>4 == 6 {
		family = 30 // Darwin AF_INET6
	}
	var frame []byte
	if offset >= 4 {
		frame = buf[offset-4:]
	} else {
		frame = make([]byte, len(raw)+4)
		copy(frame[4:], raw)
	}
	binary.BigEndian.PutUint32(frame, family)
	// The descriptor can close concurrently with a peer-map update.
	_, _ = file.Write(frame)
}

func (d *Device) Close() error {
	d.once.Do(func() {
		d.cancel()
		d.setFile(nil)
		close(d.events)
		d.proxy.Close()
	})
	return nil
}
