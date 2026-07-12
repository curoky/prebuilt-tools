#!/usr/bin/env bash
#
# ls-compatible front-end backed by eza (see packages/eza-ls/default.nix).
#
# Goal: behave like an enhanced `ls` for interactive use while accepting as many
# common GNU `ls` options as can be faithfully mapped to eza. Anything eza cannot
# express (e.g. -Q/-C/-m/-w, --sort=version) transparently falls back to the
# system /bin/ls, so no supported real-`ls` invocation errors out — it just loses
# the eza enhancement for that one call.
#
# Adapts the eggbean eza gist (same script as /opt/devspace/tools/eza-wrapper.sh)
# but is organized as data-driven mapping tables so option support is edited in
# one place:
#   * SHORT_MAP  : ls short flag        -> eza args (no side effects)
#   * SORT_SHORT : ls sort short flag   -> eza sort field (also reverses order)
#   * LONG_ALIAS : ls long option       -> equivalent ls short flag
#   * LONG_MAP   : ls long option       -> eza args (no short-flag equivalent)
# Value-bearing options (-I, --ignore, --sort, --time-style, --color) and the
# fallback for anything unmapped are handled explicitly in parse_args().
#
# The only packaging change vs. the gist is invoking the co-located ./eza so
# `ls` is self-contained.

set -uo pipefail

script_path="$(readlink -f "$0")"
root="$(cd "$(dirname "$script_path")" && pwd)"
readonly eza="$root/eza"

# Delegate to the real ls (preferred /bin/ls, else /usr/bin/ls). As a last
# resort on a host without any system ls, let eza produce a plain listing so we
# still output something. exec => this replaces the wrapper process.
fallback() {
  if [[ -x /bin/ls ]]; then
    exec /bin/ls "$@"
  elif [[ -x /usr/bin/ls ]]; then
    exec /usr/bin/ls "$@"
  else
    exec "$eza" "$@"
  fi
}

# Fall back early when eza is unusable, or when stdout is not a terminal (so
# piped/`ls | ...` output stays byte-for-byte identical to real ls).
[[ -x $eza ]] || fallback "$@"
[[ -t 1 ]] || fallback "$@"

# ---------------------------------------------------------------------------
# Mapping tables. Values are space-separated eza argument lists.
# ---------------------------------------------------------------------------
# ls short flag -> eza args (side-effect free). `-a` is doubled so eza shows the
# implied . and .. entries, matching `ls -a`.
declare -rA SHORT_MAP=(
  [a]="-a -a"
  [A]="-a"
  [1]="-1"
  [l]="-l"
  [x]="-x"
  [F]="-F"
  [R]="-R"
  [d]="-d"
  [i]="-i"
  [o]="--octal-permissions"
)

# ls sort short flag -> eza sort field. These also imply a reverse so the newest
# / largest sort out the same way GNU ls orders them.
declare -rA SORT_SHORT=(
  [t]="modified"
  [u]="accessed"
  [c]="changed"
  [S]="size"
  [X]="extension"
)

# ls long option -> equivalent ls short flag (kept in one place so synonyms
# never drift from their short forms).
declare -rA LONG_ALIAS=(
  [all]="a"
  [almost-all]="A"
  [oneline]="1"
  [long]="l"
  [across]="x"
  [classify]="F"
  [recursive]="R"
  [recurse]="R"
  [reverse]="r"
  [inode]="i"
  [human-readable]="h"
  [sort-by-size]="S"
)

# ls long option -> eza args (no short-flag equivalent).
declare -rA LONG_MAP=(
  [group-directories-first]="--group-directories-first"
)

# ---------------------------------------------------------------------------
# State accumulated while parsing.
# ---------------------------------------------------------------------------
eza_opts=()
operands=()
want_reverse=0    # set by -r and by the sort flags
human=1           # 1 = human-readable sizes (default), <=0 = raw bytes (-k)
color="always"    # eza --color value (TTY output, so colour by default)

help() {
  cat <<EOF
  ${0##*/} — eza-backed ls. Common GNU ls options are mapped to eza; options eza
  cannot represent fall back to the system /bin/ls.

  Mapped short: -a -A -1 -l -x -F -R -r -d -i -o -h -k  and sorts -t -u -c -S -X
  Mapped long : --all --almost-all --oneline --long --across --classify
                --recursive --reverse --inode --human-readable
                --group-directories-first --color[=WHEN] --sort=WORD
                --time-style=STYLE --ignore=GLOB --help --version(->/bin/ls)

  --sort=WORD: none, size, time, extension, name (ls aliases accepted).
  Anything else (e.g. -Q -C -m -w -p -v, --sort=version) -> /bin/ls fallback.
EOF
  exit 0
}

# ls --sort=WORD value -> eza sort field, or non-zero if eza has no equivalent.
map_sort_word() {
  case "$1" in
    none | None) echo none ;;
    size) echo size ;;
    time | mtime | modified) echo modified ;;
    ctime | changed) echo changed ;;
    atime | accessed) echo accessed ;;
    extension) echo extension ;;
    name) echo name ;;
    *) return 1 ;;
  esac
}

# Apply one ls short flag letter, consulting the tables. Returns non-zero when
# the letter has no eza mapping, so the caller can fall back to /bin/ls.
apply_short() {
  local letter="$1"
  if [[ -n ${SHORT_MAP[$letter]:-} ]]; then
    # shellcheck disable=SC2206 # intentional word-split of space-separated args
    eza_opts+=(${SHORT_MAP[$letter]})
  elif [[ -n ${SORT_SHORT[$letter]:-} ]]; then
    eza_opts+=(-s "${SORT_SHORT[$letter]}")
    want_reverse=1
  else
    case "$letter" in
      r) want_reverse=1 ;;
      h) human=1 ;;
      k) human=0 ;;
      *) return 1 ;;
    esac
  fi
}

# ---------------------------------------------------------------------------
# Argument parsing. Any unmappable option makes the whole call fall back to the
# system ls (with the original, unmodified "$@").
# ---------------------------------------------------------------------------
parse_args() {
  local -a argv=("$@")
  local i=0 arg letter rest field alias
  while ((i < ${#argv[@]})); do
    arg="${argv[i]}"
    case "$arg" in
      --) # end of options: everything after is an operand
        ((i++))
        while ((i < ${#argv[@]})); do
          operands+=("${argv[i]}")
          ((i++))
        done
        break
        ;;
      --help) help ;;
      --version) fallback "$@" ;;
      --color | --colour) color="always" ;;
      --color=* | --colour=*) color="${arg#*=}" ;;
      --ignore=*) eza_opts+=(--ignore-glob="${arg#*=}") ;;
      --ignore)
        ((i++))
        eza_opts+=(--ignore-glob="${argv[i]:-}")
        ;;
      --time-style=*) eza_opts+=(--time-style="${arg#*=}") ;;
      --time-style)
        ((i++))
        eza_opts+=(--time-style="${argv[i]:-}")
        ;;
      --sort=*)
        field="$(map_sort_word "${arg#*=}")" || fallback "$@"
        eza_opts+=(-s "$field")
        ;;
      --sort)
        ((i++))
        field="$(map_sort_word "${argv[i]:-}")" || fallback "$@"
        eza_opts+=(-s "$field")
        ;;
      --*) # table-driven long options, else fall back
        alias="${arg#--}"
        if [[ -n ${LONG_ALIAS[$alias]:-} ]]; then
          apply_short "${LONG_ALIAS[$alias]}" || fallback "$@"
        elif [[ -n ${LONG_MAP[$alias]:-} ]]; then
          # shellcheck disable=SC2206
          eza_opts+=(${LONG_MAP[$alias]})
        else
          fallback "$@"
        fi
        ;;
      -I)
        ((i++))
        eza_opts+=(--ignore-glob="${argv[i]:-}")
        ;;
      -I*) eza_opts+=(--ignore-glob="${arg#-I}") ;;
      -?*) # clustered / single short flags: split into letters
        rest="${arg#-}"
        while [[ -n $rest ]]; do
          letter="${rest:0:1}"
          rest="${rest:1}"
          apply_short "$letter" || fallback "$@"
        done
        ;;
      *) operands+=("$arg") ;; # path or "-"
    esac
    ((i++))
  done
}

parse_args "$@"

# ---------------------------------------------------------------------------
# Enhanced defaults applied on top of the parsed options.
#   -g  show group    -H show hardlinks  -b binary size units  --git status
# (These mirror the reference gist's "eza feature" defaults.)
# ---------------------------------------------------------------------------
((want_reverse)) && eza_opts+=(-r)
((human <= 0)) && eza_opts+=(-B) || eza_opts+=(-b)
eza_opts+=(-g -H --color="$color")

# Show git status only inside a work tree (avoids errors/delay elsewhere).
if [[ $(git -C "${operands[0]:-.}" rev-parse --is-inside-work-tree 2>/dev/null) == true ]]; then
  eza_opts+=(--git)
fi

exec -a ls "$eza" --color-scale "${eza_opts[@]}" "${operands[@]}"
