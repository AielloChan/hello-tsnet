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
- 管理 API 校验精确 Host 和同源 Origin，不启用 CORS。
- 目标服务是否可访问仍由 tailnet ACL/grants 控制。

## 开发验证

```bash
go test -race ./...
go vet ./...
node --check web/app.js
```

验收记录见 [`docs/acceptance.md`](docs/acceptance.md)，实现计划见 [`docs/superpowers/plans/2026-09-20-hello-tsnet-mvp.md`](docs/superpowers/plans/2026-09-20-hello-tsnet-mvp.md)。

## 资料

- [Tailscale tsnet](https://tailscale.com/docs/features/tsnet)
- [tsnet.Server API](https://tailscale.com/docs/reference/tsnet-server-api)
