#!/usr/bin/env bash

# Bootstrap installer for the repository's nixcache command.
#
# The binary is published as a single tar.gz OCI layer at
# ghcr.io/curoky/standalone-binaries:nixcache-<arch>. The archive contains
# nixcache/nixcache, so a fresh CI runner only needs curl and tar.
set -euo pipefail

REGISTRY="ghcr.io"
REPOSITORY="curoky/standalone-binaries"

INSTALL_DIR="${NIXCACHE_INSTALL_DIR:-$HOME/.local/bin}"
ARCH="${NIXCACHE_ARCH:-}"

die() {
  echo "error: $*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage:
  install.sh [--prefix DIR] [--arch ARCH]

Options:
  --prefix DIR  Install directory (default: ~/.local/bin)
  --arch ARCH   linux-x86_64 | darwin-arm64

Environment:
  NIXCACHE_INSTALL_DIR
  NIXCACHE_ARCH
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --prefix)
      [ $# -ge 2 ] || die "--prefix requires a directory"
      INSTALL_DIR="$2"
      shift 2
      ;;
    --prefix=*)
      INSTALL_DIR="${1#*=}"
      shift
      ;;
    --arch)
      [ $# -ge 2 ] || die "--arch requires an architecture"
      ARCH="$2"
      shift 2
      ;;
    --arch=*)
      ARCH="${1#*=}"
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

command -v curl >/dev/null 2>&1 || die "curl is required"
command -v tar >/dev/null 2>&1 || die "tar is required"

if [ -z "$ARCH" ]; then
  case "$(uname -s)/$(uname -m)" in
    Linux/x86_64 | Linux/amd64) ARCH="linux-x86_64" ;;
    Darwin/arm64 | Darwin/aarch64) ARCH="darwin-arm64" ;;
    *) die "unsupported platform; pass --arch linux-x86_64 or darwin-arm64" ;;
  esac
fi

case "$ARCH" in
  linux-x86_64 | darwin-arm64) ;;
  *) die "unsupported architecture: $ARCH" ;;
esac

TAG="nixcache-$ARCH"
echo "> Installing nixcache ($ARCH) into $INSTALL_DIR"

token="$(curl -fsSL \
  "https://${REGISTRY}/token?scope=repository:${REPOSITORY}:pull" |
  tr ',' '\n' | grep -o '"token":"[^"]*"' | head -n1 | cut -d'"' -f4)"
[ -n "$token" ] || die "failed to obtain registry token"

manifest="$(curl -fsSL \
  -H "Authorization: Bearer ${token}" \
  -H "Accept: application/vnd.oci.image.manifest.v1+json" \
  -H "Accept: application/vnd.docker.distribution.manifest.v2+json" \
  "https://${REGISTRY}/v2/${REPOSITORY}/manifests/${TAG}")"
digest="$(printf '%s' "$manifest" |
  tr ',' '\n' | grep -o '"digest":"sha256:[a-f0-9]*"' | tail -n1 | cut -d'"' -f4)"
[ -n "$digest" ] || die "could not find layer digest for $TAG"

mkdir -p "$INSTALL_DIR"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
curl -fsSL \
  -H "Authorization: Bearer ${token}" \
  "https://${REGISTRY}/v2/${REPOSITORY}/blobs/${digest}" |
  tar -xz -C "$tmp"
[ -f "$tmp/nixcache/nixcache" ] || die "archive did not contain nixcache/nixcache"
mv -f "$tmp/nixcache/nixcache" "$INSTALL_DIR/nixcache"
chmod +x "$INSTALL_DIR/nixcache"

echo "> Installed: $INSTALL_DIR/nixcache"
