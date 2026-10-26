# macOS 菜单栏应用

`./scripts/build-app.sh` 在 macOS 上构建 `dist/hello-tsnet.app`，目标为构建机器的架构，最低系统版本 macOS 15。需要 Go 1.27.1、Xcode Command Line Tools 和 cgo；不增加第三方 Go 依赖。

应用通过 AppKit `NSStatusItem` 显示菜单，`LSUIElement` 隐藏 Dock 图标。Go 网关与 AppKit 在同一进程：主 OS 线程运行 AppKit，goroutine 运行现有 `app.App`。

- 关于：系统原生关于窗口，显示应用名称、图标和版本。
- 控制面板：用默认浏览器打开当前 loopback 管理地址。
- 退出应用：取消网关 context，等待原有关闭流程完成，再退出 AppKit。

SIGINT、SIGTERM 和 macOS 正常终止请求也走关闭流程。数据目录中的 `instance.lock` 使用系统文件锁，阻止多个新版 CLI/GUI 实例同时操作同一个 tsnet 身份；锁随进程退出释放，锁文件留在原地。

打包先在临时目录完成构建、图标转换、plist 检查和 ad-hoc 签名验证，再移动到目标位置。已有产物保留为 `dist/hello-tsnet-<时间>-<进程号>.app`。ad-hoc 签名用于本机运行，不等同于 Developer ID 签名或 Apple 公证。

## 固定路径更新

暂存目录固定为 `dist/.app-build`，仅用于构建，从不执行其中的应用。构建完成后运行 `python3 scripts/install-app.py`，安装目标固定为 `/Applications/Hello TSNet.app`。脚本只停止这个精确路径的进程，最多等候 12 秒，不强制终止；原位替换 Contents 前在 `dist/hello-tsnet-installed.previous.tar` 备份旧应用，验证失败时恢复包含执行权限的旧文件。

默认只通过 `open -a` 对固定绝对路径发起一次启动；`--no-open` 只更新。禁止按 bundle ID 查找并启动副本，禁止从构建、暂存、测试、备份目录启动真正的网关。安全软件拦截后不要自动重复启动，需要用户处理提示。固定路径和签名 identifier 能减少路径变化，但 ad-hoc 签名的内容哈希随版本变化，不能保证安全软件免于再次确认。

## 图标

`AppIcon.png` 是内置 imagegen 生成的项目资源，提示词为：粉色圆角方形底、白色 Tailscale 风格 3×3 点阵、Hello Kitty 粉白配色、透明外边缘、无文字和装饰。构建时用系统 `sips` 和 `iconutil` 转换为多尺寸 `.icns`。

菜单栏单独使用 SF Symbols 的 `circle.grid.3x3.fill`，设置 `NSImage.template = YES`，让 AppKit 按菜单栏明暗背景和选中状态渲染黑白图标，不再从粉色应用图标缩放生成菜单栏 PNG。关于窗口使用系统文字色；控制面板通过 `prefers-color-scheme` 自动适配明暗模式。

这是独立项目图标，不表示 Tailscale 或 Sanrio 官方应用。

## 验证

```bash
env -u GOROOT go test -race ./...
env -u GOROOT go vet ./...
./scripts/test-menubar.sh
./scripts/build-app.sh
```

原生菜单测试需要已登录的 macOS 图形会话。真实 tailnet 与菜单鼠标点击属于手工验收，详见 `docs/acceptance.md`。

参考：[Apple NSStatusItem](https://developer.apple.com/documentation/appkit/nsstatusitem)、[Tailscale 品牌入口](https://tailscale.com/press)。
