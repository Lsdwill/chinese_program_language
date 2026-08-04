#!/usr/bin/env bash
set -euo pipefail

bin="${1:?用法：scripts/test-library.sh ./huayan}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cp -a examples/图书馆 "$tmp/图书馆"

printf '3\nR1\n张三\n138\n1\nB1\n华言\n作者\n2024\n4\nB1\nR1\n6\n5\nB1\n7\nB1\n0\n' |
    (cd /tmp && "$bin" "$tmp/图书馆/主.hua" >"$tmp/out" 2>"$tmp/err")

for marker in 登记成功 添加成功 借阅成功 归还成功 删除成功; do
    grep -Fq "$marker" "$tmp/out"
done
grep -Fq 'B1' "$tmp/out"
[[ ! -s "$tmp/err" ]]
[[ "$(cat "$tmp/图书馆/数据/图书.json")" == '[]' ]]
echo "图书馆端到端通过"
