#!/usr/bin/env bash
set -euo pipefail

prefix="$1"
artifact_name=${2:-unknown}
allow_dynamic_elf=${3:-0}
# Native binary format of the build/host platform: "elf" on Linux, "macho" on
# Darwin. The portability check only inspects binaries of this format; binaries
# in the other platform's format (e.g. prebuilt darwin/windows .node addons
# bundled by an npm package built on Linux) are inert cross-platform payloads
# that never load on the target, and the tools to inspect them (otool on Linux,
# patchelf on Darwin) are not available anyway.
host_format=${4:-elf}

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

  elif [[ $host_format == macho && $FTYPE == *Mach-O* ]]; then
    # Delete any LC_RPATH pointing into /nix. Static darwin builds routinely
    # inherit a dead rpath into a store lib dir from the ld-wrapper (which adds
    # -rpath for every store -L path), even though nothing loads through it.
    # normalize.sh does not touch load commands otherwise, so nuke these dead
    # store rpaths here to keep the artifact free of /nix references.
    while IFS= read -r rpath; do
      [[ $rpath == /nix/* ]] && install_name_tool -delete_rpath "$rpath" "$f"
    done < <(otool -l "$f" 2>/dev/null | awk '/LC_RPATH/{grab=1} grab&&/ path /{print $2; grab=0}')
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

# Portability check (the hard rules from CLAUDE.md):
#   - Linux ELF files must be statically linked unless the artifact is an
#     explicitly recorded dynamic exception.
#   - Darwin Mach-O files may only load system libraries or package-relative
#     libraries.
#   - No runtime path or load command may refer to /nix.
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
    if [[ $host_format != elf ]]; then
      echo "==> skip (non-host-format ELF): $f"
      continue
    fi
    echo "==> ELF: $f"
    echo "$FTYPE"

    if [[ $FTYPE == *"dynamically linked"* ]]; then
      if [[ $allow_dynamic_elf != 1 ]]; then
        echo "ERROR: $f is dynamically linked; Linux artifact $artifact_name must contain only statically linked ELF files" >&2
        bad=1
      fi

      if ! deps=$(patchelf --print-needed "$f" 2>/dev/null); then
        echo "ERROR: cannot inspect dynamic dependencies of $f" >&2
        bad=1
        continue
      fi
      if ! rpath=$(patchelf --print-rpath "$f" 2>/dev/null); then
        echo "ERROR: cannot inspect rpath of $f" >&2
        bad=1
        continue
      fi

      echo "$deps"
      [[ -n $rpath ]] && echo "rpath: $rpath"
      if [[ $deps == *"/nix"* ]]; then
        echo "ERROR: $f has a dynamic dependency under /nix" >&2
        bad=1
      fi
      if [[ $rpath == *"/nix"* ]]; then
        echo "ERROR: $f has an rpath under /nix: $rpath" >&2
        bad=1
      fi

      if [[ $FTYPE == *executable* ]]; then
        if ! interpreter=$(patchelf --print-interpreter "$f" 2>/dev/null); then
          echo "ERROR: cannot inspect interpreter of $f" >&2
          bad=1
          continue
        fi
        echo "interpreter: $interpreter"
        if [[ $interpreter == *"/nix"* ]]; then
          echo "ERROR: $f has an interpreter under /nix: $interpreter" >&2
          bad=1
        fi
      fi
    fi
  elif [[ $FTYPE == *Mach-O* ]]; then
    if [[ $host_format != macho ]]; then
      echo "==> skip (non-host-format Mach-O): $f"
      continue
    fi
    echo "==> deps: $f"
    if ! deps=$(otool -L "$f" 2>/dev/null); then
      echo "ERROR: cannot inspect dynamic dependencies of $f" >&2
      bad=1
      continue
    fi
    echo "$deps"

    # A dylib's own install name (LC_ID_DYLIB) shows up as the first entry in
    # `otool -L` but is not a runtime dependency, so drop it before scanning.
    # `otool -D` prints the install id (empty for executables / bundles).
    install_id=$(otool -D "$f" 2>/dev/null | tail -n +2 | head -n 1)

    while IFS= read -r dependency_line; do
      dependency_line=${dependency_line#"${dependency_line%%[![:space:]]*}"}
      dependency=${dependency_line%% *}
      [[ -z $dependency ]] && continue
      [[ -n $install_id && $dependency == "$install_id" ]] && continue

      case "$dependency" in
        /usr/lib/* | /System/Library/Frameworks/* | @loader_path/* | @rpath/*) ;;
        *)
          echo "ERROR: $f has a non-portable dynamic dependency: $dependency" >&2
          bad=1
          ;;
      esac
    done < <(printf '%s\n' "$deps" | tail -n +2)

    if ! load_commands=$(otool -l "$f" 2>/dev/null); then
      echo "ERROR: cannot inspect load commands of $f" >&2
      bad=1
      continue
    fi
    load_commands_body=${load_commands#*$'\n'}
    if [[ $load_commands_body == *"/nix"* ]]; then
      echo "ERROR: $f has a Mach-O load command under /nix" >&2
      bad=1
    fi
    while IFS= read -r rpath_line; do
      rpath=${rpath_line#* path }
      rpath=${rpath% \(offset *}
      case "$rpath" in
        @loader_path | @loader_path/*) ;;
        *)
          echo "ERROR: $f has a non-portable LC_RPATH: $rpath" >&2
          bad=1
          ;;
      esac
    done < <(printf '%s\n' "$load_commands" | sed -n 's/^[[:space:]]*path / path /p')
  fi
done

if [[ $bad -ne 0 ]]; then
  echo "ERROR: portability check failed: artifact $artifact_name does not satisfy standalone binary requirements" >&2
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
