# Linux local packages: shared set (common.nix) merged with the current
# architecture's set (x86_64.nix / aarch64.nix). Architecture is selected from
# the static (target) platform so cross builds resolve correctly.
{
  pkgs,
  pkgsStatic,
}:
let
  common = import ./common.nix { inherit pkgs pkgsStatic; };
  arch =
    if pkgsStatic.stdenv.hostPlatform.isAarch64 then
      import ./aarch64.nix { inherit pkgs pkgsStatic; }
    else
      import ./x86_64.nix { inherit pkgs pkgsStatic; };
in
common // arch
