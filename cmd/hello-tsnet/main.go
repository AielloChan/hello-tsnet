package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"hello-tsnet/internal/app"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	options, err := app.DefaultOptions(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "参数错误：%v\n", err)
		return 2
	}
	application, err := app.New(options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化失败：%v\n", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Printf("hello-tsnet 管理页面：http://%s\n", options.WebAddress)
	if err := application.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "运行失败：%v\n", err)
		return 1
	}
	return 0
}
