# hello-tsnet

一个轻量的本机 TCP → Tailscale 网络转发器。

它把官方 [`tsnet`](https://tailscale.com/docs/features/tsnet) 直接嵌入单个 Go 进程，不需要安装 Tailscale 客户端、系统扩展、Docker 或 Linux 虚拟机。管理页面只监听 `127.0.0.1`，可动态添加和删除转发规则，例如：

```text
127.0.0.1:30001  →  screen-share.example.net:5900
```

## 第一版范围

- 官方 Tailscale 浏览器授权，状态保存在当前用户目录。
- 本地 Web UI 动态管理 TCP 转发规则。
- 规则持久化，进程重启后自动恢复。
- 每条规则只监听本机 loopback。
- 单文件二进制，无前端构建步骤。

暂不支持 UDP、局域网暴露、远程管理和多用户。

## macOS 应用

在 macOS 15 或更新版本上构建菜单栏应用（需要 Go 和 Xcode Command Line Tools）：

```bash
./scripts/build-app.sh
python3 scripts/install-app.py
```

安装脚本固定更新 `~/Applications/hello-tsnet.app` 并只启动一次。之后始终打开这个固定路径，不从构建目录、暂存目录或历史副本运行。脚本会先停止该路径的旧实例，备份并更新后校验签名；更新失败则恢复旧文件，不自动重试启动。只更新不启动可加 `--no-open`。

菜单栏显示粉白点阵图标，提供“关于”“控制面板”“退出应用”三个菜单；控制面板通过默认浏览器打开。应用不显示 Dock 图标，关闭浏览器或关于窗口不会停止转发。

应用复用下文的身份与规则目录。首次切换前请先停止旧命令行进程，避免端口占用；选择“退出应用”会同时停止网关。当前采用本机 ad-hoc 签名，未做 Developer ID 签名、公证或登录自启。实现与构建说明见 [`macos/README.md`](macos/README.md)。

## 从设备配置转发

控制面板列出当前节点可见的设备，显示系统、Tailscale IP、DNS 名称、在线/离线/状态未知及可用的最后在线时间，支持按名称或 IP 搜索。设备状态每 10 秒刷新；读取失败保留上次列表并标为未知。

点击设备旁的“添加转发”，自动填写目标地址并建议一个未被已有规则使用的本机端口。可选择 SSH、屏幕共享/VNC、远程桌面/RDP，或自定义 TCP 端口。保存后映射显示在对应设备下，支持编辑和删除；无法明确归属的规则保留在“其他映射”。端口是否被其他进程占用仍在保存时校验。

设备数据来自已登录 tsnet 的本地接口，不需要管理后台 API Key。可见设备不等于有权访问全部端口，设备在线也不等于服务可连接。

## 构建

需要 Go 1.27.1 或更高版本：

```bash
go mod download
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/hello-tsnet ./cmd/hello-tsnet
```

如果当前 shell 手动设置过错误的 `GOROOT` 或空 `GOPROXY`，可仅对本次构建覆盖，不需要修改全局配置：

```bash
env -u GOROOT GOPROXY=https://proxy.golang.org,direct GOSUMDB=sum.golang.org \
  go build -trimpath -ldflags="-s -w" -o dist/hello-tsnet ./cmd/hello-tsnet
```

## 运行与首次授权

```bash
./dist/hello-tsnet
```

打开 [http://127.0.0.1:6060](http://127.0.0.1:6060)。首次运行时页面会显示 Tailscale 官方授权链接；完成一次授权后，节点身份会持久化，后续启动不再要求登录。

页面中添加示例规则：

```text
名称：Screen Sharing
本机端口：30001
Tailnet 目标：screen-share.example.net:5900
```

随后在 macOS Screen Sharing 中连接：

```text
vnc://127.0.0.1:30001
```

可选参数：

```text
-listen 127.0.0.1:6060
-hostname hello-tsnet
-data-dir ~/Library/Application Support/hello-tsnet
```

程序拒绝监听 `0.0.0.0` 或局域网地址。

## 停止与卸载

前台运行时按 `Ctrl-C` 停止。卸载只需删除二进制；如需同时移除本机保存的独立节点身份和规则，再删除：

```text
~/Library/Application Support/hello-tsnet
```

删除状态目录后，下次运行需要重新授权。还应在 Tailscale 管理后台删除不再使用的 `hello-tsnet` 节点。

## 安全模型

- Tailscale 登录由官方授权页面完成；项目不要求也不保存账号密码。
- tsnet 节点身份存储在 `~/Library/Application Support/hello-tsnet/tsnet`。
- 转发规则存储在 `~/Library/Application Support/hello-tsnet/rules.json`。
- 管理页面默认地址为 `http://127.0.0.1:6060`。
- 所有映射端口固定监听 `127.0.0.1`，不能从局域网直接访问。
- 管理 API 只接受当前监听端口的 `127.0.0.1`/`localhost` Host 与 HTTP Origin 白名单，不启用 CORS。
- 目标服务是否可访问仍由 tailnet ACL/grants 控制。

## 开发验证

```bash
go test -race ./...
go vet ./...
node --check web/app.js
```

验收记录见 [`docs/acceptance.md`](docs/acceptance.md)，实现计划见 [`docs/superpowers/plans/2026-09-20-hello-tsnet-mvp.md`](docs/superpowers/plans/2026-09-20-hello-tsnet-mvp.md)。

## 研发交接

后续 agent 开始开发、运维或发布前，先阅读 [`docs/HANDOFF.md`](docs/HANDOFF.md)。该文档包含当前运行状态、架构边界、API、持久化格式、安全约束、验证证据、回滚步骤和后续研发清单。

## 资料

- [Tailscale tsnet](https://tailscale.com/docs/features/tsnet)
- [tsnet.Server API](https://tailscale.com/docs/reference/tsnet-server-api)
