package routehelper

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

const (
	messageLimit   = 2 << 20
	controlTimeout = 45 * time.Second
)

type unixSession struct {
	conn      *net.UnixConn
	reader    *bufio.Reader
	withHosts bool
}

func socketPath(uid int) string {
	return "/var/run/org.hellotsnet.app." + strconv.Itoa(uid) + ".sock"
}

func Open(ctx context.Context, local []netip.Addr) (Session, *os.File, error) {
	conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", socketPath(os.Getuid()))
	if err != nil {
		return nil, nil, errors.New("请先安装路由助手")
	}
	socket := conn.(*net.UnixConn)
	stop := context.AfterFunc(ctx, func() { _ = socket.Close() })
	defer stop()
	success := false
	defer func() {
		if !success {
			_ = socket.Close()
		}
	}()
	uid, err := socketUID(socket)
	if err != nil || uid != 0 {
		return nil, nil, errors.New("无法验证路由助手身份")
	}
	_ = socket.SetDeadline(time.Now().Add(10 * time.Second))
	if err := json.NewEncoder(socket).Encode(local); err != nil {
		return nil, nil, fmt.Errorf("初始化路由助手: %w", err)
	}
	file, err := receiveFile(socket)
	if err != nil {
		return nil, nil, err
	}
	success = true
	return &unixSession{conn: socket, reader: bufio.NewReaderSize(socket, messageLimit), withHosts: !NeedsUpgrade() && bundledHelperSHA256 != ""}, file, nil
}

func (s *unixSession) Update(ctx context.Context, snapshot Snapshot) (Reply, error) {
	stop := context.AfterFunc(ctx, s.Close)
	defer stop()
	_ = s.conn.SetDeadline(time.Now().Add(controlTimeout))
	var request any = snapshot.Peers
	if s.withHosts {
		request = peerUpdate{Peers: snapshot.Peers, Hosts: snapshot.Hosts}
	}
	if err := json.NewEncoder(s.conn).Encode(request); err != nil {
		return Reply{}, err
	}
	var reply Reply
	err := readMessage(s.reader, &reply)
	return reply, err
}

func (s *unixSession) Close() { _ = s.conn.Close() }

func readMessage(reader *bufio.Reader, value any) error {
	line, err := reader.ReadSlice('\n')
	if err != nil {
		return err
	}
	return json.Unmarshal(line, value)
}

func socketUID(socket *net.UnixConn) (uint32, error) {
	raw, err := socket.SyscallConn()
	if err != nil {
		return 0, err
	}
	var uid uint32
	var lookupErr error
	err = raw.Control(func(fd uintptr) {
		credentials, err := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if err != nil {
			lookupErr = err
		} else {
			uid = credentials.Uid
		}
	})
	return uid, errors.Join(err, lookupErr)
}

func receiveFile(socket *net.UnixConn) (*os.File, error) {
	var marker [1]byte
	control := make([]byte, unix.CmsgSpace(16))
	n, count, flags, _, err := socket.ReadMsgUnix(marker[:], control)
	if err != nil {
		return nil, errors.New("路由助手未能创建网卡")
	}
	if n == 1 && marker[0] == 'E' && count == 0 {
		var message string
		if err := readMessage(bufio.NewReaderSize(socket, 4096), &message); err != nil {
			return nil, errors.New("路由助手未能创建网卡")
		}
		return nil, errors.New(message)
	}
	messages, err := unix.ParseSocketControlMessage(control[:count])
	if err != nil {
		return nil, err
	}
	var fds []int
	defer func() {
		for _, fd := range fds {
			_ = unix.Close(fd)
		}
	}()
	for _, message := range messages {
		rights, err := unix.ParseUnixRights(&message)
		if err != nil {
			return nil, err
		}
		fds = append(fds, rights...)
	}
	if flags&unix.MSG_CTRUNC != 0 || n != 1 || marker[0] != 'T' || len(fds) != 1 {
		return nil, errors.New("路由助手返回了无效的网卡描述符")
	}
	fd := fds[0]
	name, err := unix.GetsockoptString(fd, 2, 2)
	if err != nil || !isUtun(name) {
		return nil, errors.New("路由助手返回的描述符不是 utun")
	}
	unix.CloseOnExec(fd)
	if err := unix.SetNonblock(fd, true); err != nil {
		return nil, err
	}
	fds = nil // Ownership transfers to the returned file.
	return os.NewFile(uintptr(fd), name), nil
}
