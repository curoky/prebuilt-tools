# x86_64-linux-only local packages.
{
  pkgs,
  pkgsStatic,
}:
let
  mkPython = pkgsStatic.callPackage ../../python { };
in
{
  # python311's musl-static cross build currently fails on aarch64-linux, so it
  # is only built here on x86_64-linux (see aarch64.nix and the docs regression
  # table).
  python311 = mkPython {
    python = pkgsStatic.python311;
    setupLocal = ../../python/311/Setup.local;
  };
}
