package forward

import (
	"io"
	"net"
	"testing"
	"time"
)

func TestProxyConnPreservesTCPHalfClose(t *testing.T) {
	localClient, localProxy := tcpPair(t)
	remoteProxy, remoteServer := tcpPair(t)
	done := make(chan struct{})
	go func() {
		proxyConn(localProxy, remoteProxy)
		close(done)
	}()

	request := []byte("request")
	if _, err := localClient.Write(request); err != nil {
		t.Fatalf("local Write() error = %v", err)
	}
	if err := localClient.CloseWrite(); err != nil {
		t.Fatalf("local CloseWrite() error = %v", err)
	}
	gotRequest, err := io.ReadAll(remoteServer)
	if err != nil {
		t.Fatalf("remote ReadAll() error = %v", err)
	}
	if string(gotRequest) != string(request) {
		t.Fatalf("remote request = %q, want %q", gotRequest, request)
	}

	response := []byte("response after request EOF")
	if _, err := remoteServer.Write(response); err != nil {
		t.Fatalf("remote Write() error = %v", err)
	}
	if err := remoteServer.CloseWrite(); err != nil {
		t.Fatalf("remote CloseWrite() error = %v", err)
	}
	gotResponse, err := io.ReadAll(localClient)
	if err != nil {
		t.Fatalf("local ReadAll() error = %v", err)
	}
	if string(gotResponse) != string(response) {
		t.Fatalf("local response = %q, want %q", gotResponse, response)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("proxyConn() did not return")
	}
}

func tcpPair(t *testing.T) (*net.TCPConn, *net.TCPConn) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	accepted := make(chan *net.TCPConn, 1)
	errs := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			errs <- err
			return
		}
		accepted <- conn.(*net.TCPConn)
	}()
	clientConn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	client := clientConn.(*net.TCPConn)
	var server *net.TCPConn
	select {
	case server = <-accepted:
	case err := <-errs:
		t.Fatalf("Accept() error = %v", err)
	case <-time.After(time.Second):
		t.Fatal("Accept() timed out")
	}
	t.Cleanup(func() { _ = client.Close() })
	t.Cleanup(func() { _ = server.Close() })
	return client, server
}
