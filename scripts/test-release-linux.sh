#!/usr/bin/env bash
set -euo pipefail

bin="${1:?用法：scripts/test-release-linux.sh ./huayan-v0.4.0-linux-amd64}"
root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cp "$root/tests/release/文件编码数据库.hua" "$tmp/"
cp "$root/tests/release/命令行参数.hua" "$tmp/"

output="$($bin "$tmp/文件编码数据库.hua")"
grep -Fqx '华言 Linux' <<<"$output"
grep -Fqx 'e58d8ee8a880204c696e7578' <<<"$output"
grep -Fqx '一' <<<"$output"
grep -Fqx '空' <<<"$output"
[[ -s "$tmp/二进制.dat" ]]

args_output="$($bin "$tmp/命令行参数.hua" -- 一个参数 二号)"
grep -Fq '一个参数' <<<"$args_output"
grep -Fq '二号' <<<"$args_output"

set +e
error_output="$($bin "$root/tests/release/运行时错误.hua" 2>&1 >/dev/null)"
status=$?
set -e
[[ "$status" -eq 1 ]]
grep -Fq 'Linux 黑盒错误测试' <<<"$error_output"

echo 'Linux 发布包场景测试通过'
