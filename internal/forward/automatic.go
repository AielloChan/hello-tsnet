package forward

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"gvisor.dev/gvisor/pkg/waiter"
)

const (
	nic            = 1
	maxSessions    = 128
	udpIdleTimeout = 60 * time.Second
)

// Gateway terminates only remote-initiated TCP and UDP flows. Its callers
// supply authenticated packets addressed to the local Tailnet node.
type Gateway struct {
	stack  *stack.Stack
	link   *channel.Endpoint
	ctx    context.Context
	cancel context.CancelFunc
	slots  chan struct{}
}

func NewGateway() (*Gateway, error) {
	ctx, cancel := context.WithCancel(context.Background())
	g := &Gateway{
		ctx: ctx, cancel: cancel, slots: make(chan struct{}, maxSessions),
		stack: stack.New(stack.Options{
			NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol},
			TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol},
		}),
		link: channel.New(256, 1280, ""),
	}
	for _, err := range []tcpip.Error{g.stack.CreateNIC(nic, g.link),
		g.stack.SetPromiscuousMode(nic, true), g.stack.SetSpoofing(nic, true)} {
		if err != nil {
			g.Close()
			return nil, fmt.Errorf("initialize loopback forwarding: %s", err)
		}
	}
	g.stack.SetRouteTable([]tcpip.Route{{Destination: header.IPv4EmptySubnet, NIC: nic},
		{Destination: header.IPv6EmptySubnet, NIC: nic}})
	g.stack.SetTransportProtocolHandler(tcp.ProtocolNumber, tcp.NewForwarder(g.stack, 0, maxSessions, g.acceptTCP).HandlePacket)
	g.stack.SetTransportProtocolHandler(udp.ProtocolNumber, udp.NewForwarder(g.stack, g.acceptUDP).HandlePacket)
	return g, nil
}

func (g *Gateway) Inject(raw []byte) {
	if len(raw) == 0 || g.ctx.Err() != nil {
		return
	}
	protocol := ipv4.ProtocolNumber
	if raw[0]>>4 == 6 {
		protocol = ipv6.ProtocolNumber
	}
	p := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(raw)})
	g.link.InjectInbound(protocol, p)
	p.DecRef()
}

func (g *Gateway) Read(ctx context.Context) []byte {
	p := g.link.ReadContext(ctx)
	if p == nil {
		return nil
	}
	defer p.DecRef()
	v := p.ToView()
	defer v.Release()
	return append([]byte(nil), v.AsSlice()...)
}

func (g *Gateway) Close() {
	g.cancel()
	g.stack.Close()
	g.link.Close()
	g.stack.Wait()
}

func (g *Gateway) reserve() bool {
	if g.ctx.Err() != nil {
		return false
	}
	select {
	case g.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (g *Gateway) acceptTCP(r *tcp.ForwarderRequest) {
	if !g.reserve() {
		r.Complete(true)
		return
	}
	go func() {
		defer func() { <-g.slots }()
		port := r.ID().LocalPort
		var q waiter.Queue
		ep, err := r.CreateEndpoint(&q)
		r.Complete(err != nil)
		if err == nil {
			g.relay(gonet.NewTCPConn(&q, ep), "tcp", port)
		}
	}()
}

func (g *Gateway) acceptUDP(r *udp.ForwarderRequest) bool {
	if !g.reserve() {
		return false
	}
	var q waiter.Queue
	ep, err := r.CreateEndpoint(&q)
	if err != nil {
		<-g.slots
		return false
	}
	port := r.ID().LocalPort
	go func() {
		defer func() { <-g.slots }()
		g.relay(gonet.NewUDPConn(&q, ep), "udp", port)
	}()
	return true
}

func (g *Gateway) relay(incoming net.Conn, protocol string, port uint16) {
	defer incoming.Close()
	if port == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(g.ctx, 5*time.Second)
	local, err := (&net.Dialer{}).DialContext(ctx, protocol, net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port))))
	cancel()
	if err != nil {
		return
	}
	defer local.Close()
	stop := context.AfterFunc(g.ctx, func() {
		_ = incoming.Close()
		_ = local.Close()
	})
	defer stop()
	if protocol == "udp" {
		proxyDatagrams(incoming, local, udpIdleTimeout)
	} else {
		proxyConn(incoming, local)
	}
}
