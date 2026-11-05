package direct

import (
	"context"
	"fmt"
	"hello-tsnet/internal/routehelper"
	"net/netip"
	"slices"
	"time"
)

func (d *Device) Sync(ctx context.Context, fetch func(context.Context) (routehelper.Snapshot, error), events <-chan struct{}, report func(string, bool)) {
	var session routehelper.Session
	var known routehelper.Snapshot
	var routed []netip.Addr
	closeSession := func() {
		d.setFile(nil)
		if session != nil {
			session.Close()
			session = nil
		}
		routed = nil
	}
	defer closeSession()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		snapshotCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		next, fetchErr := fetch(snapshotCtx)
		cancel()
		if fetchErr == nil {
			if !slices.Equal(known.Local, next.Local) {
				closeSession()
			}
			known = next
		}
		d.dispatch.setAddresses(known.Local, routed)
		if len(known.Local) > 0 {
			if session == nil {
				connected, file, err := routehelper.Open(ctx, known.Local)
				if err != nil {
					report("设备直访尚未就绪："+err.Error(), false)
				} else {
					session = connected
					d.setFile(file)
				}
			}
			if session != nil {
				reply, err := session.Update(ctx, known)
				if err != nil {
					closeSession()
					d.dispatch.setAddresses(known.Local, nil)
					report("设备直访暂不可用："+err.Error(), false)
				} else {
					want := addrSet(known.Peers)
					routed = slices.DeleteFunc(reply.Routed, func(addr netip.Addr) bool { return !want[addr] })
					d.dispatch.setAddresses(known.Local, routed)
					message := fmt.Sprintf("设备直访已启用 · %d 个地址", len(routed))
					if reply.Failed > 0 {
						message += fmt.Sprintf(" · %d 条路由待同步", reply.Failed)
					}
					if fetchErr != nil {
						message += " · 正在恢复设备同步"
					}
					if reply.HostsError != "" {
						message += " · 主机名同步失败：" + reply.HostsError
					}
					report(message, fetchErr == nil && reply.Failed == 0 && len(routed) == len(known.Peers))
				}
			}
		} else {
			report("等待 Tailnet 地址", false)
		}
		select {
		case <-ctx.Done():
			return
		case <-events:
		case <-ticker.C:
		}
	}
}
