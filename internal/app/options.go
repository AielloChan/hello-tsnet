package app

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var hostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,62}$`)

type Options struct {
	DataDir  string
	Hostname string
}

type Paths struct {
	StateDir string
}

func ParseOptions(args []string, userConfigDir string) (Options, error) {
	options := Options{DataDir: filepath.Join(userConfigDir, "hello-tsnet"), Hostname: "hello-tsnet"}
	set := flag.NewFlagSet("hello-tsnet", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&options.DataDir, "data-dir", options.DataDir, "directory for Tailscale state")
	set.StringVar(&options.Hostname, "hostname", options.Hostname, "tailnet node hostname")
	if err := set.Parse(args); err != nil {
		return Options{}, err
	}
	if set.NArg() != 0 {
		return Options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(set.Args(), " "))
	}
	if strings.TrimSpace(options.DataDir) == "" {
		return Options{}, fmt.Errorf("data directory is required")
	}
	if !hostnamePattern.MatchString(options.Hostname) {
		return Options{}, fmt.Errorf("hostname must contain only letters, numbers, and hyphens")
	}
	options.DataDir = filepath.Clean(options.DataDir)
	return options, nil
}

func DefaultOptions(args []string) (Options, error) {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return Options{}, fmt.Errorf("find user config directory: %w", err)
	}
	return ParseOptions(args, userConfigDir)
}

func PreparePaths(base string) (Paths, error) {
	paths := Paths{StateDir: filepath.Join(base, "tsnet")}
	if err := os.MkdirAll(paths.StateDir, 0o700); err != nil {
		return Paths{}, fmt.Errorf("create data directories: %w", err)
	}
	for _, dir := range []string{base, paths.StateDir} {
		if err := os.Chmod(dir, 0o700); err != nil {
			return Paths{}, fmt.Errorf("set private directory permissions on %s: %w", dir, err)
		}
	}
	return paths, nil
}
