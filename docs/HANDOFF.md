# Hello TSNet 研发交接

应用固定安装路径为 `/Applications/Hello TSNet.app`。安装器会备份旧包，`--no-open` 只更新文件。启动新版本前结束使用同一个身份目录的旧版本或实验程序。应用和 CLI 共用用户目录中的 `instance.lock`。

原生控制面板使用 AppKit，提供设备和设置两页。设备列表的 IPv4 和 IPv6 地址可分别点击复制。设置显示当前 Tailscale 账号，并提供退出登录。退出后关闭控制面板，菜单里的「控制面板」切换为「授权登录」，授权通过官方浏览器页面完成。关闭设备窗口时应用继续运行。

设置页可安装或卸载路由助手。助手缺失时，菜单栏会提供安装入口。菜单栏图标在连接 Tailnet 或同步路由时闪动，路由就绪后固定点亮上排中央和中排三颗粉色点。

旧版助手仍可配置 IP 路由。在设置页更新助手后，已成功配置路由的设备也可以通过完整 `.ts.net` 域名和带 `.local` 后缀的主机名访问。助手只管理 `/etc/hosts` 中 `# BEGIN Hello TSNet` 与 `# END Hello TSNet` 之间的条目。已有同名条目优先，重复的短名只保留完整域名。退出应用、退出登录或卸载助手时会清理受管条目。

启动时一个 `tsnet.Server` 持有同一个节点身份。服务端在启动前注入自定义 TUN，将本机发起的原生 IP 流量和入站同端口代理的回复送入 Tailscale。解密且通过 Tailnet 策略的回包交给 macOS 网卡。远端主动发起的 TCP、UDP 连接通过有界的用户态网络栈转到本机 `127.0.0.1` 同号端口。屏幕共享的原生回程允许媒体先于本机 UDP 发包到达。

- `internal/tailnet` 管理 tsnet、设备快照和设备变更通知。通知失效或断开时有周期校对。
- `internal/direct` 分派原生连接回包和远端发起的连接，维护网卡会话。
- `internal/routehelper` 实现经 Unix socket 验证 UID 的助手协议和主机路由增量更新。
- `internal/forward` 接受用户态 TCP、UDP 流并双向转发到 loopback。并发会话有上限，空闲 UDP 会话自动回收。
- `internal/menubar` 与 `internal/desktop` 管理原生界面和退出生命周期。

首次启动时，应用提示安装助手，经 macOS 管理员授权后运行应用包内的安装脚本。CLI 也可通过 `sudo /bin/bash scripts/route-helper.sh install` 安装。助手安装到 root 所有的路径，由 launchd 的 Unix socket 激活。它验证获准 UID 和地址范围，并传递一张 utun 的文件描述符。App 从当前设备清单生成路由，每个目标是 IPv4 `/32` 或 IPv6 `/128`。已有冲突路由保留并报告待同步。会话关闭会释放网卡。App 本身没有管理 HTTP 端口。Tailscale 的 WireGuard 传输仍使用网络套接字。

旧版规则文件 `~/Library/Application Support/hello-tsnet/rules.json` 只供回退。新版从同一目录复用 `tsnet/`，不读取旧规则。回退时退出新版，卸载助手，再恢复安装器保留的旧包。

手工验收应使用已授权的真实 Tailnet：

1. 按 [安装步骤](../README.md) 构建并安装新版及助手。从菜单打开原生设备窗口，查看本机 IP 和已同步的地址数量。
2. 核对 `route -n get <设备 IPv4>` 和 `route -n get -inet6 <设备 IPv6>` 指向同一张 utun。设备加入、移除或换地址后重新核对。仅离线的设备保留路由。
3. 从 A 用目标真实 IP 连接 TCP、UDP 服务。向 Mac mini 建立高性能屏幕共享，操作并重连。
4. 从 Tailnet 的另一台设备访问 A 上仅监听 `127.0.0.1` 的 TCP、UDP 服务，使用 A 的 Tailscale 地址和原端口。验证 Tailnet 拒绝的访问无法到达。
5. 退出 App，核对助手创建的路由和 utun 已清理，原生设备面板始终没有占用 TCP 或 UDP 管理端口。

仓库测试验证原生菜单和窗口、设备地址快照、路由增量与重试，以及用户态数据经过本机真实 TCP/UDP 服务的同端口转发。该覆盖范围与上述手工验收分开记录。
