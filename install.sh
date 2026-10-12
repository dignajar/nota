#!/bin/sh
# Install nota from a GitHub release, verifying its sha256 checksum.
#
#   curl -fsSL https://raw.githubusercontent.com/dignajar/nota/main/install.sh | sh
#
# Environment:
#   NOTA_VERSION   version to install, e.g. v0.1.0 (default: latest release)
#   NOTA_BIN_DIR   install directory (default: ~/.local/bin)
set -eu

REPO="dignajar/nota"
BIN_DIR="${NOTA_BIN_DIR:-$HOME/.local/bin}"
# NOTA_BASE_URL is only meant for testing the script against a local mirror.
BASE_URL="${NOTA_BASE_URL:-https://github.com/$REPO/releases}"

say() { printf '%s\n' "$*" >&2; }
die() { say "install.sh: $*"; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required"; }

need curl
need tar
need uname

case "$(uname -s)" in
  Linux)  os=linux ;;
  Darwin) os=darwin ;;
  *) die "unsupported OS: $(uname -s) (nota supports Linux and macOS)" ;;
esac
case "$(uname -m)" in
  x86_64|amd64)  arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

if command -v sha256sum >/dev/null 2>&1; then
  sha256() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
  sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
  die "sha256sum or shasum is required"
fi

version="${NOTA_VERSION:-}"
if [ -z "$version" ]; then
  # /releases/latest redirects to /releases/tag/<version>
  version="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$BASE_URL/latest" | sed 's|.*/||')"
  [ -n "$version" ] && [ "$version" != "latest" ] || die "could not find the latest release"
fi
case "$version" in v*) ;; *) version="v$version" ;; esac

archive="nota_${version#v}_${os}_${arch}.tar.gz"
url="$BASE_URL/download/$version"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

say "Downloading nota $version for $os/$arch"
curl -fsSL -o "$tmp/$archive" "$url/$archive" || die "download failed: $url/$archive"
curl -fsSL -o "$tmp/checksums.txt" "$url/checksums.txt" || die "download failed: $url/checksums.txt"

want="$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")"
[ -n "$want" ] || die "$archive is not listed in checksums.txt"
got="$(sha256 "$tmp/$archive")"
[ "$want" = "$got" ] || die "checksum mismatch for $archive (expected $want, got $got)"

tar -xzf "$tmp/$archive" -C "$tmp" nota
mkdir -p "$BIN_DIR"
install -m 0755 "$tmp/nota" "$BIN_DIR/nota"

say "Installed $BIN_DIR/nota ($("$BIN_DIR/nota" version))"
case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) say "Note: $BIN_DIR is not on your PATH." ;;
esac
say "Next: nota install-service && nota doctor"
