package forward

import (
	"context"
	"io"
	"net"
	"net/netip"
	"testing"
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
)

func TestAutomaticForwardingTCPAndUDP(t *testing.T) {
	t.Run("IPv4", func(t *testing.T) {
		testForwarding(t, ipv4.NewProtocol, ipv4.ProtocolNumber, header.IPv4EmptySubnet, "100.64.0.2", "100.64.0.1")
	})
	t.Run("IPv6", func(t *testing.T) {
		testForwarding(t, ipv6.NewProtocol, ipv6.ProtocolNumber, header.IPv6EmptySubnet, "fd7a:115c:a1e0::2", "fd7a:115c:a1e0::1")
	})
}

func testForwarding(t *testing.T, factory stack.NetworkProtocolFactory, ipProtocol tcpip.NetworkProtocolNumber, route tcpip.Subnet, source, destination string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	gateway, err := NewGateway()
	if err != nil {
		t.Fatal(err)
	}
	defer gateway.Close()
	peer := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{factory},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol},
	})
	defer peer.Close()
	link := channel.New(128, 1280, "")
	if err := peer.CreateNIC(1, link); err != nil {
		t.Fatal(err)
	}
	src := tcpip.AddrFromSlice(netip.MustParseAddr(source).AsSlice())
	dst := tcpip.AddrFromSlice(netip.MustParseAddr(destination).AsSlice())
	if err := peer.AddProtocolAddress(1, tcpip.ProtocolAddress{Protocol: ipProtocol,
		AddressWithPrefix: tcpip.AddressWithPrefix{Address: src, PrefixLen: src.BitLen()}}, stack.AddressProperties{}); err != nil {
		t.Fatal(err)
	}
	peer.SetRouteTable([]tcpip.Route{{Destination: route, NIC: 1}})
	go func() {
		for {
			p := link.ReadContext(ctx)
			if p == nil {
				return
			}
			v := p.ToView()
			gateway.Inject(v.AsSlice())
			v.Release()
			p.DecRef()
		}
	}()
	go func() {
		for {
			raw := gateway.Read(ctx)
			if raw == nil {
				return
			}
			p := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(raw)})
			link.InjectInbound(ipProtocol, p)
			p.DecRef()
		}
	}()

	tcpListener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tcpListener.Close()
	go func() {
		if conn, err := tcpListener.Accept(); err == nil {
			defer conn.Close()
			_, _ = io.Copy(conn, conn)
		}
	}()
	udpListener, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udpListener.Close()
	go func() {
		buf := make([]byte, 128)
		for {
			n, addr, err := udpListener.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = udpListener.WriteTo(buf[:n], addr)
		}
	}()

	for _, protocol := range []string{"tcp", "udp"} {
		t.Run(protocol, func(t *testing.T) {
			var conn net.Conn
			var err error
			switch protocol {
			case "tcp":
				conn, err = gonet.DialContextTCP(ctx, peer, tcpip.FullAddress{
					NIC: 1, Addr: dst, Port: uint16(tcpListener.Addr().(*net.TCPAddr).Port)}, ipProtocol)
			case "udp":
				remote := tcpip.FullAddress{NIC: 1, Addr: dst, Port: uint16(udpListener.LocalAddr().(*net.UDPAddr).Port)}
				conn, err = gonet.DialUDP(peer, nil, &remote, ipProtocol)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
			if _, err := conn.Write([]byte("echo")); err != nil {
				t.Fatal(err)
			}
			reply := make([]byte, 4)
			if _, err := io.ReadFull(conn, reply); err != nil || string(reply) != "echo" {
				t.Fatalf("reply = %q, %v", reply, err)
			}
			if protocol == "udp" {
				if _, err := conn.Write(nil); err != nil {
					t.Fatal(err)
				}
				if n, err := conn.Read(reply); err != nil || n != 0 {
					t.Fatalf("zero-length datagram = %d bytes, %v", n, err)
				}
			}
		})
	}
}
