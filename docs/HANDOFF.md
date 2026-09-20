# Hello TSNet 研发交接

更新日期：2026-09-20

## 接手步骤

1. 阅读根目录 `AGENTS.md`，其约束对所有后续修改生效。
3. 读取 `docs/acceptance.md`，把自动测试、真实 tailnet 验收和资源测量分开理解。
4. 修改行为前先写失败测试；提交前执行本文“验证命令”。
5. 涉及当前运行服务时，先确认 6060、30001 和旧 Docker 容器状态，避免两个网关同时争抢端口。

完成标准：下一位 agent 能从源码和本文重建当前运行状态，不依赖此前聊天记录，也不会覆盖本机 Tailscale 身份或规则文件。

## 当前结论

- MVP 已实现、测试、独立 review、真实授权和真实 VNC 验收。
- 当前方案是一个原生 arm64 Go 进程，不依赖 OrbStack、Docker、系统扩展或 TUN。
- 应用显示名已统一为 **Hello TSNet**，安装路径为 `/Applications/Hello TSNet.app`。原用户目录副本已移至项目 `dist/hello-tsnet-user-install-<时间>.app` 备份；后续只更新系统应用目录的固定路径。bundle ID、可执行文件名和用户数据目录保持不变。
- tsnet 节点名为 `hello-tsnet`，已完成 Tailscale 官方浏览器授权。
- 当前规则为 `127.0.0.1:30001 → screen-share.example.net:5900`，Screen Sharing 连接地址是 `vnc://127.0.0.1:30001`。
- 管理页面为 `http://127.0.0.1:6060`，规则可在 Web UI 中动态新增、编辑和删除，不需要改代码或重启。
- 旧 Docker 容器 `ts-gateway` 已停止但未删除，只作为短期回滚点。它与原生进程不能同时占用 30001。
- 已增加 macOS 菜单栏应用 `dist/hello-tsnet.app`：粉白点阵图标，以及“关于”“控制面板”“退出应用”三个菜单；控制面板复用默认浏览器中的本地 Web UI。应用由 Launch Services 启动后不依赖终端会话。项目没有登录自启，macOS 重启后需重新打开应用。
- 本机安装位置固定为 `/Applications/Hello TSNet.app`。0.5.0 已更新到该位置，本次使用 `--no-open` 未主动启动。此前 0.3.0 在原用户目录 `~/Applications/hello-tsnet.app` 首次启动约 9 秒后被 SIGKILL；用户明确要求重试后，14:16 从原路径重开约 11 秒后再次被 SIGKILL，6060/30001 均未监听。时间上符合用户安全软件的倒计时拦截，但未直接确认来源；等待用户处理拦截并打开后继续真实设备验收，禁止自动反复启动。
- 0.3.0 已实现设备列表与按设备配置转发：设备名称、系统、地址、在线/离线/未知/授权过期，搜索、已有规则分组；0.5.0 已移除服务预设下拉，改为端口提示。自动测试与模拟数据浏览器验收已通过；新版真实设备数量、状态及转发尚待应用成功启动后验收。
- 0.4.0 已加入 TCP、UDP、TCP + UDP 配置，新建默认两者、旧规则保持 TCP。自动测试、模拟界面验收、构建签名与独立只读 review 通过；真实 tailnet UDP 尚待手工验收。
- 0.5.0 已加入“共享本机服务”：填写名称、同号端口和协议，其他设备可经 tsnet 节点访问本机服务。自动化本机 socket 测试、模拟浏览器验收、独立 review 与构建安装已通过；真实 tailnet 入站访问待手工验收，未主动启动。
- “关于”窗口保留 macOS 标准面板，说明文字水平居中：14 pt 中等字重的主说明、12 pt 次要说明，使用随浅色/深色模式变化的系统文字色，并调整行距与段距。此次样式更新只构建和安装，不主动启动；窗口实际显示待用户解除拦截后验收。

## 为什么仍需本地端口映射

`tsnet` 只在 `hello-tsnet` 进程内部提供 userspace tailnet。macOS 路由表、Finder 和 Screen Sharing 不会获得 Tailscale 的 `100.64.0.0/10` 路由，因此不能直接把 Screen Sharing 指向对端 Tailscale IP。

每条规则分配一个未占用的本机端口：

```text
Screen Sharing → 127.0.0.1:30001 → screen-share.example.net:5900
SSH            → 127.0.0.1:30002 → server-a:22
Web service    → 127.0.0.1:30003 → 100.x.y.z:8080
```

新增规则只需通过 Web UI；Manager 会动态创建 listener。无需预分配端口，也无需修改程序。若要让所有 macOS 应用直接访问 Tailscale IP，必须引入系统级 TUN/Network Extension 和路由，这不属于当前“轻量、用户空间、隔离运行”的目标。

## 架构与数据流

转发路径：

```text
本机应用
  → 127.0.0.1:<localPort>
  → internal/forward.Manager
  → internal/tailnet.Runtime.Dial
  → tsnet userspace tailnet
  → <target host>:<target port>
```

规则写入路径：

```text
Web API
  → internal/rules.Service（串行化）
  → Manager.Prepare（先绑定所有新增端口，不接流量）
  → Store.Save（0600 临时文件 + rename）
  → ChangeSet.Commit（一次发布完整规则快照并开始 accept）
```

这个顺序是核心不变量：所有可能失败的端口绑定都发生在写盘之前；写盘成功后的 Commit 不返回错误。禁止改回“部分生效后失败再双回滚”。

启动顺序：

```text
预留 Web listener
  → Store.Load
  → Service.Restore
  → 启动 HTTP Serve
  → Runtime.Start
  → 后台等待 tailnet ready
```

规则文件损坏或恢复失败时，进程保留原文件并退出，不会按空配置覆盖。tsnet Start 失败时，Web 保持可用并显示 error。恢复 inbound listener 会通过 tsnet.Listen 隐式 Start（不等待授权）；后续 Runtime.Start 保留已捕获的授权 URL。恢复失败会有界关闭 Manager 和 Runtime，释放隐式启动资源。

关闭顺序：

```text
停止 Web
  → 关闭 listener
  → 最多等待 5 秒转发连接 drain
  → 强制关闭剩余连接
  → 关闭 tsnet
```

## 代码职责

- `cmd/hello-tsnet`：参数解析、信号转交和退出码。
- `internal/app`：唯一装配入口、启动顺序和有界关闭。
- `internal/config`：规则模型、校验、版本化 JSON 和原子持久化。
- `internal/rules`：唯一写入口和 Prepare → Save → Commit 编排。
- `internal/forward`：loopback/tsnet listener、规则快照、30 秒连接级拨号超时、双向 TCP 半关闭与有界 UDP 会话。
- `internal/tailnet`：tsnet 生命周期、官方授权 URL 捕获、pending/ready/error 状态和 Dial 适配。
- `internal/web`：loopback HTTP API、Host/Origin 校验、body 上限和嵌入式静态页面。
- `web`：无框架、无构建步骤的 HTML/CSS/JavaScript，通过 `go:embed` 进入二进制。
- `cmd/hello-tsnet-app`：macOS 应用入口，锁定主 OS 线程、解析参数并转交信号。
- `internal/desktop`：原生外壳与网关的生命周期协调，退出等待网关完成关闭。
- `internal/menubar`：cgo + Objective-C AppKit 菜单、关于窗口、默认浏览器打开和主线程事件循环。
- `macos`、`scripts/build-app.sh`：应用图标、Info.plist 和 `.app` 构建；详情见 `macos/README.md`。

## API 契约

默认 authority：`127.0.0.1:6060`。服务也接受同端口的 `localhost` Host；其他 Host 返回 403。写请求的 Origin 必须是同一监听端口的 `http://127.0.0.1:<port>` 或 `http://localhost:<port>` 之一，不启用 CORS。当前实现分别校验 Host 和 Origin 白名单，不要求两者使用相同的主机名别名。

### 状态

```http
GET /api/status
```

返回 `pending`、`ready` 或 `error`。首次授权 URL 只在 pending 时存在于内存响应中；响应始终设置 `Cache-Control: no-store`；ready 后清除 URL。

### 规则

```http
GET    /api/rules
POST   /api/rules
PUT    /api/rules/{id}
DELETE /api/rules/{id}
```

POST/PUT JSON：

```json
{
  "name": "Screen Sharing",
  "localPort": 30001,
  "target": "screen-share.example.net:5900",
  "protocol": "tcp",
  "direction": "outbound"
}
```

- POST/PUT 需要 `Content-Type: application/json`，body 上限 64 KiB。
- DELETE 不需要 body 或 Content-Type，但仍必须通过 Host 和 Origin 校验。
- outbound 本机端口范围为 1024–65535，inbound 为 1–65535；数字端口跨方向唯一，规则 ID 也必须唯一。
- outbound target 必须是 `host:port`；支持 MagicDNS 名称、Tailscale IP 和带方括号的 IPv6。inbound 不传 target，服务端固定派生为 `127.0.0.1:同端口`。
- 同一个本机端口只能映射一个目标；目标变更会复用 listener，不中断无关规则。

### 转发协议

规则 `protocol` 支持 `tcp`、`udp`、`both`。POST 省略时默认 both；PUT 省略时保留旧规则协议。旧内存规则缺字段按 TCP 处理；界面新建默认 TCP + UDP，编辑旧规则保持 TCP。数字本机端口仍跨规则唯一。

协调器按 `(方向, 协议, 端口)` 准备所有 TCP/UDP socket，仅在 Save 成功后一次发布目标快照。UDP socket 绑定后先不读取，Commit 后才启动；失败/Abort 释放全部暂存 socket。协议删减、规则删除、目标变更会取消对应旧 UDP 会话，目标切换不重绑仍使用的 socket。退出时取消 UDP 拨号与会话；关停后的 Commit 只关闭暂存 socket，不启动新 listener。

UDP 使用 tsnet `Dial(ctx, "udp", target)`，每个本机源地址/端口一个连接，保留报文边界及零长度报文。空闲 60 秒回收，每 listener 最多 128 个会话，每会话队列 8 个报文，超限丢弃；目标连接写入设置 5 秒超时，拨号最长 30 秒。返回本机的共享 UDP socket 共用写入期限，可能被其他客户端刷新，不作为每次写入独立的严格 5 秒上限。一端暂不可达只影响对应会话。单报文实际大小还受操作系统和路径 MTU 限制，不保证交付。

### 共享本机服务（0.5.0）

界面保留“我的设备”的 outbound 规则，另设“共享本机服务”。共享表单仅名称、本机端口、协议，默认 both，端口不预填；服务预设下拉已移除，端口框下显示灰色常用端口提示。

POST 示例：`{"name":"开发服务","localPort":8000,"protocol":"both","direction":"inbound"}`。流量路径是 `其他设备 → hello-tsnet:8000 → 127.0.0.1:8000`；对端需有访问授权，本机服务必须监听 IPv4 loopback 或包含它的地址。`GET /api/status` 的 hostname 返回配置的节点名，用于显示访问地址；不等同于验证过的 MagicDNS 名称（若后台重命名或名称冲突，应以 Tailscale 实际节点地址为准）。

使用固定依赖 tsnet.Server.Listen("tcp"/"udp", ":端口") 在节点上监听。tsnet 的 UDP listener.Accept 返回每流的保留报文边界 net.Conn；不可替换成需要已分配节点 IP 的 ListenPacket wildcard。Prepare 仅注册，不 Accept；Save 后 Commit 才接流量。共享目标使用系统 net.Dialer 拨号 loopback，访问远端仍使用 Runtime.Dial。

每个共享协议 listener 最多 128 个连接/会话，超限关闭新连接；UDP 双向活动延长 60 秒空闲期限，每次写入最长 5 秒，保留空报文。删除/删减协议会取消该 listener 的全部共享连接。管理端口在 app 装配层拒绝共享（默认 6060，亦覆盖自定义管理端口），保持管理 API 仅本机访问。管理监听端口必须为固定的 1–65535，拒绝端口 0，避免实际分配端口与共享保护值不一致。

### 可见设备

`GET /api/devices` 使用 tsnet `LocalClient().Status()` 返回公开显示字段的数组：`id`、`name`、`dnsName`、`addresses`、`os`、`state` 和可选 `lastSeen`。不返回原始状态对象、授权 URL、节点公钥或对端公网连接地址。只保留最新 netmap 中的 peer，不包含自身。

请求有 5 秒超时，响应 `Cache-Control: no-store`，受现有 Host 白名单保护。未就绪、读取失败、后端非 Running 或本节点未连接控制面时返回 503，而不是空数组。`Online` 为假且没有最后在线证据时显示 unknown；已过期节点显示 expired。前端每 10 秒刷新，失败保留上次设备列表并统一显示未知；真实空列表只有成功返回 `[]` 时成立。

映射表保留名称列最小宽度和单行操作按钮，长地址保持完整；超宽时仅表格内横向滚动。

从设备卡片新增映射时自动绑定当前设备，目标地址输入框隐藏并禁用，仅需填写名称、本机端口、目标端口和协议；编辑已有规则时保留地址修改能力。

已有规则按完整 DNS、无歧义短 DNS、IPv4/IPv6 归属；多台设备同名时不猜测，保留在设备区的“未关联设备的转发”。保存仍复用原有规则 API 与协调器，不改变规则文件的原子保存流程。规则请求用序号丢弃过期响应，避免旧刷新覆盖刚保存的列表。

## 持久化和敏感数据

应用目录：

```text
~/Library/Application Support/hello-tsnet/
├── rules.json        # 0600
├── instance.lock     # 0600，新版 CLI/GUI 共用的单实例文件锁
└── tsnet/            # 0700，包含节点身份和 Tailscale 状态
```

`rules.json` 当前写入版本为 3；读取 version 1 时只在内存补全 TCP，version 1/2 补全 outbound，不主动重写文件。version 2 要求每条规则显式指定协议；version 3 同时要求显式方向和协议。旧二进制不能读取新版本；回滚时需恢复对应旧规则备份，不能只修改版本号：

```json
{
  "version": 3,
  "rules": [
    {
      "id": "随机 128-bit 十六进制 ID",
      "name": "Screen Sharing",
      "localPort": 30001,
      "target": "screen-share.example.net:5900",
      "protocol": "tcp",
      "direction": "outbound"
    }
  ]
}
```

仓库不得包含上述目录、真实授权 URL、auth key、cookie、节点私钥或编译产物。`.gitignore` 已忽略 `dist/` 和 `.data/`。Tailscale 账号密码从未进入本项目；登录由官方页面完成。

## 安全边界

- Web UI 和每条映射固定监听 `127.0.0.1`；命令行拒绝 `0.0.0.0` 和 LAN 地址。
- API 使用精确 Host 白名单和 loopback Origin 白名单阻止 DNS rebinding/跨站写入；不启用 CORS。Host 与 Origin 可分别使用 `127.0.0.1` 或 `localhost`，只要端口与监听端口一致。
- 授权 URL 视作短期凭据：只存内存、只经 loopback API 返回、no-store、ready 后清除。
- tailnet 目标是否可访问仍由 Tailscale ACL/grants 决定；本项目不绕过 ACL。
- 每条连接的 tailnet Dial 最长 30 秒，避免未授权或目标不可达时无限占用 goroutine/FD。
- 当前没有 Web 登录口令。安全性依赖 loopback 绑定和同源校验；若未来允许 LAN/远程管理，必须先重新设计认证和 TLS。

## 运行和诊断

构建：

```bash
env -u GOROOT GOPROXY=https://proxy.golang.org,direct GOSUMDB=sum.golang.org \
  go build -trimpath -ldflags="-s -w" -o dist/hello-tsnet ./cmd/hello-tsnet
```

本机 shell 曾存在 `GOROOT=/` 和空 `GOPROXY`；上面的覆盖只作用于当前命令。不要为了构建本项目直接改用户全局 Go 配置。

启动：

```bash
./dist/hello-tsnet
```

菜单栏版本使用 `./scripts/build-app.sh` 构建，运行 `python3 scripts/install-app.py` 更新并从固定路径 `/Applications/Hello TSNet.app` 启动一次。可用 `--no-open` 只更新。不要运行 `dist/` 或随机暂存路径中的应用，也不要按 bundle ID 查找副本，避免触发用户安全软件反复确认。固定路径不能保证新内容哈希免于重新审批；被拦截时等待用户处理，不循环重试。

两个入口使用同一数据目录，不能同时运行；新版入口通过文件锁拦截重复启动。退出菜单、SIGTERM 和系统正常退出请求都会关闭网关，锁文件保留但锁自动释放。安装脚本保留 `dist/hello-tsnet-installed.previous.tar`，失败恢复旧文件；不会自动启动回滚副本。现有 CLI 入口仍可使用，但本机验收默认只运行固定安装的 `.app`。

检查：

```bash
curl -s http://127.0.0.1:6060/api/status
curl -s http://127.0.0.1:6060/api/rules
lsof -nP -iTCP:6060 -sTCP:LISTEN
lsof -nP -iTCP:30001 -sTCP:LISTEN
pgrep -fl '/dist/hello-tsnet'
docker ps -a --filter name='^/ts-gateway$'
```

不要把 PID 写入文档或脚本；PID 每次启动都会改变。

## 验证证据

完整验收在 `docs/acceptance.md`。关键结果：

- `go test -race ./...`、`go vet ./...`、release build、JavaScript 语法检查和测量脚本语法检查均通过。
- 真实 VNC 握手两次返回 `RFB 003.889`；重启后身份和规则恢复。
- Screen Sharing 打开 “Example Mac” 连接窗口。
- 登录稳定、一个 listener、零活跃转发连接时：RSS 37,008 KiB，60 秒 CPU 0.000%，二进制 19,815,730 bytes，状态目录 48 KiB。
- 独立 review 报告的 DELETE 415、无限拨号等待、README Go 版本和关闭按钮问题均已修复并定向复核。

验证命令：

```bash
env -u GOROOT GOPROXY=https://proxy.golang.org,direct GOSUMDB=sum.golang.org \
  go test -race ./...
env -u GOROOT GOPROXY=https://proxy.golang.org,direct GOSUMDB=sum.golang.org \
  go vet ./...
node --check web/app.js
bash -n scripts/measure.sh
env -u GOROOT GOPROXY=https://proxy.golang.org,direct GOSUMDB=sum.golang.org \
  go build -trimpath -ldflags="-s -w" -o dist/hello-tsnet ./cmd/hello-tsnet
```

真实 tailnet、Screen Sharing 和资源结果必须单独验收；自动测试不能替代。

## 回滚与再次切换

回滚到 Docker：

1. 先优雅停止原生 `hello-tsnet`，确认 30001 已释放。
2. 运行 `docker start ts-gateway`。
3. 确认容器为 Up，且 `127.0.0.1:30001` 由 OrbStack 监听。
4. 用 Screen Sharing 验证 `vnc://127.0.0.1:30001`。

切回原生：

1. 运行 `docker stop ts-gateway`，等待 OrbStack 释放 30001。
2. 启动 `./dist/hello-tsnet`。
3. 确认 `/api/status` 为 ready、规则存在、30001 由 `hello-tsnet` 监听。
4. 验证 VNC 握手或 Screen Sharing。

任何方向都必须先停旧 listener 再启新 listener。当前未删除 Docker 容器和 volume；删除属于后续显式清理动作。

## 已完成工作

- 评估 OrbStack 容器基线，确认虚拟机底座约 1 GiB 宿主 RSS，而 Tailscale 容器本身约 53 MiB。
- 移除 GOST 运行组件，仅保留旧 Tailscale Docker 网关作为过渡。
- 创建原生 Go 项目、AGENTS、README、实现计划、验收和资源测量脚本。
- 使用官方 `tailscale.com/tsnet` v1.102.4 实现无 TUN、无 root 的用户空间 tailnet。
- 实现动态 TCP CRUD、原子持久化、重启恢复、首次授权 UI、本地安全校验和有界退出。
- 完成真实 Tailscale 授权、30001 切换、VNC/Screen Sharing 验收和重启恢复。
- 停止旧 `ts-gateway`，保留回滚点。
- 创建并推送私有 GitHub 仓库；默认分支为 `main`。

## 明确限制

- 支持 TCP/UDP；不支持 UDP 广播、多播或自动服务发现。
- macOS 不能直接访问 tailnet `100.x` 地址；非 tsnet 应用必须使用本地映射。
- 没有 SOCKS5/HTTP 代理 UI；支持代理的应用若需要可作为独立后续能力评估。
- 已有固定路径安装脚本；没有 LaunchAgent、图形安装器、自动升级、Developer ID 签名或 notarization。菜单栏应用已有本机 ad-hoc 签名，当前构建目标为本机架构、macOS 15+，未验证 Intel 或更早系统。
- 没有结构化日志、指标、连通性诊断或规则导入导出。
- 没有 LAN/远程 Web 管理和多用户认证。
- 本地端口冲突会拒绝整次变更，保留此前工作规则；不会自动挑选其他端口。
- 现有自动测试没有单独覆盖 Add/Delete 混合并发以及真实活跃连接超过 5 秒后的强制关闭；0.5.0 已补 Service.Restore 装配失败后的有界 Manager/Runtime 清理回归。历史 MVP 计划中的未完成复选框不代表后续新增测试缺失。

## 后续研发优先级

1. **登录自启与升级**：已有固定安装位置和手动更新脚本，后续设计可逆的登录自启和升级流程；不依赖项目 `dist/` 运行。
2. **发布流程**：增加 GitHub Actions 的 race/test/vet/build，生成 arm64 release、SHA-256 和版本号；再评估签名/notarization。
3. **可诊断性**：在不泄漏目标凭据的前提下展示规则 listener 状态、最近拨号错误和手动连通性测试。
4. **配置运维**：评估导入/导出和备份规则；保持 tsnet 身份与普通规则分离。
5. **可选代理**：仅为明确支持 SOCKS5 的应用评估 tsnet Loopback；不要用它替代 Screen Sharing 的固定 TCP 映射。

每个后续任务完成时更新本文的“当前结论”“明确限制”和“后续研发优先级”，并把不可重复的真实验收证据追加到 `docs/acceptance.md`。
