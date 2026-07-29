# Stable entry point for locally-defined packages (patched / wrapped / pinned).
# Returns { common, linux, darwin }; the flake merges common + the current
# platform's set. Packages are wired in explicitly via callPackage in the
# per-platform files below — the directory is never auto-scanned.
{
  pkgs,
  pkgsStatic,
  pkgs2605Static,
}:
{
  common = import ./local/common.nix {
    inherit pkgs pkgsStatic;
  };
  linux = import ./local/linux.nix {
    inherit pkgs pkgsStatic pkgs2605Static;
  };
  darwin = import ./local/darwin.nix {
    inherit pkgs pkgsStatic;
  };
}
