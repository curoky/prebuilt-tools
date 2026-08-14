#!/bin/sh
set -eu

case $0 in
*/*) script_path=$0 ;;
*) script_path=$(command -v "$0") ;;
esac

bindir=$(CDPATH= cd -P -- "${script_path%/*}" && pwd)
root=${bindir%/*}
engine=${0##*/}

export GDFONTPATH="$root/share/fonts"

exec "$bindir/_dot" "-K$engine" "$@"
