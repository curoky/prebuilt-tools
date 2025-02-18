{ lib, stdenv, fetchurl, curl }:

stdenv.mkDerivation rec {
  pname = "curl";
  version = "1.0.0";

  src = fetchurl {
    url = "https://curl.se/ca/cacert-2024-09-24.pem";
    sha256 = "sha256-GJ089tEDGF+6BtdsGvkVJjxtQiJUgaF1noU7M6yFdUA=";
  };

  unpackPhase = ":";

  buildInputs = [ curl ];

  installPhase = ''
    mkdir -p $out
    cp -r ${curl.bin}/bin $out/bin
    cp -r ${curl.dev}/share $out/share

    mkdir -p $out/etc/ssl/certs/
    cp ${src} $out/etc/ssl/certs/ca-certificates.crt
  '';
}
