#!/usr/bin/env bash

# Bootstrap installer for the `bm` command.
#
# `binman` is published as an OCI artifact at
# ghcr.io/curoky/standalone-binaries under the tag `binman-<arch>` (a single
# .tar.gz layer containing ./binman/bm). On a fresh, minimal host there is
# no oras/go/nix, only curl + tar, so this script pulls the layer blob directly
# over the ghcr registry HTTP API and drops the `bm` binary onto PATH.
# Afterwards `bm` self-upgrades like any other package.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/curoky/standalone-binaries/master/cmd/binman/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/curoky/standalone-binaries/master/cmd/binman/install.sh | bash -s -- --prefix /usr/local/bin
#   curl -fsSL https://raw.githubusercontent.com/curoky/standalone-binaries/master/cmd/binman/install.sh | bash -s -- wget
#
# With package arguments the script bootstraps `bm`, then asks it to download
# and extract those packages into the current directory without installing
# package-manager state.
#
# Overrides (env or flag, flag wins):
#   BINMAN_INSTALL_DIR / --prefix DIR   install directory (default: ~/.local/bin)
#   BINMAN_ARCH        / --arch ARCH    arch tag: linux-x86_64 | linux-arm64 | darwin-arm64
set -euo pipefail

REGISTRY="ghcr.io"
REPOSITORY="curoky/standalone-binaries"

INSTALL_DIR="${BINMAN_INSTALL_DIR:-$HOME/.local/bin}"
ARCH="${BINMAN_ARCH:-}"
PACKAGES=()

die() {
  echo "error: $*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage:
  install.sh [--prefix DIR] [--arch ARCH] [package...]

Options:
  --prefix DIR  Install directory (default: ~/.local/bin)
  --arch ARCH   linux-x86_64 | linux-arm64 | darwin-arm64
  package...    Download and extract packages into the current directory

Environment:
  BINMAN_INSTALL_DIR
  BINMAN_ARCH
EOF
}

# Parse flags (allow `bash -s -- --prefix ... --arch ...`).
while [ $# -gt 0 ]; do
  case "$1" in
    --prefix | --prefix=*)
      if [ "$1" = "--prefix" ]; then
        INSTALL_DIR="$2"
        shift 2
      else
        INSTALL_DIR="${1#*=}"
        shift
      fi
      ;;
    --arch | --arch=*)
      if [ "$1" = "--arch" ]; then
        ARCH="$2"
        shift 2
      else
        ARCH="${1#*=}"
        shift
      fi
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    --) # everything after -- is a package name
      shift
      while [ $# -gt 0 ]; do
        PACKAGES+=("$1")
        shift
      done
      ;;
    -*) die "unknown argument: $1" ;;
    *) # positional: treat as a package name
      PACKAGES+=("$1")
      shift
      ;;
  esac
done

command -v curl >/dev/null 2>&1 || die "curl is required"
command -v tar >/dev/null 2>&1 || die "tar is required"

# Detect the publish arch tag, mirroring detectArch() in main.go. Only
# linux-x86_64, linux-arm64 and darwin-arm64 are published; anything else must
# use --arch.
if [ -z "$ARCH" ]; then
  os="$(uname -s)"
  machine="$(uname -m)"
  case "$os/$machine" in
    Linux/x86_64 | Linux/amd64) ARCH="linux-x86_64" ;;
    Linux/aarch64 | Linux/arm64) ARCH="linux-arm64" ;;
    Darwin/arm64 | Darwin/aarch64) ARCH="darwin-arm64" ;;
    *) die "unsupported platform $os/$machine; pass --arch linux-x86_64, linux-arm64 or darwin-arm64" ;;
  esac
fi

TAG="binman-$ARCH"

echo "> Installing bm ($ARCH) into $INSTALL_DIR"

# Run each retry in a fresh process so curl re-resolves registry DNS instead of
# repeatedly connecting to the same unhealthy edge address.
CURL_ARGS=(
  -4
  -fsSL
  --connect-timeout 20
)
CURL_ATTEMPTS=6
CURL_RETRY_DELAY=2

curl_with_retry() {
  local request="$1"
  shift
  local attempt
  for ((attempt = 1; attempt <= CURL_ATTEMPTS; attempt++)); do
    if command -v getent >/dev/null 2>&1; then
      getent ahostsv4 "$REGISTRY" >&2 || true
    fi
    if curl "${CURL_ARGS[@]}" \
      --write-out "> ${request} attempt ${attempt}/${CURL_ATTEMPTS} remote_ip=%{remote_ip} http=%{http_code} connect=%{time_connect} tls=%{time_appconnect}\n" \
      "$@"; then
      return 0
    fi
    if [ "$attempt" -eq "$CURL_ATTEMPTS" ]; then
      return 1
    fi
    echo "warning: ${request} attempt ${attempt}/${CURL_ATTEMPTS} failed; retrying in ${CURL_RETRY_DELAY}s" >&2
    sleep "$CURL_RETRY_DELAY"
  done
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# 1. Complete the standard registry challenge before requesting a pull token.
# A 401 response is expected and confirms that the registry endpoint is ready.
curl_with_retry "registry challenge" \
  --no-fail \
  -o /dev/null \
  "https://${REGISTRY}/v2/"

# 2. Anonymous pull token for ghcr.
curl_with_retry "registry token" \
  -o "$tmp/token.json" \
  "https://${REGISTRY}/token?scope=repository:${REPOSITORY}:pull"
token="$(tr ',' '\n' <"$tmp/token.json" |
  grep -o '"token":"[^"]*"' | head -n1 | cut -d'"' -f4)"
[ -n "$token" ] || die "failed to obtain registry token"

# 3. Resolve the manifest and pull out the single layer digest.
curl_with_retry "manifest" \
  -H "Authorization: Bearer ${token}" \
  -H "Accept: application/vnd.oci.image.manifest.v1+json" \
  -H "Accept: application/vnd.docker.distribution.manifest.v2+json" \
  -o "$tmp/manifest.json" \
  "https://${REGISTRY}/v2/${REPOSITORY}/manifests/${TAG}"

# The artifact has one content layer; take the last digest in the layers array.
digest="$(tr ',' '\n' <"$tmp/manifest.json" |
  grep -o '"digest":"sha256:[a-f0-9]*"' | tail -n1 | cut -d'"' -f4)"
[ -n "$digest" ] || die "could not find layer digest for $TAG (is it published?)"

# 4. Download the blob into a temp dir, then move the bm binary into place.
# The archive layout is ./binman/bm; extracting to a tmp dir avoids tar
# member-match quirks (leading "./", matching the dir as well as the file).
# Using a file also prevents retried responses from being concatenated in a
# streaming tar pipeline.
curl_with_retry "blob" \
  -H "Authorization: Bearer ${token}" \
  -o "$tmp/binman.tar.gz" \
  "https://${REGISTRY}/v2/${REPOSITORY}/blobs/${digest}"
tar -xzf "$tmp/binman.tar.gz" -C "$tmp"
[ -f "$tmp/binman/bm" ] || die "archive did not contain binman/bm"
mkdir -p "$INSTALL_DIR"
mv -f "$tmp/binman/bm" "$INSTALL_DIR/bm"

chmod +x "$INSTALL_DIR/bm"

echo "> Installed: $INSTALL_DIR/bm"

# A failing self-check below must not change the installer's exit status: the
# binary is already in place. Probe it, but always report success for the
# install itself.
if "$INSTALL_DIR/bm" --help >/dev/null 2>&1; then
  case ":$PATH:" in
    *":$INSTALL_DIR:"*) echo "> bm is on PATH and ready." ;;
    *)
      echo "> Note: $INSTALL_DIR is not on PATH. Add it, e.g.:"
      echo "    export PATH=\"$INSTALL_DIR:\$PATH\""
      ;;
  esac
  if [ "${#PACKAGES[@]}" -gt 0 ]; then
    "$INSTALL_DIR/bm" download --arch "$ARCH" "${PACKAGES[@]}" ||
      die "bm download failed for: ${PACKAGES[*]}"
  fi
else
  echo "> Warning: $INSTALL_DIR/bm was installed but could not be executed here" >&2
  echo "  (possible noexec mount or libc mismatch). Try running it directly." >&2
  if [ "${#PACKAGES[@]}" -gt 0 ]; then
    die "cannot download packages (${PACKAGES[*]}): bm is not runnable here"
  fi
fi

exit 0
