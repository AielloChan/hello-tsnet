#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/.."
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
clang -fobjc-arc -fblocks -framework AppKit -framework ServiceManagement -framework ImageIO -framework CoreGraphics \
    scripts/test-menubar.m internal/menubar/panel_darwin.m internal/menubar/preferences_darwin.m \
    internal/menubar/avatar_darwin.m -o "$scratch/test-menubar"
"$scratch/test-menubar"
