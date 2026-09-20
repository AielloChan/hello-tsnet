package app

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const defaultShutdownTimeout = 5 * time.Second

var hostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,62}$`)

type Options struct {
	DataDir         string
	WebAddress      string
	Hostname        string
	ShutdownTimeout time.Duration
}

type Paths struct {
	StateDir string
	RulesFile string
}

func ParseOptions(args []string, userConfigDir string) (Options, error) {
	defaults := Options{
		DataDir:         filepath.Join(userConfigDir, "hello-tsnet"),
		WebAddress:      "127.0.0.1:6060",
		Hostname:        "hello-tsnet",
		ShutdownTimeout: defaultShutdownTimeout,
	}
	set := flag.NewFlagSet("hello-tsnet", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&defaults.DataDir, "data-dir", defaults.DataDir, "directory for rules and Tailscale state")
	set.StringVar(&defaults.WebAddress, "listen", defaults.WebAddress, "local web address")
	set.StringVar(&defaults.Hostname, "hostname", defaults.Hostname, "tailnet node hostname")
	if err := set.Parse(args); err != nil {
		return Options{}, err
	}
	if set.NArg() != 0 {
		return Options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(set.Args(), " "))
	}
	host, port, err := net.SplitHostPort(defaults.WebAddress)
	if err != nil || host != "127.0.0.1" || port == "" {
		return Options{}, fmt.Errorf("listen address must be 127.0.0.1:<port>")
	}
	if strings.TrimSpace(defaults.DataDir) == "" {
		return Options{}, fmt.Errorf("data directory is required")
	}
	if !hostnamePattern.MatchString(defaults.Hostname) {
		return Options{}, fmt.Errorf("hostname must contain only letters, numbers, and hyphens")
	}
	defaults.DataDir = filepath.Clean(defaults.DataDir)
	return defaults, nil
}

func DefaultOptions(args []string) (Options, error) {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return Options{}, fmt.Errorf("find user config directory: %w", err)
	}
	return ParseOptions(args, userConfigDir)
}

func PreparePaths(base string) (Paths, error) {
	paths := Paths{
		StateDir: filepath.Join(base, "tsnet"),
		RulesFile: filepath.Join(base, "rules.json"),
	}
	if err := os.MkdirAll(paths.StateDir, 0o700); err != nil {
		return Paths{}, fmt.Errorf("create data directories: %w", err)
	}
	for _, dir := range []string{base, paths.StateDir} {
		if err := os.Chmod(dir, 0o700); err != nil {
			return Paths{}, fmt.Errorf("set private directory permissions on %s: %w", dir, err)
		}
	}
	if _, err := os.Stat(paths.RulesFile); err == nil {
		if err := os.Chmod(paths.RulesFile, 0o600); err != nil {
			return Paths{}, fmt.Errorf("set private rules permissions: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return Paths{}, fmt.Errorf("inspect rules file: %w", err)
	}
	return paths, nil
}
