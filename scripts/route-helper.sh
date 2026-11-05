#!/bin/bash
set -euo pipefail
exec /bin/bash "$(/usr/bin/dirname "$0")/../internal/routehelper/route-helper.sh" "$@"
