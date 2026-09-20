# AGENTS.md

## 项目目标

`hello-tsnet` 是一个仅运行在当前用户空间的 macOS TCP/UDP 转发网关。它通过官方 `tailscale.com/tsnet` 嵌入 Tailscale，不安装系统扩展、网络接口或后台守护进程。

继续开发、运行维护、发布或排查前，先阅读 [`docs/HANDOFF.md`](docs/HANDOFF.md)。该文档是当前运行状态、关键设计决策、验证证据、回滚方式和后续工作的入口。

## 工程约束

- 使用 Go 标准库和官方 `tailscale.com/tsnet`，除非确有必要，不增加第三方依赖。
- 管理界面只能监听 loopback；不得暴露到局域网或 tailnet，共享规则必须拒绝管理界面的端口。
- 访问远端规则绑定 loopback；共享本机服务规则绑定 tsnet 节点，同端口转到 127.0.0.1，必须显式选择共享方向。
- Tailscale 状态和转发规则必须存放在用户数据目录，权限不得宽于 `0700`/`0600`。
- 不记录或回显 auth key、cookie、节点私钥及其他凭据。
- 支持 TCP、UDP 或两者；新规则默认两者，旧规则保持 TCP。不要提前加入 HTTP 反向代理、多用户或远程管理。
- 规则变更必须通过协调器原子生效；失败时保留此前仍可工作的规则。
- 修改行为时先写失败测试，确认失败原因正确后再写实现。
- 禁止主动格式化无关代码，保持最小范围修改。

## 分层

- `cmd/hello-tsnet`: 参数解析、信号转交和退出码，不承载业务装配。
- `internal/config`: 规则模型、校验和 JSON 持久化。
- `internal/forward`: loopback/tsnet 监听器、双向 TCP 转发与有界 UDP 会话。
- `internal/rules`: 串行规则写入，以及 Prepare → Save → Commit 编排。
- `internal/tailnet`: tsnet 生命周期、授权状态、拨号和节点监听适配。
- `internal/web`: loopback 管理 API 与嵌入式页面。
- `internal/app`: 唯一的依赖装配、启动顺序和有界关闭实现。
- `web`: 无构建步骤的静态资源。

规则写操作必须集中在单一 service 中串行执行。协调器先准备所有可能失败的 listener，规则文件保存成功后再执行不返回错误的内存 commit；不得用“先部分生效、失败再回滚”模拟事务。

## 验证

提交前至少执行：

```bash
go test ./...
go vet ./...
go build ./cmd/hello-tsnet
```

涉及真实 tailnet 的验证必须明确标记为手工验收，不得用单元测试结果替代。

## Git

- 使用 Conventional Commits。
- description、body、footer 使用中文。
- 不提交 Tailscale 状态、auth key、本机规则文件或编译产物。
