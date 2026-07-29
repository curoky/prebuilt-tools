# prettier (on static node)
#
# prettier running on our fully-static (musl) `nodejs-slim26` package, instead
# of being `nix bundle`'d into a self-extracting executable. Same approach as
# packages/pnpm: the interpreter is overridden upstream-style in
# packages/local/node-tools.nix (`pkgs.prettier.override { nodejs = nodejs-slim26; }`), so
# the `prettier` argument here is already built against our static node. This
# derivation reuses that prettier's JS distribution and ships a relative-path
# wrapper that invokes the sibling static node explicitly, so the static node
# travels with the deployed tool instead of depending on a node on the host
# PATH after the standalone normalize pass.
#
# Upstream nixpkgs ships prettier as:
#   $out/bin/prettier                                  (wrapper invoking node)
#   $out/lib/node_modules/prettier/...                 (the JS, self-contained)
#
# Deploy layout:
#   $store/
#     nodejs-slim26/bin/node   (separate package; static musl ELF)
#     prettier/
#       bin/prettier           (wrapper: exec $store/nodejs-slim26/bin/node \
#                                          $root/libexec/prettier/bin/prettier.cjs "$@")
#       libexec/prettier/...   (prettier JS, from the nixpkgs prettier)
#
# Verification:
#   $out/bin/prettier --version   # => prettier version (with sibling node present)
{
  lib,
  stdenvNoCC,
  writeText,
  prettier,
}:

let
  wrapper = writeText "prettier-wrapper.sh" ''
    #!/usr/bin/env bash
    script_path="$(readlink -f "$0")"
    root="$(cd "$(dirname "$script_path")/.." && pwd)"
    store="$(cd "$root/.." && pwd)"
    exec "$store/nodejs-slim26/bin/node" "$root/libexec/prettier/bin/prettier.cjs" "$@"
  '';
in
stdenvNoCC.mkDerivation {
  pname = "prettier";
  inherit (prettier) version;

  dontUnpack = true;

  installPhase = ''
    runHook preInstall

    # Reuse the upstream nixpkgs prettier's JS distribution.
    mkdir -p $out/libexec
    cp -R ${prettier}/lib/node_modules/prettier $out/libexec/prettier

    # Replace the upstream bin wrapper with a relative-path wrapper that invokes
    # the sibling static node explicitly.
    mkdir -p $out/bin
    cp ${wrapper} $out/bin/prettier
    chmod +x $out/bin/prettier

    runHook postInstall
  '';

  meta = {
    description = "prettier running on the sibling standalone node package";
    homepage = "https://prettier.io/";
    license = lib.licenses.mit;
    platforms = [
      "x86_64-linux"
      "aarch64-darwin"
    ];
    mainProgram = "prettier";
  };
}
