#!/usr/bin/env bash
set -euo pipefail

threshold="${1:-85}"
report="$(mktemp)"
trap 'rm -f "$report"' EXIT

go test ./internal/... -cover | tee "$report"
awk -v threshold="$threshold" '
    /coverage: [0-9.]+% of statements/ {
        package=$2
        coverage=$5
        gsub(/%/, "", coverage)
        if ((coverage + 0) < threshold) {
            printf "核心包覆盖率不足：%s 为 %s%%，要求 %s%%\n", package, coverage, threshold > "/dev/stderr"
            failed=1
        }
    }
    END { exit failed }
' "$report"
echo "核心包覆盖率门槛通过：${threshold}%"
