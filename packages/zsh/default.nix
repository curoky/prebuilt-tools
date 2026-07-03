{
  lib,
  stdenv,
  fetchurl,
  zsh,
  writeText,
}:

let
  wrapperScript = writeText "wrapper.sh" ''
    #!/usr/bin/env bash

    script_path="$(readlink -f "$0")"
    root=$(cd "$(dirname "$script_path")" && pwd)/..
    export FPATH=$FPATH:$root/share/zsh/${zsh.version}/functions

    exec -a "$0" "$root/bin/_zsh" "$@"
  '';
in

zsh.overrideAttrs (oldAttrs: rec {
  # Work around a GCC internal compiler error (ICE) in the object-size pass
  # (compute_builtin_object_size) triggered while compiling Src/sort.o with
  # _FORTIFY_SOURCE enabled for the static/musl build.
  hardeningDisable = (oldAttrs.hardeningDisable or [ ]) ++ [ "fortify" ];

  # nixpkgs sets `--enable-zshenv=$out/etc/zshenv`, pinning the global zshenv
  # into the read-only Nix store. Drop that flag and point it at /etc/zsh/zshenv
  # so a system-wide /etc/zsh/zshenv is honored.
  configureFlags =
    (lib.filter (f: !(lib.hasPrefix "--enable-zshenv=" f)) (oldAttrs.configureFlags or [ ]))
    ++ [ "--enable-zshenv=/etc/zsh/zshenv" ];

  patchPhase = oldAttrs.patchPhase or "" + ''
    echo "link=either" >> Src/Modules/system.mdd
    echo "link=either" >> Src/Modules/regex.mdd
    echo "link=either" >> Src/Modules/mathfunc.mdd
    substituteInPlace Src/Modules/termcap.c \
      --replace '#ifndef HAVE_BOOLCODES' '#if 0'
    # --replace 'char *boolcodes[]' 'const char * const boolcodes[]'
  '';

  outputs = [
    "out"
    "man"
  ];

  # nativeBuildInputs = builtins.filter (dep: dep.pname or "" != "yodl") oldAttrs.nativeBuildInputs;

  # postInstall = (oldAttrs.postInstall or "") + ''
  postInstall = ''
    mv $out/bin/zsh $out/bin/_zsh
    cp ${wrapperScript} $out/bin/zsh
    chmod +x $out/bin/zsh
  '';
})
