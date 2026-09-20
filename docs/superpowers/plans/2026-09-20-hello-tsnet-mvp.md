# hello-tsnet MVP Implementation Plan

> **For agentic workers:** Use subagents only for work that is independent, parallel-safe, and provides a clear quality or latency benefit; otherwise execute locally. `[x]` 表示已有对应实现或验证；`[ ]` 表示交接时仍缺少计划中明确要求的回归覆盖。

**Goal:** 构建一个原生 macOS 单进程应用，通过官方 tsnet 将本机 loopback TCP 端口动态转发到 tailnet 目标。

**Architecture:** 进程由六个职责单一的部分组成：JSON 配置仓储、串行规则服务、TCP 转发协调器、loopback Web/API、tsnet 运行时适配器、应用装配层。`cmd` 只负责参数解析、信号转交和退出码。Web 层只调用规则服务；规则服务按 Prepare → Save → Commit 串行提交；所有出站连接统一通过 `tsnet.Server.Dial`，不创建系统网络接口。

**Tech Stack:** Go 1.27、Go 标准库、`tailscale.com/tsnet` v1.102.4、HTML/CSS/原生 JavaScript。

**Resource Budget:** 登录稳定后，空闲 RSS 目标小于 200 MiB，60 秒平均 CPU 小于单个逻辑核心的 0.5%，release 二进制小于 60 MiB，状态目录小于 10 MiB。任何超标都必须在验收报告中解释，不能只记录而不判断。

**Status:** 已于 2026-09-20 完成。自动验证、真实 Tailscale 授权、Screen Sharing、重启恢复、资源测量和独立 review 均已通过；证据见 `docs/acceptance.md`。

---

## Chunk 1: Core behavior

### Task 1: 规则模型与持久化

**Files:**
- Create: `go.mod`
- Create: `internal/config/rule.go`
- Create: `internal/config/rule_test.go`
- Create: `internal/config/store.go`
- Create: `internal/config/store_test.go`

- [x] 写失败测试：接受名称、本地端口与 `host:port` 目标；拒绝空名称、特权/越界端口、重复端口和无效目标。
- [x] 运行 `go test ./internal/config`，确认测试因实现缺失而失败。
- [x] 实现 `Rule`、`ValidateRule`、`ValidateRules`；规则 ID 使用 `crypto/rand` 生成，不引入 UUID 依赖。
- [x] 写失败测试：不存在的配置文件返回空规则；保存后可读取；文件权限为 `0600`；替换通过临时文件加 rename 完成。
- [x] 运行测试确认预期失败，再实现 `Store.Load` 和 `Store.Save`。
- [x] 运行 `go test ./internal/config`，确认全部通过。

### Task 2: 动态 TCP 转发协调器

**Files:**
- Create: `internal/forward/manager.go`
- Create: `internal/forward/manager_test.go`
- Create: `internal/forward/proxy.go`
- Create: `internal/forward/proxy_test.go`

- [x] 以 `Dialer` 和 `ListenFunc` 接口写失败测试：新增规则准备 listener；Prepare 后不启动 accept/dial；Abort 关闭全部暂存 listener；Commit 一次发布完整规则快照后才启动 accept；删除规则在 commit 后关闭 listener；不变规则不重启；同端口目标变更复用 listener；多 listener 准备中途失败会关闭全部暂存 listener 且不改变现状。
- [x] 运行 `go test ./internal/forward`，确认测试因实现缺失而失败。
- [x] 实现 `Manager.Prepare(rules) (*ChangeSet, error)`；固定监听 `127.0.0.1:<port>`，禁止调用方传入 bind host。Prepare 只绑定端口，禁止启动 accept loop。`ChangeSet.Abort()` 仅关闭暂存 listener，`ChangeSet.Commit()` 不返回错误并在单个临界区发布完整目标快照，随后启动新增 accept loop 并关闭删除项。
- [x] 写失败测试：代理双向复制字节；一端 `CloseWrite` 后对端仍可返回完整响应；两个方向都结束后释放连接；拨号失败只终止当前连接。
- [x] 运行测试确认预期失败，再实现 `proxyConn` 与 accept loop。
- [x] 运行 `go test ./internal/forward`，确认全部通过。

### Task 3: 串行规则服务

**Files:**
- Create: `internal/rules/service.go`
- Create: `internal/rules/service_test.go`

- [x] 写失败测试：并发 Add 被互斥串行化且不丢更新；Prepare 失败不写磁盘；Save 失败会 Abort 且运行状态不变；Save 成功后只调用一次不会失败的 Commit。
- [ ] 补充 Add/Delete 混合并发回归，证明写操作串行化且不会丢更新。
- [x] 运行 `go test ./internal/rules`，确认测试因实现缺失而失败。
- [x] 实现唯一写入口 `Service.Replace`、`Service.Add`、`Service.Delete`，由 service 持有规则快照和互斥锁；Web 层不得直接操作 Store 或 Manager。
- [x] 写失败测试：启动恢复会先完整 Prepare 再一次 Commit；无效或冲突的持久化配置返回清晰错误，不部分启动。
- [x] 运行测试确认预期失败，再实现 `Service.Restore`。
- [x] 运行 `go test ./internal/rules`，确认全部通过。

## Chunk 2: Local management and runtime

### Task 4: loopback Web API 与静态界面

**Files:**
- Create: `internal/web/server.go`
- Create: `internal/web/server_test.go`
- Create: `web/index.html`
- Create: `web/app.js`
- Create: `web/styles.css`
- Create: `web/assets.go`

- [x] 写失败测试：`GET /api/rules`、`POST /api/rules`、`PUT /api/rules/{id}`、`DELETE /api/rules/{id}` 的成功、未找到和校验失败响应。
- [x] 写失败测试：Host 只接受实际监听 authority 对应的 `127.0.0.1`/`localhost`；写请求的 Origin 必须属于同一监听端口的 loopback 白名单；缺失或跨源 Origin、CORS preflight、非 JSON、超过 64 KiB 的 body 均被拒绝。当前实现不要求 Host 与 Origin 使用相同的主机名别名。
- [x] 写失败测试：真实嵌入式静态路由可返回首页；真实 `/api/status` 在 pending 时返回内存授权 URL，在 ready 时隐藏该 URL；两种状态均包含 `Cache-Control: no-store`。
- [x] 运行 `go test ./internal/web`，确认测试因实现缺失而失败。
- [x] 实现 handler，仅通过串行 RulesService 修改规则；所有 API 响应禁用 CORS，并设置合适的内容类型。
- [x] 在 `web/assets.go` 用 `go:embed` 导出只读静态文件系统，并实现无框架 UI：规则新增/编辑表单、pending/ready/error 状态、删除操作和授权链接区域，产物不依赖启动工作目录。
- [x] 授权 URL 只从内存状态读取；状态响应设置 `Cache-Control: no-store`，ready 后不再返回 URL，任何 handler 日志均不得包含 URL。
- [x] 运行 `go test ./internal/web`，确认全部通过。

### Task 5: tsnet 运行时与进程装配

**Files:**
- Create: `internal/tailnet/runtime.go`
- Create: `internal/tailnet/runtime_test.go`
- Create: `cmd/hello-tsnet/main.go`
- Create: `cmd/hello-tsnet/main_test.go`
- Create: `internal/app/app.go`
- Create: `internal/app/app_test.go`
- Modify: `README.md`

- [x] 写失败测试：从 `UserLogf` 捕获且只捕获 `https://login.tailscale.com/` 授权 URL；URL 只存在内存；状态分为 pending/ready/error；ready 时清除 URL；状态快照不会泄漏 auth key。
- [x] 运行测试确认预期失败，再实现 `Runtime`，将 `tsnet.Server.Dir` 指向私有状态目录，将 `Dial` 适配为协调器接口。
- [x] 写失败测试：默认路径、默认 Web 地址和命令行覆盖解析正确。
- [x] 写失败测试：创建或修复应用目录和 tsnet 目录权限为 `0700`，规则文件权限为 `0600`。
- [x] 以注入的 store/service/runtime/server/manager fake 固定启动顺序：预留 Web listener → Store.Load → Service.Restore → 开始 HTTP Serve → runtime Start；覆盖 Load 失败时关闭预留 listener、不继续 Restore/Start。
- [ ] 补充 Service.Restore 失败的装配级回归，确认关闭预留 listener、保留原文件、不调用 Save、不按空规则继续并返回非零退出错误。
- [x] 装配级覆盖 runtime Start 失败后进程仍等待取消，以及退出顺序：停止 Web → 关闭 listener → 以 5 秒 deadline 关闭 manager → 关闭 tsnet。
- [ ] 用真实活跃转发连接覆盖 manager 最多等待 5 秒 drain 后强制关闭；当前装配测试只验证传入了 deadline，没有验证真实连接的 drain/强制关闭路径。
- [x] 运行测试确认预期失败，再在 `internal/app` 装配 store、runtime、manager 和 Web server，严格遵循上述恢复与启动顺序。等待 Running 的逻辑只能在后台更新状态，绝不能阻塞 Web。SIGINT/SIGTERM 使用上述有界退出顺序，VNC 长连接不得阻止进程退出。
- [x] 更新 README：构建、首次登录、添加 `30001 → screen-share.example.net:5900`、运行和卸载说明。
- [x] 运行 `go test ./...`、`go vet ./...`、`go build ./cmd/hello-tsnet`。

## Chunk 3: Acceptance

### Task 6: 本机验收与资源基线

**Files:**
- Create: `docs/acceptance.md`
- Create: `scripts/measure.sh`

- [x] 启动 Web UI，确认只监听 `127.0.0.1:6060`。
- [x] 未授权时确认 Web UI 可用且状态为 pending，授权 URL 不落盘、响应带 `Cache-Control: no-store`。
- [x] 打开官方授权链接并完成节点授权。
- [x] 添加 `30001 → screen-share.example.net:5900`，使用 Screen Sharing 连接 `vnc://127.0.0.1:30001`。
- [x] 重启进程，确认身份和规则恢复且无需重新登录。
- [x] 固定测量契约：先用计划中的 release 命令单独构建并启动被测二进制，再把 PID、二进制路径和状态目录传给 `scripts/measure.sh`。脚本本身不构建；它记录 `sw_vers -productVersion` 与 `uname -m`，在一条 listener、零活跃连接条件下预热 120 秒，以 60 秒前后进程 CPU time 差值计算单核百分比，并用 `ps -o rss=`、`stat -f%z`、`du -sk` 采集 RSS、二进制和状态目录大小。
- [x] 逐项对照 Resource Budget 和 OrbStack 既有基线，明确通过或超标；保留原始命令输出，不用主观观察代替测量。
- [x] 执行完整验证和一次独立只读 review；只修复实质问题并复核受影响部分。
