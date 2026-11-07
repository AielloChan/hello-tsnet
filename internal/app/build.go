package app

import (
	"path/filepath"

	"hello-tsnet/internal/tailnet"
)

func New(options Options) (*App, error) {
	paths, err := PreparePaths(options.DataDir)
	if err != nil {
		return nil, err
	}
	runtime := tailnet.New(paths.StateDir, options.Hostname)
	return &App{
		InstancePath: filepath.Join(options.DataDir, "instance.lock"),
		Runtime:      runtime,
		Status:       runtime.Status,
		Devices:      runtime.Devices,
		Ping:         runtime.Ping,
		Login:        runtime.Login,
		Logout:       runtime.Logout,
	}, nil
}
