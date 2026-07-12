#!/usr/bin/env bats
#
# Compatibility characterization tests for the eza-backed `ls` wrapper
# (packages/eza-ls/ls-wrapper.sh).
#
# NOTE ON SCOPE: this wrapper is deliberately NOT a drop-in, byte-for-byte
# replacement for GNU coreutils `ls`. Its job is to render eza's enhanced output
# under the `ls` command name for interactive use. These tests therefore
# *characterize* the compatibility boundary rather than assert full parity:
#   1. which ls-style options the wrapper accepts vs. rejects (getopts allow-list)
#   2. that, for the options it does accept, the set of listed entries matches
#      what GNU `ls` lists (order/format/colour intentionally differ)
#   3. the documented fallbacks (non-TTY -> /bin/ls; missing eza -> /bin/ls)
#
# The wrapper falls back to /bin/ls whenever stdout is not a TTY, so every case
# that must exercise the eza path is run under a pseudo-TTY via `script`.
#
# Run:  WRAPPER=/path/to/eza-ls/bin/ls bats packages/eza-ls/tests/compat.bats
# (defaults to ./result/bin/ls, i.e. `nix build .#eza-ls` output).

setup_file() {
  WRAPPER="${WRAPPER:-$BATS_TEST_DIRNAME/../../../result/bin/ls}"
  [[ -x $WRAPPER ]] || {
    echo "wrapper not found/executable: $WRAPPER (build with: nix build .#eza-ls)" >&2
    return 1
  }
  export WRAPPER
  export GNU_LS=/bin/ls
}

setup() {
  FIX="$BATS_TEST_TMPDIR/fix"
  mkdir -p "$FIX/sub"
  printf 'a' >"$FIX/small.txt"
  printf 'aaaaaaaaaa' >"$FIX/large.txt"
  echo hi >"$FIX/sub/inner.txt"
  ln -s small.txt "$FIX/link.txt"
}

# Run the wrapper under a pseudo-TTY (so it uses eza, not the /bin/ls fallback).
# Usage: run_tty <args...> ; sets $output / $status like bats `run`.
run_tty() {
  run script -qec "$WRAPPER $* '$FIX'" /dev/null
}

# Strip ANSI escapes + CR that `script`/eza emit, so we can compare text.
strip_ansi() { sed 's/\x1b\[[0-9;?]*[a-zA-Z]//g' | tr -d '\r'; }

# Extract the bare set of entry names the wrapper printed for $FIX, one per line,
# sorted & unique. eza appends " -> target" for symlinks; drop the arrow+target.
wrapper_names() {
  run_tty "$@"
  printf '%s\n' "$output" |
    strip_ansi |
    sed 's/ -> .*//' |
    tr -s ' ' '\n' |
    grep -vE '^$' |
    sort -u
}

gnu_names() {
  "$GNU_LS" "$@" "$FIX" |
    tr -s ' ' '\n' |
    grep -vE '^$' |
    sort -u
}

# ---------------------------------------------------------------------------
# 1. Option allow-list: which ls options the wrapper accepts vs. rejects.
# ---------------------------------------------------------------------------

# Options the wrapper maps to eza (exit 0, and NOT via the /bin/ls fallback).
@test "accepts ls-style short options it maps to eza" {
  for opt in -l -a -A -1 -x -F -R -r -d -i -S -t -u -c -h -k -o -g; do
    run_tty "$opt"
    [ "$status" -eq 0 ] || {
      echo "expected '$opt' to be accepted, got status=$status: $output" >&2
      return 1
    }
  done
}

@test "prints ls-style help for --help" {
  run_tty --help
  [ "$status" -eq 0 ]
  [[ $output == *"Mapped short:"* ]]
}

# Common long options the wrapper maps to eza.
@test "accepts common GNU ls long options (mapped to eza)" {
  for opt in --all --almost-all --long --oneline --reverse --recursive \
    --inode --human-readable --classify --group-directories-first \
    --across --color=auto --sort=size --time-style=iso; do
    run_tty "$opt"
    [ "$status" -eq 0 ] || {
      echo "expected long option '$opt' to be accepted, got status=$status: $output" >&2
      return 1
    }
  done
}

# Options eza cannot faithfully represent must NOT error out anymore: they fall
# back to /bin/ls and therefore still exit 0 with real-ls behaviour.
@test "unmappable options fall back to /bin/ls instead of erroring" {
  for opt in -Q -B -v -C -m -p -q -w80 --sort=version --format=commas; do
    run_tty "$opt"
    [ "$status" -eq 0 ] || {
      echo "expected '$opt' to succeed via /bin/ls fallback, got status=$status: $output" >&2
      return 1
    }
  done
}

# The fallback really is /bin/ls: -Q output matches `/bin/ls -Q` (same TTY).
@test "-Q falls back to /bin/ls (output matches real ls -Q)" {
  run_tty -Q
  local expected
  expected="$(script -qec "$GNU_LS -Q '$FIX'" /dev/null)"
  [ "$output" = "$expected" ]
}

# ---------------------------------------------------------------------------
# 2. Entry-set parity for the options the wrapper accepts.
#    (Order, columns and colour differ by design; only the NAME SET is compared.)
# ---------------------------------------------------------------------------

@test "default listing: same entry set as GNU ls" {
  [ "$(wrapper_names)" = "$(gnu_names)" ]
}

@test "-1 listing: same entry set as GNU ls" {
  [ "$(wrapper_names -1)" = "$(gnu_names -1)" ]
}

@test "-a listing: same entry set as GNU ls (includes . and ..)" {
  [ "$(wrapper_names -a)" = "$(gnu_names -a)" ]
}

# ---------------------------------------------------------------------------
# 3. Documented fallbacks.
# ---------------------------------------------------------------------------

@test "non-TTY (piped) falls back to /bin/ls: plain names, no eza colour/arrows" {
  # No pseudo-TTY here: stdout is a pipe, so the wrapper must exec /bin/ls.
  run bash -c "'$WRAPPER' '$FIX' | cat"
  [ "$status" -eq 0 ]
  # /bin/ls does not print the eza symlink arrow.
  [[ $output != *" -> "* ]]
  [[ $output == *"link.txt"* ]]
}

@test "non-TTY output equals /bin/ls output exactly" {
  run bash -c "'$WRAPPER' '$FIX' | cat"
  expected="$("$GNU_LS" "$FIX")"
  [ "$output" = "$expected" ]
}
