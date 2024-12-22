{ lib, stdenv, python3, fetchurl, fetchFromGitHub, cmake, fetchzip, opencc, unzip, python3Packages, pkgs}:

stdenv.mkDerivation rec {
  pname = "tmux-conf";
  version = "1.0.0";

  src = fetchFromGitHub {
    owner = "gpakosz/";
    repo = ".tmux";
    rev = "4cb811769abe8a2398c7c68c8e9f00e87bad4035";
    sha256 = "sha256-e7Ymv3DD7FY2i7ij9woZ6o/edJGbEfm2K8wrD2H43Yk=";
  };

  installPhase = ''
    mkdir -p $out/share/
    cp ${src}/.tmux.conf $out/share/tmux.conf
  '';

  meta = with lib; {
    description = "The .tmux.conf file from gpakosz/.tmux repository on GitHub";
  };
}
