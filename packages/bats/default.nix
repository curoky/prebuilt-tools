# bats (Bash Automated Testing System).
#
# nixpkgs' default `bats` is post-processed by resholve, which bakes absolute
# /nix/store paths into both the external command calls (coreutils, findutils,
# ncurses, ...) and its own $BATS_ROOT / `source` lines. normalize.go strips
# store-path fragments from script bodies, which would leave that resholved
# build with an empty BATS_ROOT and broken `source ""` lines.
#
# `bats.unresholved` is the plain upstream `install.sh` output: 100% Bash that
# derives BATS_ROOT from $BASH_SOURCE at runtime and shells out to PATH-resolved
# commands (env, readlink, ...). Its only /nix reference is the Bash shebang,
# which normalize.go rewrites to `#!/usr/bin/env bash` — leaving a fully
# relocatable, standalone tree.
{ bats }:

bats.unresholved.overrideAttrs (oldAttrs: {
  pname = "bats";
  doInstallCheck = false;
})
