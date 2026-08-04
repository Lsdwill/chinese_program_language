#!/usr/bin/env bash
set -euo pipefail

bin="${1:?用法：scripts/check-conformance.sh ./huayan}"
root="${2:-tests/conformance}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
count=0

while IFS= read -r -d '' file; do
    rel="${file#"$root"/}"
    stem="${rel%.hua}"
    out="$tmp/out"
    err="$tmp/err"
    set +e
    "$bin" "$file" >"$out" 2>"$err"
    status=$?
    set -e
    expected_status="$(tr -d '[:space:]' < "$root/$stem.exit")"
    if [[ "$status" != "$expected_status" ]]; then
        echo "退出码不匹配：$rel，实际 $status，期望 $expected_status" >&2
        exit 1
    fi
    cmp "$out" "$root/$stem.out"
    expected_err="$(cat "$root/$stem.err")"
    if [[ "$expected_err" == "<空>" ]]; then
        [[ ! -s "$err" ]] || { echo "标准错误不为空：$rel" >&2; exit 1; }
    elif [[ "$expected_err" == '<包含>'* ]]; then
        needle="${expected_err#<包含>}"
        grep -Fqx "$needle" "$err" 2>/dev/null || grep -Fq "$needle" "$err" || {
            echo "标准错误不包含期望内容：$rel" >&2
            exit 1
        }
    else
        cmp "$err" "$root/$stem.err"
    fi
    count=$((count + 1))
done < <(find "$root" -type f -name '*.hua' -print0 | sort -z)

[[ "$count" -gt 0 ]] || { echo "没有一致性案例" >&2; exit 1; }
echo "一致性通过：$count 个案例"
