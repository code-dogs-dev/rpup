#!/bin/sh
# Download the latest rpup release for this OS/arch and install the binary.
#
#   curl -sSfL https://raw.githubusercontent.com/code-dogs-dev/rpup/main/install.sh | sh
#
# Override the destination with RPUP_INSTALL_DIR (default /usr/local/bin).
set -eu

REPO="code-dogs-dev/rpup"
BIN_DIR="${RPUP_INSTALL_DIR:-/usr/local/bin}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  darwin | linux) ;;
  *) echo "rpup: unsupported OS: $os" >&2; exit 1 ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "rpup: unsupported arch: $arch" >&2; exit 1 ;;
esac

tag=$(curl -sSfL "https://api.github.com/repos/$REPO/releases/latest" \
  | grep -m1 '"tag_name"' | cut -d'"' -f4)
if [ -z "$tag" ]; then
  echo "rpup: could not determine the latest release" >&2
  exit 1
fi
ver=${tag#v}

url="https://github.com/$REPO/releases/download/$tag/rpup_${ver}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "rpup: downloading $tag for $os/$arch..."
curl -sSfL "$url" | tar -xz -C "$tmp"

if [ -w "$BIN_DIR" ]; then
  install -m 0755 "$tmp/rpup" "$BIN_DIR/rpup"
else
  echo "rpup: $BIN_DIR is not writable, using sudo"
  sudo install -m 0755 "$tmp/rpup" "$BIN_DIR/rpup"
fi

echo "rpup: installed $ver -> $BIN_DIR/rpup"
echo "rpup: add  eval \"\$(rpup hook zsh)\"  to your shell rc to enable auto-switching"
