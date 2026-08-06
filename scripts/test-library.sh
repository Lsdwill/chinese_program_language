#!/usr/bin/env bash
set -euo pipefail

bin="${1:?用法：scripts/test-library.sh ./huayan}"
bin="$(cd "$(dirname "$bin")" && pwd)/$(basename "$bin")"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cp -a examples/图书馆 "$tmp/图书馆"

printf '1\nB1\n华言\n作者\n2024\n3\nR1\n张三\n138\n5\nB1\nR1\n7\n6\nB1\nR1\n8\nB1\n0\n' |
    (cd "$tmp" && "$bin" 图书馆/主.hua >"$tmp/out" 2>"$tmp/err")

for marker in '添加图书成功' '登记读者成功' '借阅成功' '归还成功' '删除图书成功'; do
    grep -Fq "$marker" "$tmp/out"
done
grep -Fq 'B1' "$tmp/out"
[[ ! -s "$tmp/err" ]]
[[ -s "$tmp/图书馆/数据/图书馆.db" ]]
echo "图书馆端到端通过"
