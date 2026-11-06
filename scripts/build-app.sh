#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")/.."
if [[ "$(uname -s)" != Darwin ]]; then
    echo "构建 macOS 应用需要 macOS 和 Xcode Command Line Tools。" >&2
    exit 1
fi

mkdir -p dist
staging="$PWD/dist/.app-build"
if ! mkdir "$staging"; then
    echo "构建目录已存在；请确认没有其他构建任务后清理 dist/.app-build 再试。" >&2
    exit 1
fi
trap 'rm -rf "$staging"' EXIT
bundle="$staging/hello-tsnet.app"
resources="$bundle/Contents/Resources"
mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Helpers" "$resources" "$staging/AppIcon.iconset"
cp macos/Info.plist "$bundle/Contents/Info.plist"
cp internal/routehelper/route-helper.sh "$resources/route-helper.sh"
env -u GOROOT CGO_ENABLED=1 MACOSX_DEPLOYMENT_TARGET=15.0 \
    GOPROXY=https://proxy.golang.org,direct GOSUMDB=sum.golang.org \
    go build -trimpath -ldflags="-s -w" -o "$bundle/Contents/Helpers/hello-tsnet-route-helper" ./cmd/hello-tsnet-route-helper
codesign --force --sign - --identifier org.hellotsnet.app.routes "$bundle/Contents/Helpers/hello-tsnet-route-helper"
helper_digest=$(/usr/bin/shasum -a 256 "$bundle/Contents/Helpers/hello-tsnet-route-helper")
env -u GOROOT CGO_ENABLED=1 MACOSX_DEPLOYMENT_TARGET=15.0 \
    GOPROXY=https://proxy.golang.org,direct GOSUMDB=sum.golang.org \
    go build -trimpath -ldflags="-s -w -X hello-tsnet/internal/routehelper.bundledHelperSHA256=${helper_digest%% *}" \
    -o "$bundle/Contents/MacOS/hello-tsnet" ./cmd/hello-tsnet-app

licenses="$resources/Licenses"
mkdir -p "$licenses/Go"
cp LICENSE "$licenses/Hello-TSNet.txt"
go_root=$(env -u GOROOT go env GOROOT)
go_license="$go_root/LICENSE"
if [[ ! -f "$go_license" ]]; then
    # Homebrew stores the license one level above libexec.
    go_license="$(dirname "$go_root")/LICENSE"
fi
cp "$go_license" "$licenses/Go/LICENSE"
if [[ -f "$go_root/PATENTS" ]]; then
    cp "$go_root/PATENTS" "$licenses/Go/"
fi
modules=$(env -u GOROOT CGO_ENABLED=1 go list -deps \
    -f '{{with .Module}}{{if not .Main}}{{.Path}}{{"\t"}}{{.Dir}}{{end}}{{end}}' \
    ./cmd/hello-tsnet-app ./cmd/hello-tsnet-route-helper | sort -u)
while IFS=$'\t' read -r module_path module_dir; do
    [[ -n "$module_path" ]] || continue
    destination="$licenses/$module_path"
    mkdir -p "$destination"
    for document in LICENSE LICENSE.txt LICENSE.md COPYING NOTICE NOTICE.txt NOTICE.md PATENTS; do
        if [[ -f "$module_dir/$document" ]]; then
            cp "$module_dir/$document" "$destination/"
        fi
    done
done <<< "$modules"

for size in 16 32 128 256 512; do
    sips -z "$size" "$size" macos/AppIcon.png --out "$staging/AppIcon.iconset/icon_${size}x${size}.png" >/dev/null
    double=$((size * 2))
    sips -z "$double" "$double" macos/AppIcon.png --out "$staging/AppIcon.iconset/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$staging/AppIcon.iconset" -o "$resources/AppIcon.icns"
plutil -lint "$bundle/Contents/Info.plist"
codesign --force --sign - --identifier org.hellotsnet.app "$bundle"
codesign --verify --deep --strict "$bundle"

destination="$PWD/dist/hello-tsnet.app"
if [[ -e "$destination" ]]; then
    # Renaming preserves a running executable and provides a local rollback copy.
    backup="$PWD/dist/hello-tsnet-$(date +%Y%m%d-%H%M%S)-$$.app"
    mv "$destination" "$backup"
fi
mv "$bundle" "$destination"
echo "已构建：$destination"
