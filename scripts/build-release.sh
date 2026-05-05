#!/usr/bin/env bash
set -euo pipefail

version="${1:-0.2.16}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dist="$root/dist"
mkdir -p "$dist"
rm -f "$dist"/vmrelay_*.tar.gz "$dist"/checksums.txt

platforms=(
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
)

for platform in "${platforms[@]}"; do
  read -r goos goarch <<<"$platform"
  outdir="$dist/vmrelay_${goos}_${goarch}"
  rm -rf "$outdir"
  mkdir -p "$outdir"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags "-s -w -X main.version=${version}" -o "$outdir/vmrelay" "$root/cmd/vmrelay"
  tar -C "$outdir" -czf "$dist/vmrelay_${goos}_${goarch}.tar.gz" vmrelay
done

(cd "$dist" && sha256sum vmrelay_*.tar.gz > checksums.txt)
cat "$dist/checksums.txt"
