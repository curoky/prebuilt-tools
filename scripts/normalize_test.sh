#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
normalize="$repo_root/scripts/normalize.sh"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$tmp/mock-common"
cat >"$tmp/mock-common/nuke-refs" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$tmp/mock-common/nuke-refs"

pass=0
static_elf=$(command -v patchelf)

expect_success() {
  local name=$1
  shift

  if output=$("$@" 2>&1); then
    pass=$((pass + 1))
    printf 'ok %d - %s\n' "$pass" "$name"
  else
    printf 'not ok - %s\n%s\n' "$name" "$output" >&2
    exit 1
  fi
}

expect_failure() {
  local name=$1
  local expected=$2
  shift 2

  if output=$("$@" 2>&1); then
    printf 'not ok - %s: command unexpectedly succeeded\n' "$name" >&2
    exit 1
  fi
  if [[ $output != *"$expected"* ]]; then
    printf 'not ok - %s: missing expected error %q\n%s\n' "$name" "$expected" "$output" >&2
    exit 1
  fi

  pass=$((pass + 1))
  printf 'ok %d - %s\n' "$pass" "$name"
}

make_linux_fixture() {
  local name=$1
  local source=$2
  local prefix="$tmp/$name"

  mkdir -p "$prefix/bin"
  cp "$source" "$prefix/bin/$name"
  printf '%s\n' "$prefix"
}

static_prefix=$(make_linux_fixture static "$static_elf")
expect_success "static ELF is accepted" \
  env PATH="$tmp/mock-common:$PATH" bash "$normalize" "$static_prefix" static 0

dynamic_prefix=$(make_linux_fixture dynamic-tool /bin/true)
expect_failure "dynamic ELF is rejected" "is dynamically linked" \
  env PATH="$tmp/mock-common:$PATH" bash "$normalize" "$dynamic_prefix" dynamic-tool 0

openssl_prefix=$(make_linux_fixture openssl /bin/true)
expect_success "openssl-named files are skipped" \
  env PATH="$tmp/mock-common:$PATH" bash "$normalize" "$openssl_prefix" openssl 0

exception_prefix=$(make_linux_fixture music-decrypto /bin/true)
expect_success "recorded dynamic ELF exception is accepted" \
  env PATH="$tmp/mock-common:$PATH" bash "$normalize" "$exception_prefix" music-decrypto 1

bad_exception_prefix=$(make_linux_fixture music-decrypto-bad-rpath /bin/true)
patchelf --set-rpath /nix/store/not-portable "$bad_exception_prefix/bin/music-decrypto-bad-rpath"
expect_failure "dynamic exception still rejects Nix rpath" "has an rpath under /nix" \
  env PATH="$tmp/mock-common:$PATH" bash "$normalize" "$bad_exception_prefix" music-decrypto 1

mkdir -p "$tmp/mock-darwin"
cat >"$tmp/mock-darwin/file" <<'EOF'
#!/usr/bin/env bash
shift
for path in "$@"; do
  printf '%s\0: Mach-O 64-bit executable arm64\n' "$path"
done
EOF
cat >"$tmp/mock-darwin/otool" <<'EOF'
#!/usr/bin/env bash
mode=$1
path=$2
name=${path##*/}

case "$mode:$name" in
  -L:portable)
    printf '%s:\n' "$path"
    printf '\t/usr/lib/libSystem.B.dylib (compatibility version 1.0.0, current version 1.0.0)\n'
    printf '\t@loader_path/libhelper.dylib (compatibility version 0.0.0, current version 0.0.0)\n'
    ;;
  -L:bad-absolute)
    printf '%s:\n' "$path"
    printf '\t/opt/homebrew/lib/libhelper.dylib (compatibility version 0.0.0, current version 0.0.0)\n'
    ;;
  -L:bad-load-command)
    printf '%s:\n' "$path"
    printf '\t/usr/lib/libSystem.B.dylib (compatibility version 1.0.0, current version 1.0.0)\n'
    ;;
  -l:bad-load-command)
    printf '%s:\n' "$path"
    printf '          cmd LC_RPATH\n'
    printf '         path /nix/store/not-portable/lib (offset 12)\n'
    ;;
  -L:bad-rpath)
    printf '%s:\n' "$path"
    printf '\t@rpath/libhelper.dylib (compatibility version 0.0.0, current version 0.0.0)\n'
    ;;
  -l:bad-rpath)
    printf '%s:\n' "$path"
    printf '          cmd LC_RPATH\n'
    printf '         path /opt/homebrew/lib (offset 12)\n'
    ;;
  -l:*)
    printf '%s:\n' "$path"
    printf 'Load command 0\n'
    printf '          cmd LC_RPATH\n'
    printf '         path @loader_path/../lib (offset 12)\n'
    ;;
  *)
    exit 1
    ;;
esac
EOF
chmod +x "$tmp/mock-darwin/file" "$tmp/mock-darwin/otool"

make_darwin_fixture() {
  local name=$1
  local prefix="$tmp/darwin-$name"

  mkdir -p "$prefix/bin"
  printf 'fixture\n' >"$prefix/bin/$name"
  printf '%s\n' "$prefix"
}

portable_prefix=$(make_darwin_fixture portable)
expect_success "Darwin system and package-relative dependencies are accepted" \
  env PATH="$tmp/mock-darwin:$tmp/mock-common:$PATH" bash "$normalize" "$portable_prefix" portable 0 macho

bad_absolute_prefix=$(make_darwin_fixture bad-absolute)
expect_failure "Darwin non-system absolute dependency is rejected" "non-portable dynamic dependency" \
  env PATH="$tmp/mock-darwin:$tmp/mock-common:$PATH" bash "$normalize" "$bad_absolute_prefix" bad-absolute 0 macho

bad_load_command_prefix=$(make_darwin_fixture bad-load-command)
expect_failure "Darwin Nix load command is rejected" "Mach-O load command under /nix" \
  env PATH="$tmp/mock-darwin:$tmp/mock-common:$PATH" bash "$normalize" "$bad_load_command_prefix" bad-load-command 0 macho

bad_rpath_prefix=$(make_darwin_fixture bad-rpath)
expect_failure "Darwin absolute LC_RPATH is rejected" "non-portable LC_RPATH" \
  env PATH="$tmp/mock-darwin:$tmp/mock-common:$PATH" bash "$normalize" "$bad_rpath_prefix" bad-rpath 0 macho

# Cross-platform payloads: an npm package built on one platform bundles prebuilt
# native addons for the *other* platform. Those inert binaries must be skipped
# (and the inspection tool for them is absent anyway), not treated as failures.
cross_macho_prefix=$(make_darwin_fixture bad-load-command)
expect_success "non-host-format Mach-O is skipped on an elf host" \
  env PATH="$tmp/mock-darwin:$tmp/mock-common:$PATH" bash "$normalize" "$cross_macho_prefix" cross-macho 0 elf

cross_elf_prefix=$(make_linux_fixture cross-elf /bin/true)
expect_success "non-host-format ELF is skipped on a macho host" \
  env PATH="$tmp/mock-common:$PATH" bash "$normalize" "$cross_elf_prefix" cross-elf 0 macho

printf '1..%d\n' "$pass"
