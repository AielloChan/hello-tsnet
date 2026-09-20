package forward

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"hello-tsnet/internal/config"
)

type shareListener struct {
	connections chan net.Conn
	closed chan struct{}
	once sync.Once
}
func newShareListener() *shareListener { return &shareListener{connections: make(chan net.Conn), closed: make(chan struct{})} }
func (l *shareListener) Accept() (net.Conn, error) {
	select { case c := <-l.connections: return c, nil; case <-l.closed: return nil, net.ErrClosed }
}
func (l *shareListener) Close() error { l.once.Do(func(){close(l.closed)}); return nil }
func (l *shareListener) Addr() net.Addr { return fakeAddr("tailnet:8000") }
func sharedRule(port int, protocol config.Protocol) config.Rule {
	return config.Rule{ID:"shared", Name:"本机服务", LocalPort:port, Protocol:protocol, Direction:config.DirectionInbound}
}
func TestSharePrepareUsesOnlyTailnetAndAbortsBoth(t *testing.T) {
	listener := newFakeListener()
	calls := 0
	manager := NewManager(stubDialer{}, nil, nil, func(network, address string)(net.Listener,error){
		calls++
		if address != ":8000" { t.Fatalf("address = %q", address) }
		if network == "udp" { return nil, errors.New("occupied") }
		return listener,nil
	})
	defer manager.Close(context.Background())
	if _,err := manager.Prepare([]config.Rule{sharedRule(8000,config.ProtocolBoth)}); err == nil { t.Fatal("want failure") }
	if calls != 2 || !listener.isClosed() || len(manager.Rules()) != 0 { t.Fatal("partial share remained") }
	select { case <-listener.started: t.Fatal("accepted before commit"); default: }
}
func TestShareTCPUsesExistingLocalServiceAndDeleteClosesConnections(t *testing.T) {
	local,err := net.Listen("tcp","127.0.0.1:0")
	if err != nil { t.Fatal(err) }; defer local.Close()
	go func(){ c,e := local.Accept(); if e == nil { defer c.Close(); _,_ = io.Copy(c,c) } }()
	port := local.Addr().(*net.TCPAddr).Port
	listener := newShareListener()
	manager := NewManager(stubDialer{},nil,nil,func(network,address string)(net.Listener,error){
		if network != "tcp" || address != ":"+strconv.Itoa(port) { t.Errorf("listen %s %s",network,address) }
		return listener,nil
	})
	defer manager.Close(context.Background())
	change,err := manager.Prepare([]config.Rule{sharedRule(port,config.ProtocolTCP)})
	if err != nil { t.Fatal(err) }; change.Commit()
	client,server := net.Pipe(); defer client.Close()
	listener.connections <- server
	_ = client.SetDeadline(time.Now().Add(time.Second))
	if _,err := client.Write([]byte("hello")); err != nil { t.Fatal(err) }
	buf := make([]byte,5)
	if _,err := io.ReadFull(client,buf); err != nil || string(buf) != "hello" { t.Fatalf("echo = %q %v",buf,err) }
	change,err = manager.Prepare(nil); if err != nil { t.Fatal(err) }; change.Commit()
	if _,err := client.Read(buf); err == nil { t.Fatal("deleted sharing still connected") }
}

type packetFlow struct { net.PacketConn; peer net.Addr }
func (c packetFlow) Read(p []byte)(int,error) { n,_,e := c.ReadFrom(p); return n,e }
func (c packetFlow) Write(p []byte)(int,error) { return c.WriteTo(p,c.peer) }
func (c packetFlow) RemoteAddr() net.Addr { return c.peer }
func TestShareUDPDatagramsClientsAndIdleCleanup(t *testing.T) {
	local,err := net.ListenPacket("udp","127.0.0.1:0")
	if err != nil { t.Fatal(err) }; defer local.Close()
	go func(){ b:=make([]byte,65535); for { n,a,e:=local.ReadFrom(b); if e!=nil{return}; _,_=local.WriteTo(b[:n],a) } }()
	listener := newShareListener()
	manager := NewManager(stubDialer{},nil,nil,func(string,string)(net.Listener,error){ return listener,nil })
	manager.udpIdleTimeout = 100*time.Millisecond
	defer manager.Close(context.Background())
	change,err := manager.Prepare([]config.Rule{sharedRule(local.LocalAddr().(*net.UDPAddr).Port,config.ProtocolUDP)})
	if err != nil { t.Fatal(err) }; change.Commit()
	var flows []net.PacketConn
	var clients []net.Conn
	for i:=0;i<2;i++ {
		flow,e := net.ListenPacket("udp","127.0.0.1:0"); if e!=nil{t.Fatal(e)}; defer flow.Close()
		client,e := net.Dial("udp",flow.LocalAddr().String()); if e!=nil{t.Fatal(e)}; defer client.Close()
		flows=append(flows,flow); clients=append(clients,client)
		listener.connections <- packetFlow{flow,client.LocalAddr()}
	}
	for _,payload:=range [][]byte{[]byte("first"),{},bytes.Repeat([]byte("x"),8192)} {
		for i,client:=range clients {
			_ = client.SetDeadline(time.Now().Add(time.Second))
			data:=append(append([]byte{},payload...),byte(i))
			if len(payload)==0 { data=nil }
			if _,e:=client.Write(data);e!=nil{t.Fatal(e)}
			buf:=make([]byte,65535); n,e:=client.Read(buf)
			if e!=nil || !bytes.Equal(buf[:n],data){t.Fatalf("client %d echo len=%d err=%v",i,n,e)}
		}
	}
	deadline:=time.Now().Add(time.Second)
	for {
		_,err=flows[0].WriteTo([]byte("closed?"),clients[0].LocalAddr())
		if errors.Is(err,net.ErrClosed){break}
		if time.Now().After(deadline){t.Fatal("idle flow was not closed")}
		time.Sleep(10*time.Millisecond)
	}
}

func TestShareFailedProtocolAdditionPreservesWorkingTCP(t *testing.T) {
	listener := newFakeListener()
	manager := NewManager(stubDialer{},nil,nil,func(network,_ string)(net.Listener,error){
		if network=="udp" {return nil,errors.New("UDP occupied")}; return listener,nil
	})
	defer manager.Close(context.Background())
	rule:=sharedRule(8000,config.ProtocolTCP)
	change,err:=manager.Prepare([]config.Rule{rule});if err!=nil{t.Fatal(err)};change.Commit()
	rule.Protocol=config.ProtocolBoth
	if _,err:=manager.Prepare([]config.Rule{rule});err==nil{t.Fatal("want failure")}
	if listener.isClosed() || manager.Rules()[0].Protocol!=config.ProtocolTCP {t.Fatal("previous TCP rule lost")}
}

func TestShareConnectionLimitAndRemoval(t *testing.T) {
	local,err:=net.Listen("tcp","127.0.0.1:0");if err!=nil{t.Fatal(err)};defer local.Close()
	listener:=newShareListener()
	manager:=NewManager(stubDialer{},nil,nil,func(string,string)(net.Listener,error){return listener,nil})
	manager.shareConnectionLimit=1
	defer manager.Close(context.Background())
	change,err:=manager.Prepare([]config.Rule{sharedRule(local.Addr().(*net.TCPAddr).Port,config.ProtocolTCP)})
	if err!=nil{t.Fatal(err)};change.Commit()
	first,flow:=net.Pipe();defer first.Close();listener.connections<-flow
	_ = local.(*net.TCPListener).SetDeadline(time.Now().Add(time.Second))
	accepted,err:=local.Accept();if err!=nil{t.Fatal(err)};defer accepted.Close()
	second,overflow:=net.Pipe();defer second.Close();listener.connections<-overflow
	_ = second.SetReadDeadline(time.Now().Add(time.Second))
	if _,err=second.Read(make([]byte,1));err!=io.EOF{t.Fatalf("overflow error=%v",err)}
	change,err=manager.Prepare(nil);if err!=nil{t.Fatal(err)};change.Commit()
	_ = first.SetReadDeadline(time.Now().Add(time.Second))
	if _,err=first.Read(make([]byte,1));err!=io.EOF{t.Fatalf("removed flow error=%v",err)}
}
