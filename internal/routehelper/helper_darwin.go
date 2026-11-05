package routehelper

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/net/route"
	"golang.org/x/sys/unix"
)

// ServeRouteSession is called by the administrator-installed, socket-activated
// helper. The approved user's socket owns a single interface for its lifetime.
func ServeRouteSession(conn *net.UnixConn, owner uint32) (result error) {
	defer conn.Close()
	uid, err := socketUID(conn)
	if err != nil || uid != owner || owner == 0 {
		return errors.New("unauthorized route client")
	}
	defer func() { result = errors.Join(result, ClearHosts()) }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	reader := bufio.NewReaderSize(conn, messageLimit)
	var local []netip.Addr
	if err := readMessage(reader, &local); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	if err := validateLocal(local); err != nil {
		return err
	}
	file, err := configureInterface(local)
	if err != nil {
		if _, _, writeErr := conn.WriteMsgUnix([]byte{'E'}, nil, nil); writeErr == nil {
			_ = json.NewEncoder(conn).Encode(err.Error())
		}
		return err
	}
	defer file.Close()

	if _, _, err := conn.WriteMsgUnix([]byte{'T'}, unix.UnixRights(int(file.Fd())), nil); err != nil {
		return err
	}
	iface, err := net.InterfaceByName(file.Name())
	if err != nil {
		return err
	}
	current := routeSet{installed: make(map[netip.Addr]bool)}
	for {
		_ = conn.SetDeadline(time.Now().Add(controlTimeout))
		var raw json.RawMessage
		if err := readMessage(reader, &raw); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		var update peerUpdate
		if len(raw) != 0 && raw[0] == '{' {
			err = json.Unmarshal(raw, &update)
		} else {
			err = json.Unmarshal(raw, &update.Peers)
		}
		if err != nil {
			return err
		}
		if err := validatePeers(local, update.Peers); err != nil {
			return err
		}
		if len(update.Hosts) > maxRouteAddresses {
			return errors.New("too many host entries")
		}
		actual, err := interfaceRoutes(iface.Index)
		if err != nil {
			return err
		}
		for addr := range current.installed {
			if !actual[addr] {
				delete(current.installed, addr)
			}
		}
		failed := 0
		_ = current.update(update.Peers, func(action string, addr netip.Addr) error {
			if err := runSystem("/sbin/route", "-q", "-n", action, family(addr), "-host", addr.String(),
				"-interface", iface.Name); err != nil {
				failed++
				return err
			}
			return nil
		})
		reply := Reply{Routed: current.addresses(), Failed: failed}
		if err := syncHosts(hostsPath, update.Hosts, current.installed); err != nil {
			reply.HostsError = err.Error()
		}
		if err := json.NewEncoder(conn).Encode(reply); err != nil {
			return err
		}
	}
}

func configureInterface(local []netip.Addr) (*os.File, error) {
	assigned, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	want := addrSet(local)
	for _, a := range assigned {
		if prefix, err := netip.ParsePrefix(a.String()); err == nil && want[prefix.Addr()] {
			return nil, fmt.Errorf("%s 已由其他网络接口使用", prefix.Addr())
		}
	}
	fd, err := unix.Socket(unix.AF_SYSTEM, unix.SOCK_DGRAM, 2)
	if err != nil {
		return nil, err
	}
	unix.CloseOnExec(fd)
	ok := false
	defer func() {
		if !ok {
			_ = unix.Close(fd)
		}
	}()
	info := &unix.CtlInfo{}
	copy(info.Name[:], "com.apple.net.utun_control")
	if err := unix.IoctlCtlInfo(fd, info); err != nil {
		return nil, err
	}
	if err := unix.Connect(fd, &unix.SockaddrCtl{ID: info.Id}); err != nil {
		return nil, err
	}
	name, err := unix.GetsockoptString(fd, 2, 2)
	if err != nil || !isUtun(name) {
		return nil, errors.New("invalid utun name")
	}
	for _, addr := range local {
		host := netip.PrefixFrom(addr, addr.BitLen()).String()
		if err := runSystem("/sbin/ifconfig", name, family(addr)[1:], host, addr.String()); err != nil {
			return nil, err
		}
	}
	if err := runSystem("/sbin/ifconfig", name, "mtu", "1280", "up"); err != nil {
		return nil, err
	}
	ok = true
	return os.NewFile(uintptr(fd), name), nil
}

func isUtun(name string) bool {
	number, ok := strings.CutPrefix(name, "utun")
	if !ok || number == "" {
		return false
	}
	for _, digit := range number {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func family(addr netip.Addr) string {
	if addr.Is4() {
		return "-inet"
	}
	return "-inet6"
}

func runSystem(path string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", path, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func interfaceRoutes(index int) (map[netip.Addr]bool, error) {
	raw, err := route.FetchRIB(unix.AF_UNSPEC, route.RIBTypeRoute, 0)
	if err != nil {
		return nil, err
	}
	messages, err := route.ParseRIB(route.RIBTypeRoute, raw)
	if err != nil {
		return nil, err
	}
	found := make(map[netip.Addr]bool)
	for _, message := range messages {
		item, ok := message.(*route.RouteMessage)
		if !ok || item.Index != index || item.Flags&unix.RTF_HOST == 0 || len(item.Addrs) <= unix.RTAX_DST {
			continue
		}
		switch dst := item.Addrs[unix.RTAX_DST].(type) {
		case *route.Inet4Addr:
			found[netip.AddrFrom4(dst.IP)] = true
		case *route.Inet6Addr:
			found[netip.AddrFrom16(dst.IP)] = true
		}
	}
	return found, nil
}
