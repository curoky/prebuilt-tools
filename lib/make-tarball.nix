# make-tarball.nix
#
# Pack a standalone payload into the published artifact tarball. This mirrors
# the archiving the build workflows used to do with rsync + tar:
#
#   - materialize the standalone output under a top-level directory named after
#     the package (the `bm` client strips this first component; see
#     cmd/binman/CLAUDE.md "远端协议");
#   - preserve store-internal relative symlinks as symlinks (normalize.sh
#     already inlined every external/dangling link, so the tree is
#     self-contained); the `bm` client re-creates these as symlinks on extract;
#   - produce a single deterministic `<name>.<arch>.tar.gz` gzip layer.
#
# Building this in Nix removes the platform-specific rsync/tar host tooling from
# the workflows and makes the archive reproducible and cacheable. The archive
# layout (top dir == package name, internal symlinks kept) is a cross build/
# client protocol; changing it requires updating cmd/binman as well.
{
  pkgs,
}:
name: standalone:
let
  arch = if pkgs.stdenv.hostPlatform.isDarwin then "darwin-arm64" else "linux-x86_64";
in
pkgs.runCommand "${name}-${arch}.tar.gz"
  {
    nativeBuildInputs = [
      pkgs.buildPackages.gnutar
      pkgs.buildPackages.gzip
    ];
  }
  ''
    mkdir -p build/${name}
    cp -a ${standalone}/. build/${name}/
    chmod -R u+w build/${name}

    # Deterministic archive: fixed mtime/owner and sorted entries so the layer
    # digest only changes when contents change.
    tar \
      --sort=name \
      --mtime='@0' \
      --owner=0 --group=0 --numeric-owner \
      -czf "$out" \
      -C build "${name}"
  ''
