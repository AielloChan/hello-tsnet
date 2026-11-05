<div align="center">
  <img src="macos/AppIcon.svg" width="128" height="128" alt="Hello TSNet logo">
  <h1>Hello TSNet</h1>
  <p>A native macOS menu bar app for accessing devices in your Tailnet.</p>
  <p>
    English · <a href="docs/README.zh-CN.md">简体中文</a> ·
    <a href="docs/README.zh-TW.md">繁體中文</a> ·
    <a href="docs/README.ja.md">日本語</a> ·
    <a href="docs/README.es.md">Español</a>
  </p>
</div>

Hello TSNet embeds [Tailscale tsnet](https://tailscale.com/docs/features/tsnet) to join your Tailnet as a node. Its native control panel shows your local addresses, visible devices, and connection status.

- **Reach Tailnet devices.** An optional route helper keeps IPv4 and IPv6 host routes in sync. Use a device’s Tailnet IP, full `.ts.net` name, or `.local` alias with the original service port.
- **Expose a local service to your Tailnet.** Connections allowed by your [Tailnet access policy](https://tailscale.com/docs/features/access-control/grants) can reach TCP and UDP services on the same port at `127.0.0.1`.
- **Manage the connection.** Search devices, copy addresses and names, sign in or out, and manage the helper from the native panel.

## Build and install

You need macOS 15 or newer, Go 1.27.1 or newer, Python 3, and Xcode Command Line Tools. Clone this repository and run these commands from its root.

```bash
./scripts/build-app.sh
python3 scripts/install-app.py
```

The installer places the app at `/Applications/Hello TSNet.app` and opens it. In the menu bar, choose **授权登录 (Sign in)** and complete the Tailscale authorization in your browser.

Choose **安装路由助手… (Install Route Helper)** and approve the macOS administrator prompt to enable direct access by IP and name. You can update or remove the helper later under **控制面板 → 设置 (Control Panel → Settings)**. If you installed a helper from a pre-public build, uninstall it from that build before installing the public version.

## How routing and names work

The app runs as your user. The helper uses a launchd Unix socket to set up a virtual interface and host routes for Tailnet devices. It writes name mappings in a marked block in `/etc/hosts`, gives existing entries precedence, and omits ambiguous `.local` aliases. The helper releases routes and removes its block when you quit or sign out.

## Development

```bash
go test -race ./...
go vet ./...
go build ./cmd/hello-tsnet
./scripts/test-menubar.sh
```

See the [maintainer notes](docs/HANDOFF.md) for the architecture and manual network checks.

## License

[MIT](LICENSE). Hello TSNet is an independent project that uses Tailscale’s tsnet library.
