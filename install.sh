#!/usr/bin/env bash
set -euo pipefail

repo="brontoguana/vmrelay"
install_dir="${VMRELAY_INSTALL_DIR:-/usr/local/bin}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$os" in
  darwin|linux) ;;
  *) echo "Unsupported OS: $os" >&2; exit 1 ;;
esac

case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
esac

asset="vmrelay_${os}_${arch}.tar.gz"
url="https://github.com/${repo}/releases/latest/download/${asset}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

curl -fL "$url" -o "$tmp/$asset"
tar -xzf "$tmp/$asset" -C "$tmp"

mkdir -p "$install_dir" 2>/dev/null || true
if [ -w "$install_dir" ]; then
  install -m 0755 "$tmp/vmrelay" "$install_dir/vmrelay"
else
  sudo install -m 0755 "$tmp/vmrelay" "$install_dir/vmrelay"
fi

"$install_dir/vmrelay" --version
