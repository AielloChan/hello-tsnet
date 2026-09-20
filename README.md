# Hello TSNet

一个轻量的 macOS TCP/UDP 转发器：访问 Tailscale 设备，也能向其他设备共享本机服务。

它把官方 [`tsnet`](https://tailscale.com/docs/features/tsnet) 直接嵌入单个 Go 进程，不需要安装 Tailscale 客户端、系统扩展、Docker 或 Linux 虚拟机。管理页面只监听 `127.0.0.1`，可动态添加和删除转发规则，例如：

```text
127.0.0.1:30001  →  screen-share.example.net:5900
```

## 支持范围

- 官方 Tailscale 浏览器授权，状态保存在当前用户目录。
- 本地 Web UI 动态管理 TCP、UDP 或两者的转发规则。
- 规则持久化，进程重启后自动恢复。
- 访问设备的规则监听本机 loopback；共享服务的规则监听独立的 Tailscale 节点。
- 单文件二进制，无前端构建步骤。

暂不支持局域网暴露、远程管理和多用户。

## macOS 应用

在 macOS 15 或更新版本上构建菜单栏应用（需要 Go 和 Xcode Command Line Tools）：

```bash
./scripts/build-app.sh
python3 scripts/install-app.py
```

安装脚本固定更新 `/Applications/Hello TSNet.app` 并只启动一次。之后始终打开这个固定路径，不从构建目录、暂存目录或历史副本运行。脚本会先停止该路径的旧实例，备份并更新后校验签名；更新失败则恢复旧文件，不自动重试启动。只更新不启动可加 `--no-open`。

菜单栏显示粉白点阵图标，提供“关于”“控制面板”“退出应用”三个菜单；控制面板通过默认浏览器打开。应用不显示 Dock 图标，关闭浏览器或关于窗口不会停止转发。

应用复用下文的身份与规则目录。首次切换前请先停止旧命令行进程，避免端口占用；选择“退出应用”会同时停止网关。当前采用本机 ad-hoc 签名，未做 Developer ID 签名、公证或登录自启。实现与构建说明见 [`macos/README.md`](macos/README.md)。

## 从设备配置转发

控制面板列出当前节点可见的设备，显示系统、Tailscale IP、DNS 名称、在线/离线/状态未知及可用的最后在线时间，支持按名称或 IP 搜索。设备状态每 10 秒刷新；读取失败保留上次列表并标为未知。

点击设备旁的“添加转发”，自动绑定当前设备，隐藏目标地址输入框，并建议一个未被已有规则使用的本机端口。直接填写名称和目标端口，输入框下方提供常用端口提示；协议可选择 TCP、UDP 或 TCP + UDP，新建默认两者。保存后映射显示在对应设备下，支持编辑和删除；无法明确归属的规则保留在设备区底部的“未关联设备的转发”。端口是否被其他进程占用仍在保存时校验。

旧规则保持 TCP，编辑时可显式切换。选择 TCP + UDP 时，两个协议必须都成功绑定同一本机端口才会保存；任一端口冲突都保留原规则。同一数字端口仍只允许一条规则。UDP 按客户端分隔会话，空闲 60 秒回收，每条映射最多 128 个会话，每会话待发送队列最多 8 个报文，超限丢弃新报文。

规则文件保存为 version 3；version 1 读取时补为 TCP，version 1/2 补为访问远端方向，均只在内存迁移，下次规则修改才写入新版本。回滚至旧二进制前需恢复对应旧规则备份，不能只改版本号。

设备数据来自已登录 tsnet 的本地接口，不需要管理后台 API Key。可见设备不等于有权访问全部端口，设备在线也不等于服务可连接。

## 共享本机服务

在“共享本机服务”点击“添加服务”，只需填写名称、本机端口和协议。例如本地已有 `127.0.0.1:8000` 服务，填写 `8000` 后，其他 Tailscale 设备通过 `hello-tsnet:8000` 访问；Web 服务使用 `http://hello-tsnet:8000`。无需填写第二个端口或目标地址。

默认同时共享 TCP + UDP，也可只选一个协议；应用不会占用本机已有服务的监听端口。服务必须能通过本机 `127.0.0.1` 访问，应用保持运行且对端的 Tailscale 访问策略允许该端口。共享端口支持 1–65535；控制面板自身端口禁止共享。每个协议最多 128 个同时连接/UDP 会话，UDP 空闲 60 秒回收；删除共享会关闭其已有连接。

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
- 访问设备的端口固定监听 `127.0.0.1`；共享服务只在 tsnet 节点监听，固定转到 `127.0.0.1` 同端口，不创建局域网 listener。
- 管理 API 只接受当前监听端口的 `127.0.0.1`/`localhost` Host 与 HTTP Origin 白名单，不启用 CORS。
- 目标服务是否可访问仍由 tailnet ACL/grants 控制。

## 开发验证

```bash
go test -race ./...
go vet ./...
node --check web/app.js
node --test web/devices.test.mjs
```

验收记录见 [`docs/acceptance.md`](docs/acceptance.md)，实现计划见 [`docs/superpowers/plans/2026-09-20-hello-tsnet-mvp.md`](docs/superpowers/plans/2026-09-20-hello-tsnet-mvp.md)。

## 研发交接

后续 agent 开始开发、运维或发布前，先阅读 [`docs/HANDOFF.md`](docs/HANDOFF.md)。该文档包含当前运行状态、架构边界、API、持久化格式、安全约束、验证证据、回滚步骤和后续研发清单。

## 资料

- [Tailscale tsnet](https://tailscale.com/docs/features/tsnet)
- [tsnet.Server API](https://tailscale.com/docs/reference/tsnet-server-api)
