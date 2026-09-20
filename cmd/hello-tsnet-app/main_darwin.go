//go:build darwin && cgo

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"hello-tsnet/internal/app"
	"hello-tsnet/internal/desktop"
	"hello-tsnet/internal/menubar"
)

func init() { runtime.LockOSThread() }

func main() { os.Exit(run()) }

func run() int {
	options, err := app.DefaultOptions(os.Args[1:])
	if err != nil {
		menubar.ShowError(err)
		return 2
	}
	application, err := app.New(options)
	if err != nil {
		menubar.ShowError(err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := desktop.Run(ctx, application, &menubar.Menu{ControlURL: "http://" + options.WebAddress}); err != nil {
		fmt.Fprintf(os.Stderr, "运行失败：%v\n", err)
		return 1
	}
	return 0
}
