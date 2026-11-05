package routehelper

import (
	"errors"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

const hostsPath = "/etc/hosts"

var errHostsChanged = errors.New("hosts 文件在更新期间发生变化，请重试")

func ClearHosts() error {
	for range 3 {
		if err := syncHosts(hostsPath, nil, nil); !errors.Is(err, errHostsChanged) {
			return err
		}
	}
	return errHostsChanged
}

func syncHosts(path string, entries []HostEntry, routed map[netip.Addr]bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Sys().(*syscall.Stat_t).Uid != 0 {
		return errors.New("hosts 文件必须是 root 所有的普通文件")
	}
	old, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	next, err := managedHosts(string(old), entries, routed)
	if err != nil || next == string(old) {
		return err
	}

	dir, err := os.MkdirTemp(filepath.Dir(path), ".hello-tsnet-hosts.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	staged := filepath.Join(dir, "hosts")
	// Keep the owner, permissions, ACL and extended attributes of the original.
	if err := exec.Command("/bin/cp", "-p", path, staged).Run(); err != nil {
		return err
	}
	if err := os.WriteFile(staged, []byte(next), info.Mode().Perm()); err != nil {
		return err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !os.SameFile(info, current) || info.ModTime() != current.ModTime() || info.Size() != current.Size() {
		return errHostsChanged
	}
	return os.Rename(staged, path)
}
