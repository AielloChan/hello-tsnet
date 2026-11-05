//go:build darwin && cgo

package main

/*
#include <launch.h>
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
	"unsafe"

	"hello-tsnet/internal/routehelper"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if os.Geteuid() != 0 || len(args) != 1 {
		return errors.New("expected launchd with an approved user ID")
	}
	if args[0] == "--clear-hosts" {
		return routehelper.ClearHosts()
	}
	owner, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil || owner == 0 {
		return errors.New("invalid approved user ID")
	}
	name := C.CString("RouteControl")
	defer C.free(unsafe.Pointer(name))
	var fds *C.int
	var count C.size_t
	if result := C.launch_activate_socket(name, &fds, &count); result != 0 {
		return fmt.Errorf("activate route socket: %w", syscall.Errno(result))
	}
	defer C.free(unsafe.Pointer(fds))
	if count != 1 {
		return errors.New("expected one launchd socket")
	}
	syscall.CloseOnExec(int(*fds))
	file := os.NewFile(uintptr(*fds), "RouteControl")
	listener, err := net.FileListener(file)
	_ = file.Close()
	if err != nil {
		return err
	}
	defer listener.Close()
	socket, ok := listener.(*net.UnixListener)
	if !ok {
		return errors.New("route control socket is not a Unix socket")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	cancelListener := context.AfterFunc(ctx, func() { _ = socket.Close() })
	defer cancelListener()
	if err := routehelper.ClearHosts(); err != nil {
		log.Printf("clear stale hosts entries: %v", err)
	}
	if err := socket.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	conn, err := socket.AcceptUnix()
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, os.ErrDeadlineExceeded) {
			return nil
		}
		return err
	}
	cancelConn := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer cancelConn()
	return routehelper.ServeRouteSession(conn, uint32(owner))
}
