# opencommit (on static node)
#
# opencommit running on our fully-static (musl) `nodejs-slim26` package, instead
# of being `nix bundle`'d into a self-extracting executable. Same runtime
# approach as packages/pnpm: reuse the upstream nixpkgs JS distribution and ship
# a relative-path wrapper that invokes the sibling static node explicitly, so
# the static node travels with the deployed tool instead of depending on a node
# on the host PATH after artifact assembly rewrites the upstream shebang.
#
# Unlike pnpm/prettier, the interpreter is NOT overridden at build time: this is
# an npm-based buildNpmPackage tool whose build needs `npm`, which nodejs-slim
# lacks. So it is built with the regular node and only switches to the sibling
# static node at runtime via the wrapper below.
#
# Upstream nixpkgs ships opencommit as:
#   $out/bin/{opencommit,oco}                  (wrappers invoking node)
#   $out/lib/node_modules/opencommit/...        (the JS + node_modules)
#
# Deploy layout:
#   $store/
#     nodejs-slim26/bin/node      (separate package; static musl ELF)
#     opencommit/
#       bin/{opencommit,oco}      (wrappers -> sibling node + out/cli.cjs)
#       libexec/opencommit/...    (JS, from the nixpkgs opencommit)
#
# Verification:
#   $out/bin/opencommit --version   # (with sibling node present)
{
  lib,
  stdenvNoCC,
  writeText,
  opencommit,
  nodejs-slim26,
}:

let
  wrapper = writeText "opencommit-wrapper.sh" ''
    #!/usr/bin/env bash
    script_path="$(readlink -f "$0")"
    root="$(cd "$(dirname "$script_path")/.." && pwd)"
    store="$(cd "$root/.." && pwd)"
    exec "$store/nodejs-slim26/bin/node" "$root/libexec/opencommit/out/cli.cjs" "$@"
  '';
in
stdenvNoCC.mkDerivation {
  pname = "opencommit";
  inherit (opencommit) version;

  dontUnpack = true;

  installPhase = ''
    runHook preInstall

    # Reuse the upstream nixpkgs opencommit's JS distribution.
    mkdir -p $out/libexec
    cp -R ${opencommit}/lib/node_modules/opencommit $out/libexec/opencommit

    # Replace the upstream bin wrappers with relative-path wrappers that invoke
    # the sibling static node explicitly.
    mkdir -p $out/bin
    cp ${wrapper} $out/bin/opencommit
    cp ${wrapper} $out/bin/oco
    chmod +x $out/bin/opencommit $out/bin/oco

    runHook postInstall
  '';

  # Even though opencommit is *built* with the regular node (it needs npm), make
  # sure the shipped JS actually runs on the static `nodejs-slim26` we deploy
  # alongside it — i.e. the exact command the runtime wrapper issues.
  doInstallCheck = true;
  installCheckPhase = ''
    runHook preInstallCheck
    ${nodejs-slim26}/bin/node $out/libexec/opencommit/out/cli.cjs --version
    runHook postInstallCheck
  '';

  meta = {
    description = "opencommit running on the sibling standalone node package";
    homepage = "https://github.com/di-sukharev/opencommit";
    license = lib.licenses.mit;
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
      "aarch64-darwin"
    ];
    mainProgram = "opencommit";
  };
}
