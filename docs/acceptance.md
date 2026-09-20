# hello-tsnet 验收记录

日期：2026-09-20

## 自动验证

- `go test -race ./...`：通过。
- `go vet ./...`：通过。
- release 构建：通过，arm64 Mach-O。
- release 二进制：19,815,730 bytes，低于 60 MiB 预算。

验证命令使用当前 Homebrew Go 根目录覆盖了 shell 中错误的 `GOROOT=/`；没有修改全局 Go 配置。

## 本机运行检查

- 管理界面监听：`127.0.0.1:6060`，未监听 LAN 地址。
- 首次授权前状态为 `pending`；授权后为 `ready`，状态响应不再包含授权 URL。
- 授权 URL 只通过 loopback `/api/status` 返回，响应包含 `Cache-Control: no-store`。
- 旧 OrbStack 仍占用 `127.0.0.1:30001` 时，新增规则得到冲突响应，运行状态和规则文件均未部分更新。
- 旧 `ts-gateway` 停止后，成功创建 `30001 → screen-share.example.net:5900`。
- VNC 真实握手返回 `RFB 003.889`，Screen Sharing 成功打开 “Example Mac” 连接窗口。
- 进程重启后，tsnet 身份、`ready` 状态、规则和 30001 listener 均自动恢复，VNC 握手再次成功。

## 登录稳定后的资源结果

测量环境：macOS 15.7.9、arm64；一条 listener、零活跃转发连接；预热 120 秒，再采样 60 秒。

- 空闲 RSS：37,008 KiB（约 36.1 MiB），通过。
- 60 秒平均 CPU：单核 0.000%，通过。
- release 二进制：19,815,730 bytes（约 18.9 MiB），通过。
- 状态目录：48 KiB，通过。

## 资源预算

- 空闲 RSS：小于 200 MiB。
- 60 秒平均 CPU：小于单个逻辑核心的 0.5%。
- release 二进制：小于 60 MiB。
- 状态目录：小于 10 MiB。

结论：四项资源预算全部通过。旧 `ts-gateway` 容器已停止但未删除，保留为可恢复回滚点。
