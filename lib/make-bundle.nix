# make-bundle.nix
#
# Bundle a derivation into a single self-extracting executable for tools that
# genuinely cannot be statically compiled (e.g. Node.js based tools such as
# prettier / pnpm). This is the "nix bundle" last-resort strategy from
# CLAUDE.md.
#
# Implementation: matthewbauer/nix-bundle. `nix-bootstrap-nix` produces an
# `arx` archive (the whole runtime closure, bzip2-compressed) prefixed with a
# `nix-user-chroot` launcher that, at runtime, mounts the embedded closure at
# `./nix` in a user namespace and execs the target program. This works on
# machines without a Nix store, but relies on user namespaces and is therefore
# Linux only (`nix-user-chroot` has `meta.platforms = linux`).
#
# Unlike `make-standalone.nix`, the bundle output must NOT go through
# `normalize.sh`: it is already a single self-contained file, and stripping /
# nuke-refs / shebang rewriting would corrupt the self-extracting archive.
#
# The bundled single file is placed at `$out/bin/<name>` so the result keeps
# the same `$out/bin/<name>` shape as every other package (CI tar.gz + oras).
{
  pkgs,
  nix-bundle,
}:
let
  bundle = import nix-bundle { nixpkgs = pkgs; };
in
name: drv:
let
  # Program to run inside the bundle, relative to the target derivation.
  exe = drv.meta.mainProgram or name;
  bundled = bundle.nix-bootstrap-nix {
    target = drv;
    run = "/bin/${exe}";
  };
in
pkgs.runCommand "${name}-bundle" { } ''
  mkdir -p $out/bin
  cp ${bundled} $out/bin/${name}
  chmod +x $out/bin/${name}
''
