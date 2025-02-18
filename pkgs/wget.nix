{ lib, stdenv, fetchurl, wget }:

stdenv.mkDerivation rec {
  pname = "wget";
  version = "1.0.0";

  src = fetchurl {
    url = "https://curl.se/ca/cacert-2024-09-24.pem";
    sha256 = "sha256-GJ089tEDGF+6BtdsGvkVJjxtQiJUgaF1noU7M6yFdUA=";
  };

  unpackPhase = ":";

  buildInputs = [ wget ];

  installPhase = ''
    mkdir -p $out
    cp -r ${wget}/bin $out/bin
    cp -r ${wget}/etc $out/etc
    cp -r ${wget}/share $out/share

    chmod +w $out/etc/
    mkdir -p $out/etc/ssl/certs/
    cp ${src} $out/etc/ssl/certs/ca-certificates.crt
  '';
}
