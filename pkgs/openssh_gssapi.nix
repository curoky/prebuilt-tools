{ lib, stdenv, fetchurl, openssh_gssapi, writeText}:

let
  wrapperScript = writeText "wrapper.sh" ''
    #!/usr/bin/env bash

    script_path="$(readlink -f "$0")"
    root=$(cd "$(dirname "$script_path")" && pwd)/..

    exec -a "$0" "$root/bin/_scp" -S $root/bin/ssh "$@"
  '';
in

openssh_gssapi.overrideAttrs (oldAttrs: {
  postInstall = (oldAttrs.postInstall or "") + ''
    mv $out/bin/scp $out/bin/_scp
    cp ${wrapperScript} $out/bin/scp
  '';
})
