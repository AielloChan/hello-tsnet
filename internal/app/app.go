package app

import (
	"context"

	"hello-tsnet/internal/tailnet"
)

type Runtime interface {
	Start(context.Context) error
	Close() error
}

type App struct {
	InstancePath string
	Runtime      Runtime
	Status       func() tailnet.Status
	Devices      func(context.Context) ([]tailnet.Device, error)
	Login        func(context.Context) (string, error)
	Logout       func(context.Context) error
}

func (a *App) Run(ctx context.Context) error {
	lock, err := acquireInstance(a.InstancePath)
	if err != nil {
		return err
	}
	defer lock.Close()

	// Keep the native panel available to display a startup error.
	_ = a.Runtime.Start(ctx)
	<-ctx.Done()
	return a.Runtime.Close()
}
