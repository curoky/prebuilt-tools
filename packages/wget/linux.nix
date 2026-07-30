{
  lib,
  stdenv,
  wget,
  cacert,
  writeText,
}:

let
  wrapperScript = writeText "wrapper.sh" ''
    #!/usr/bin/env bash

    script_path="$(readlink -f "$0")"
    root=$(cd "$(dirname "$script_path")" && pwd)/..

    exec -a "$0" "$root/bin/_wget" --ca-certificate $root/etc/ssl/certs/ca-bundle.crt "$@"
  '';
  wget_static = wget.overrideAttrs (oldAttrs: rec {
    doCheck = false;
  });
in

stdenv.mkDerivation rec {
  pname = "wget";
  version = "1.0.0";

  dontUnpack = true;

  nativeBuildInputs = [ wget ];

  installPhase = ''
    mkdir -p $out
    cp -r ${wget_static}/bin $out/bin
    cp -r ${wget_static}/etc $out/etc
    cp -r ${wget_static}/share $out/share

    chmod +w $out/etc/
    mkdir -p $out/etc/ssl/certs/
    cp ${cacert}/etc/ssl/certs/ca-bundle.crt $out/etc/ssl/certs/ca-bundle.crt

    chmod +w $out/bin/
    mv $out/bin/wget $out/bin/_wget
    cp ${wrapperScript} $out/bin/wget
    chmod +x $out/bin/wget
  '';
}
