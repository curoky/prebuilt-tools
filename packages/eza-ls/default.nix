# eza-ls
#
# An `ls`-compatible front-end backed by `eza`. The public command stays `ls`,
# but it runs the bundled `eza` binary with enhanced defaults (icons, git,
# grouped dirs, human sizes) so the output is eza's richer rendering.
#
# It ships as its own package (deploy dir `eza-ls`) exposing a single `ls`
# binary; the upstream `eza` manifest package is left untouched.
#
#   $store/eza-ls/
#     bin/
#       ls    (bash wrapper: translates ls-style flags, then exec's eza)
#       eza   (the real static eza binary, from pkgsStatic.eza)
#
# The wrapper (./ls-wrapper.sh) adapts the eggbean eza gist (same as
# /opt/devspace/tools/eza-wrapper.sh) and extends it to map most common GNU `ls`
# options to eza — short and long forms (e.g. -la, --sort=size, --color=auto,
# --time-style=..., --group-directories-first) — invoking the co-located ./eza.
# Options eza cannot faithfully represent (e.g. -Q/-C/-m/-w, --sort=version)
# transparently fall back to the system /bin/ls instead of erroring out; it also
# falls back when the bundled eza is unusable or stdout is piped.
{
  lib,
  stdenvNoCC,
  eza,
}:

stdenvNoCC.mkDerivation {
  pname = "eza-ls";
  inherit (eza) version;

  dontUnpack = true;

  installPhase = ''
    runHook preInstall

    mkdir -p $out/bin

    # Ship the real eza binary alongside the wrapper so `ls` is self-contained
    # and does not depend on an `eza` on the host PATH.
    cp ${lib.getExe eza} $out/bin/eza

    cp ${./ls-wrapper.sh} $out/bin/ls
    chmod +x $out/bin/ls

    runHook postInstall
  '';

  meta = {
    description = "ls-compatible front-end backed by eza";
    homepage = "https://github.com/eza-community/eza";
    license = lib.licenses.mit;
    platforms = [
      "x86_64-linux"
      "aarch64-darwin"
    ];
    mainProgram = "ls";
  };
}
