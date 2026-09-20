#!/bin/bash
set -euo pipefail

if [[ $# -lt 1 || $# -gt 3 ]]; then
  echo "usage: $0 <pid> [binary] [data-dir]" >&2
  exit 2
fi

process_id="$1"
binary_path="${2:-dist/hello-tsnet}"
data_dir="${3:-${HOME}/Library/Application Support/hello-tsnet}"

cpu_seconds() {
  ps -o time= -p "${process_id}" | awk -F '[-:]' '
    NF == 4 { print ($1 * 86400) + ($2 * 3600) + ($3 * 60) + $4; next }
    NF == 3 { print ($1 * 3600) + ($2 * 60) + $3; next }
    NF == 2 { print ($1 * 60) + $2; next }
  '
}

if ! kill -0 "${process_id}" 2>/dev/null; then
  echo "process ${process_id} is not running" >&2
  exit 1
fi

echo "macOS: $(sw_vers -productVersion)"
echo "architecture: $(uname -m)"
echo "condition: authenticated, one listener, zero active forwarded connections"
echo "warmup: 120 seconds"
sleep 120

start_cpu="$(cpu_seconds)"
start_time="$(date +%s)"
sleep 60
end_cpu="$(cpu_seconds)"
end_time="$(date +%s)"

awk -v start="${start_cpu}" -v end="${end_cpu}" -v elapsed="$((end_time - start_time))" \
  'BEGIN { printf "cpu_single_core_percent_60s: %.3f%%\n", ((end - start) / elapsed) * 100 }'
echo "rss_kib: $(ps -o rss= -p "${process_id}" | tr -d ' ')"
echo "binary_bytes: $(stat -f '%z' "${binary_path}")"
echo "state_kib: $(du -sk "${data_dir}" | awk '{print $1}')"
