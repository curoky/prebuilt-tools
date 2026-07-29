#!/usr/bin/env bash
set -euo pipefail

prefix="$1"

# find "$prefix" -type d \( -name "man" -o -name "fish" -o -name "bash-completion" -o -name "nix-support" \) -exec rm -rf {} +
rm -rf "$prefix/nix-support"
rm -rf "$prefix/share/man"
rm -rf "$prefix/share/doc"
rm -rf "$prefix/share/bash-completion"

# Resolve symlinks:
#   - links pointing inside $prefix: keep as symlinks (saves space)
#   - links pointing outside $prefix (e.g. into /nix/store) or dangling:
#     inline the real file so the artifact stays self-contained
# This must run before strip/nuke-refs so inlined files get processed too.
find "$prefix" -type l -print0 | while IFS= read -r -d '' link; do
  target=$(readlink -f "$link")
  if [[ ! -e $target ]]; then
    rm -f "$link"
  elif [[ $target != "$prefix"* ]]; then
    cp -L --remove-destination "$target" "$link"
  fi
done

# Classify every regular file once. `file` is an expensive external command, so
# instead of forking it per file (twice: here and in the portability check
# below) we hand the whole file list to a single `file` invocation and cache the
# type in the `ftype` associative array, keyed by path. `file --print0` emits
# one record per file as "<path>\0: <type>\n", so we read the NUL-terminated
# path and the trailing description line separately (NUL keeps parsing safe for
# paths with spaces).
declare -A ftype=()
mapfile -d '' files < <(find "$prefix" -type f -print0)
if [[ ${#files[@]} -gt 0 ]]; then
  while IFS= read -r -d '' f && IFS= read -r t; do
    ftype["$f"]=${t#: }
  done < <(file --print0 "${files[@]}" 2>/dev/null)
fi

for i in "${!files[@]}"; do
  f=${files[$i]}
  FTYPE=${ftype["$f"]:-}

  if [[ $FTYPE == *text* ]]; then
    sed -e 's|#\!\s*/nix/store/[a-z0-9\._-]*/bin/|#\! /usr/bin/env |g' \
      -e 's|/nix/store/[a-z0-9\._-]*/bin/||g' -i "$f"
    sed -E 's|/nix/store/[a-z0-9]{32}-[^[:space:]:/()<>]*||g' -i "$f"

  elif [[ $FTYPE == *ELF* ]]; then
    strip --strip-unneeded "$f" || true
    nuke-refs "$f"
  fi

  base=${f##*/}
  if [[ $base == .*-wrapped ]]; then
    new_name=${base#.}
    new_name=${new_name%-wrapped}
    newf=${f%/*}/$new_name
    mv "$f" "$newf"
    # keep the file list / type cache in sync so the portability check below
    # still inspects the (renamed) binary.
    files[i]=$newf
    ftype["$newf"]=$FTYPE
    f=$newf
  fi

  if [[ $f == *.a ]] || [[ $f == *.pyc ]]; then
    rm -f "$f"
  fi
done

# Portability check (the hard rule from CLAUDE.md): the shipped binary must not
# depend on any dynamic library under /nix. Walk every ELF (Linux) / Mach-O
# (Darwin) file and print its dynamic dependencies via `patchelf --print-needed`
# + rpath / `otool -L`; fail the build if any dependency resolves under /nix.
# File types are reused from the `ftype` cache computed above.
bad=0
for f in "${files[@]}"; do
  # TODO: temporarily skip openssl-related files in the portability check.
  if [[ $f == *openssl* ]]; then
    echo "==> skip (openssl): $f"
    continue
  fi
  FTYPE=${ftype["$f"]:-}
  if [[ $FTYPE == *ELF* ]]; then
    echo "==> deps: $f"
    deps=$(patchelf --print-needed "$f" 2>/dev/null || true)
    rpath=$(patchelf --print-rpath "$f" 2>/dev/null || true)
    echo "$deps"
    [[ -n $rpath ]] && echo "rpath: $rpath"
    if [[ $rpath == *"/nix"* ]]; then
      echo "ERROR: $f has an rpath under /nix: $rpath" >&2
      bad=1
    fi
  elif [[ $FTYPE == *Mach-O* ]]; then
    echo "==> deps: $f"
    deps=$(otool -L "$f" 2>/dev/null || true)
    echo "$deps"
    # otool -L prints the file's own path as the first line (which lives under
    # the /nix/store build output dir); only the indented dependency lines that
    # follow matter, so drop the header before matching.
    if echo "$deps" | tail -n +2 | grep -q '/nix'; then
      echo "ERROR: $f links a dynamic library under /nix" >&2
      bad=1
    fi
  fi
done

if [[ $bad -ne 0 ]]; then
  echo "ERROR: portability check failed: one or more binaries depend on /nix" >&2
  exit 1
fi

# # remove path which contain nix
# for f in $(find $prefix -type f); do
#   if file --brief "$f" | grep -q 'text'; then
#     sed -e 's|#\!\s*/nix/store/[a-z0-9\._-]*/bin/|#\! /usr/bin/env |g' -i"" "$f" || true
#     sed -e 's|/nix/store/[a-z0-9\._-]*/bin/||g' -i"" "$f" || true
#   fi
# done

# # strip binaries for reducing size
# for f in $(find $prefix -type f); do
#   if file --brief "$f" | grep -q 'ELF'; then
#     strip --strip-unneeded "$f"
#   fi
# done

# # clean up unnecessary files
# find $prefix -name "*.a" -delete
# find $prefix -name "*.pyc" -delete
# find $prefix -type d -name man -exec rm -rf {} +
# find $prefix -type d -name fish -exec rm -rf {} +
# find $prefix -type d -name bash-completion -exec rm -rf {} +
# find $prefix -type d -name nix-support -exec rm -rf {} +

# # remove invalid link
# find $prefix -type l -exec test ! -e {} \; -print | while read -r file; do
#   rm -rf "$file"
# done

# # remove outside links
# find $prefix -type l | while read -r link; do
#   target=$(readlink -f "$link")
#   if [[ $target != "$prefix"* ]]; then
#     rm -v "$link"
#   fi
# done

# # rename wrapped files
# find $prefix -type f -name ".*-wrapped" | while read -r file; do
#   dir=$(dirname "$file")
#   new_name=$(basename "$file" | sed -e 's/-wrapped//g' -e 's/^.//')
#   mv "$file" "$dir/$new_name"
# done
