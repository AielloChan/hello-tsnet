package forward

import (
	"io"
	"net"
	"sync"
)

func proxyConn(left, right net.Conn) {
	var copies sync.WaitGroup
	copies.Add(2)
	go copyHalf(&copies, right, left)
	go copyHalf(&copies, left, right)
	copies.Wait()
	_ = left.Close()
	_ = right.Close()
}

func copyHalf(group *sync.WaitGroup, dst, src net.Conn) {
	defer group.Done()
	_, _ = io.Copy(dst, src)
	if closeWriter, ok := dst.(interface{ CloseWrite() error }); ok {
		_ = closeWriter.CloseWrite()
	} else {
		_ = dst.Close()
	}
	if closeReader, ok := src.(interface{ CloseRead() error }); ok {
		_ = closeReader.CloseRead()
	}
}
