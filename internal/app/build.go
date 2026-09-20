package app

import (
	"net"
	"net/http"
	"path/filepath"
	"time"

	"hello-tsnet/internal/config"
	"hello-tsnet/internal/forward"
	"hello-tsnet/internal/rules"
	"hello-tsnet/internal/tailnet"
	"hello-tsnet/internal/web"
)

func New(options Options) (*App, error) {
	paths, err := PreparePaths(options.DataDir)
	if err != nil {
		return nil, err
	}
	store := config.NewStore(paths.RulesFile)
	runtime := tailnet.New(paths.StateDir, options.Hostname)
	manager := forward.NewManager(runtime, net.Listen)
	service := rules.NewService(store, managerPreparer{manager: manager})
	handler := web.NewHandler(options.WebAddress, service, func() web.Status {
		return webStatus(runtime.Status())
	}, runtime.Devices)
	httpServer := &http.Server{
		Addr:              options.WebAddress,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return &App{
		InstancePath:    filepath.Join(options.DataDir, "instance.lock"),
		Address:         options.WebAddress,
		Listen:          net.Listen,
		Loader:          store,
		Restorer:        service,
		Runtime:         runtime,
		Manager:         manager,
		Server:          &HTTPServerAdapter{Server: httpServer},
		ShutdownTimeout: options.ShutdownTimeout,
	}, nil
}

type managerPreparer struct {
	manager *forward.Manager
}

func (p managerPreparer) Prepare(items []config.Rule) (rules.Change, error) {
	return p.manager.Prepare(items)
}

func webStatus(status tailnet.Status) web.Status {
	result := web.Status{AuthURL: status.AuthURL, Error: status.Error}
	switch status.State {
	case tailnet.StateReady:
		result.State = web.StateReady
	case tailnet.StateError:
		result.State = web.StateError
	default:
		result.State = web.StatePending
	}
	return result
}
