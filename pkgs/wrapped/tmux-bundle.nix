{ lib, stdenv, fetchFromGitHub, tmux}:

stdenv.mkDerivation rec {
  pname = "tmux-bundle";
  version = "1.0.0";

  src = fetchFromGitHub {
    owner = "gpakosz/";
    repo = ".tmux";
    rev = "4cb811769abe8a2398c7c68c8e9f00e87bad4035";
    sha256 = "sha256-e7Ymv3DD7FY2i7ij9woZ6o/edJGbEfm2K8wrD2H43Yk=";
  };

  installPhase = ''
    cp -r ${tmux.out} $out/
    mkdir -p $out/share/
    chmod +w $out/share/
    cp ${src}/.tmux.conf $out/share/tmux.conf
  '';

  meta = with lib; {
    description = "The .tmux.conf file from gpakosz/.tmux repository on GitHub";
  };
}
