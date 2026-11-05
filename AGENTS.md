# AGENTS.md

## 项目目标

Hello TSNet 是 macOS 菜单栏应用。它用官方 `tailscale.com/tsnet` 加入 Tailnet，同步可见设备的主机路由，并把 Tailnet 允许的入站 TCP/UDP 连接转发至本机同号端口。管理窗口使用 AppKit 和进程内接口。

继续开发、运行维护、发布或排查前，先阅读 [研发交接](docs/HANDOFF.md)。

## 工程约束

- 主应用以普通用户权限运行。管理员助手经 Unix socket 验证对端 UID，只配置自己创建的 utun、Tailnet 设备的精确主机路由和受管 hosts 区块。
- 保持系统默认路由、DNS 和防火墙配置。退出时释放助手创建的网卡及路由。
- 管理界面保留在原生进程内。入站转发遵守 Tailnet 访问策略，目标固定为 `127.0.0.1` 同号端口。
- 优先复用 Go 标准库、官方 tsnet 和项目已使用的依赖。会话、消息和关闭等待需要有界。
- Tailscale 状态存放在用户数据目录，权限不得宽于 `0700`/`0600`。旧规则文件仅供回退。
- 不记录或回显 auth key、cookie、节点私钥及其他凭据。
- 修改行为时先写失败测试，确认失败原因正确后再写实现。
- 禁止主动格式化无关代码，保持最小范围修改。

## 验证

提交前至少执行：

```bash
go test ./...
go vet ./...
go build ./cmd/hello-tsnet
```

原生窗口运行 `./scripts/test-menubar.sh`。真实网卡和 Tailnet 的验证按手工验收记录。

## Git

- 使用 Conventional Commits。
- description、body、footer 使用中文。
- 不提交 Tailscale 状态、auth key、本机规则文件或编译产物。
