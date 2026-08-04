#!/usr/bin/env bash
set -euo pipefail

version="${1:-0.3.0}"
release="v${version}"
out="dist/huayan-${release}"
rm -rf "$out"
mkdir -p "$out"
for target in "linux amd64" "windows amd64" "darwin amd64" "darwin arm64"; do
    read -r goos goarch <<< "$target"
    suffix=""
    [[ "$goos" == windows ]] && suffix=".exe"
    name="huayan-${release}-${goos}-${goarch}"
    GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags "-s -w" -o "$out/${name}${suffix}" ./cmd/huayan
    if [[ "$goos" == windows ]]; then
        (cd "$out" && zip -q "${name}.zip" "${name}${suffix}")
    else
        tar -C "$out" -czf "$out/${name}.tar.gz" "${name}${suffix}"
    fi
done

# Keep a source snapshot alongside binaries so the release is reproducible
# without requiring the Go toolchain to be installed.
git archive --format=tar.gz --prefix="huayan-${release}/" HEAD > "$out/huayan-${release}-source.tar.gz" 2>/dev/null || \
    tar --exclude='./dist' --exclude='./.git' -czf "$out/huayan-${release}-source.tar.gz" .
(
    cd dist
    if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "huayan-${release}"/* > "huayan-${release}-SHA256SUMS"
    else
        shasum -a 256 "huayan-${release}"/* > "huayan-${release}-SHA256SUMS"
    fi
)
echo "发布文件已生成：$out"
