#!/bin/bash
set -euo pipefail

label=org.hellotsnet.app.routes
binary="/Library/PrivilegedHelperTools/$label"
plist="/Library/LaunchDaemons/$label.plist"
owner="${SUDO_UID:-}"
expected_digest=""
if [[ ${1:-} == install-from-app && $# == 4 ]]; then
    owner="$2"
    expected_digest="$4"
    if [[ ! "$expected_digest" =~ ^[0-9a-f]{64}$ ]]; then
        exit 1
    fi
    set -- install "$3"
elif [[ ${1:-} == uninstall-from-app && $# == 2 ]]; then
    owner="$2"
    set -- uninstall
fi
if [[ $EUID != 0 || ! "$owner" =~ ^[1-9][0-9]*$ ]]; then
    echo "请从日常用户的终端通过 sudo 执行。"
    exit 1
fi

remove_job() {
    if /bin/launchctl print "system/$label" >/dev/null 2>&1; then
        /bin/launchctl bootout "system/$label"
    fi
}

case "${1:-}" in
    uninstall)
        remove_job
        if /usr/bin/grep -Eq '^# (BEGIN|END) Hello TSNet[[:space:]]*$' /etc/hosts; then
            "$binary" --clear-hosts
        fi
        /bin/rm -f "$plist" "$binary"
        socket="/var/run/org.hellotsnet.app.$owner.sock"
        if [[ -S "$socket" ]]; then
            /bin/rm -f "$socket"
        fi
        echo "路由助手已移除。"
        ;;
    install)
        app="${2:-/Applications/Hello TSNet.app}"
        helper="$app/Contents/Helpers/hello-tsnet-route-helper"
        /usr/bin/codesign --verify --strict "$app"
        umask 077
        /bin/mkdir -p /Library/PrivilegedHelperTools
        stage=$(/usr/bin/mktemp -d /Library/PrivilegedHelperTools/.hello-tsnet.XXXXXX)
        trap '/bin/rm -rf "$stage"' EXIT
        /usr/bin/install -o root -g wheel -m 0755 "$helper" "$stage/helper"
        /usr/bin/codesign --verify --strict "$stage/helper"
        digest=$(/usr/bin/shasum -a 256 "$stage/helper")
        digest="${digest%% *}"
        if [[ -n "$expected_digest" && "$digest" != "$expected_digest" ]]; then
            echo "路由助手与当前应用包不匹配。"
            exit 1
        fi
        /bin/cat > "$stage/daemon.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>$label</string>
    <key>ProgramArguments</key>
    <array><string>$binary</string><string>$owner</string></array>
    <key>EnvironmentVariables</key>
    <dict><key>HELLO_TSNET_HELPER_SHA256</key><string>$digest</string></dict>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key>
    <dict><key>SuccessfulExit</key><false/></dict>
    <key>Sockets</key>
    <dict>
        <key>RouteControl</key>
        <dict>
            <key>SockPathName</key><string>/var/run/org.hellotsnet.app.$owner.sock</string>
            <key>SockPathOwner</key><integer>$owner</integer>
            <key>SockPathMode</key><integer>384</integer>
        </dict>
    </dict>
</dict>
</plist>
PLIST
        /usr/bin/plutil -lint "$stage/daemon.plist"
        /bin/chmod 0644 "$stage/daemon.plist"
        if [[ -f "$binary" && -f "$plist" ]]; then
            /bin/cp -p "$binary" "$stage/old-helper"
            /bin/cp -p "$plist" "$stage/old.plist"
        fi
        remove_job
        /bin/mv -f "$stage/helper" "$binary"
        /bin/mv -f "$stage/daemon.plist" "$plist"
        if ! /bin/launchctl bootstrap system "$plist"; then
            /bin/rm -f "$plist" "$binary"
            if [[ -f "$stage/old.plist" ]]; then
                /bin/mv "$stage/old-helper" "$binary"
                /bin/mv "$stage/old.plist" "$plist"
                /bin/launchctl bootstrap system "$plist"
            fi
            exit 1
        fi
        echo "路由助手已安装。Hello TSNet 会在连接 Tailnet 后自动同步设备地址。"
        ;;
    *)
        echo "用法：sudo /bin/bash $0 install [Hello TSNet.app 路径] 或 uninstall"
        exit 2
        ;;
esac
