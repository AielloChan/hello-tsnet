#!/bin/bash
set -euo pipefail

if [[ $# != 3 || ! -d "$1" || "$1" != *.app || "$2" != *.dmg ]]; then
    echo "Usage: $0 <app bundle> <output.dmg> <arm64|x86_64>" >&2
    exit 1
fi

app="$1"
output="$2"
architecture="$3"
case "$architecture" in
    arm64|x86_64) ;;
    *) echo "Unsupported macOS architecture: $architecture" >&2; exit 1 ;;
esac

verify_app() {
    local bundle="$1" executable actual
    codesign --verify --deep --strict "$bundle"
    test -f "$bundle/Contents/Resources/Licenses/Hello-TSNet.txt"
    for executable in Contents/MacOS/hello-tsnet Contents/Helpers/hello-tsnet-route-helper; do
        actual=$(lipo -archs "$bundle/$executable")
        if [[ "$actual" != "$architecture" ]]; then
            echo "$executable has architecture $actual, expected $architecture" >&2
            return 1
        fi
    done
}

verify_app "$app"
output_dir=$(dirname "$output")
mkdir -p "$output_dir"
output_dir=$(cd "$output_dir" && pwd -P)
work=$(mktemp -d "$output_dir/.hello-tsnet-dmg.XXXXXX")
mount="$work/mount"
attached=false
cleanup() {
    if [[ "$attached" == true ]] && ! hdiutil detach "$mount" -force -quiet; then
        echo "Failed to detach the verification volume at $mount" >&2
        return 1
    fi
    rm -rf "$work"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

mkdir "$work/staging" "$mount"
ditto "$app" "$work/staging/Hello TSNet.app"
ln -s /Applications "$work/staging/Applications"
hdiutil create -quiet -volname "Hello TSNet" -srcfolder "$work/staging" -fs HFS+ -format UDZO "$work/verified.dmg"

hdiutil attach -quiet -readonly -nobrowse -mountpoint "$mount" "$work/verified.dmg"
attached=true
test "$(readlink "$mount/Applications")" = /Applications
verify_app "$mount/Hello TSNet.app"
hdiutil detach "$mount" -quiet
attached=false

mv -f "$work/verified.dmg" "$output"
echo "Created $output"
