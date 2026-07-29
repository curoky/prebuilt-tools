# make-standalone.nix
#
# Wrap a derivation so its output becomes a self-contained, "standalone"
# payload suitable for shipping to minimal/foreign environments:
#   - copy the output into a fresh, writable $out
#   - run scripts/normalize.sh over the tree to strip binaries, rewrite
#     Nix store references, drop docs/manpages, and inline external symlinks
#
# This is purely a post-processing/normalization step; it does not change how
# the underlying package is built (static vs dynamic). See CLAUDE.md.
{
  pkgs,
  normalizeScript,
}:
name: drv:
let
  allowDynamicElf = pkgs.lib.elem name [
    "music-decrypto"
    "nsight-systems"
  ];
in
pkgs.runCommand "${name}-standalone"
  {
    nativeBuildInputs = [
      pkgs.buildPackages.binutils
      pkgs.buildPackages.file
      pkgs.buildPackages.nukeReferences
    ]
    # The final portability check in normalize.sh uses these tools to enforce
    # static Linux ELF files, portable Darwin load commands, and no /nix
    # runtime references.
    ++ pkgs.lib.optional pkgs.stdenv.hostPlatform.isDarwin pkgs.darwin.cctools
    ++ pkgs.lib.optional (!pkgs.stdenv.hostPlatform.isDarwin) pkgs.buildPackages.patchelf;
    builderScript = normalizeScript;
  }
  ''
    mkdir -p $out
    cp -pRP ${drv}/* $out/ 2>/dev/null || true
    chmod -R u+w $out

    bash $builderScript $out ${pkgs.lib.escapeShellArg name} ${if allowDynamicElf then "1" else "0"} ${if pkgs.stdenv.hostPlatform.isDarwin then "macho" else "elf"}
  ''
