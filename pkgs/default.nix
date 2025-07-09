# This file describes your repository contents.
# It should return a set of nix derivations
# and optionally the special attributes `lib`, `modules` and `overlays`.
# It should NOT import <nixpkgs>. Instead, you should take pkgs as an argument.
# Having pkgs default to <nixpkgs> is fine though, and it lets you use short
# commands such as:
#     nix-build -A mypackage

{ pkgs ? import <nixpkgs> { },
  old ? import <old> { },
  staging ? import <staging> { },
  unstable ? import <unstable> { },
}:

let
  protobuf3_20 = pkgs.pkgsStatic.protobuf3_20.overrideAttrs (oldAttrs: rec {
    postInstall = ''
      mv $out/bin/protoc $out/bin/protoc-${oldAttrs.version}
    '';
  });
  protobuf_3_8_0 = old.pkgsStatic.protobuf3_20.overrideAttrs (oldAttrs: rec {
    src = pkgs.fetchFromGitHub {
      owner = "protocolbuffers";
      repo = "protobuf";
      rev = "v3.8.0";
      sha256 = "sha256-qK4Tb6o0SAN5oKLHocEIIKoGCdVFQMeBONOQaZQAlG4=";
    };
    postInstall = ''
      mv $out/bin/protoc $out/bin/protoc-3.8.0
    '';
  });
  protobuf_3_9_2 = old.pkgsStatic.protobuf3_20.overrideAttrs (oldAttrs: rec {
    src = pkgs.fetchFromGitHub {
      owner = "protocolbuffers";
      repo = "protobuf";
      rev = "v3.9.2";
      sha256 = "sha256-1mLSNLyRspTqoaTFylGCc2JaEQOMR1WAL7ffwJPqHyA=";
    };
    postInstall = ''
      mv $out/bin/protoc $out/bin/protoc-3.9.2
    '';
  });
in
{
  # inherit silver_searcher;
  inherit protobuf3_20;
  inherit protobuf_3_8_0;
  inherit protobuf_3_9_2;

  rsync = pkgs.pkgsStatic.rsync.override {
    enableXXHash = false;
  };
  coreutils = pkgs.pkgsStatic.coreutils.override {
    singleBinary = false;
  };
  glibcLocales = pkgs.glibcLocales.override {
    allLocales = false;
  };

  # patched
  diffutils = pkgs.pkgsStatic.callPackage ./patched/diffutils.nix { };
  cmake = pkgs.pkgsStatic.callPackage ./patched/cmake.nix {};
  zellij = pkgs.pkgsStatic.callPackage ./patched/zellij.nix { };

  # pypkgs
  dool = pkgs.pkgsStatic.callPackage ./pypkgs/dool.nix { };
  netron = pkgs.pkgsStatic.callPackage ./pypkgs/netron.nix { };
  git-filter-repo = pkgs.pkgsStatic.callPackage ./pypkgs/git-filter-repo.nix { };

  # wrapped
  # bat = pkgs.pkgsStatic.callPackage ./wrapped/bat.nix { };
  vim = pkgs.pkgsStatic.callPackage ./wrapped/vim.nix { };
  # vim-bundle = pkgs.pkgsStatic.callPackage ./wrapped/vim-bundle.nix { };
  curl = pkgs.pkgsStatic.callPackage ./wrapped/curl.nix { };
  file = pkgs.pkgsStatic.callPackage ./wrapped/file.nix { };
  makeself = pkgs.pkgsStatic.callPackage ./wrapped/makeself.nix { };
  miniserve = pkgs.pkgsStatic.callPackage ./wrapped/miniserve.nix { };
  openssh_gssapi = pkgs.pkgsStatic.callPackage ./wrapped/openssh_gssapi.nix { };
  # tmux-bundle = pkgs.pkgsStatic.callPackage ./wrapped/tmux-bundle.nix { };
  wget = pkgs.pkgsStatic.callPackage ./wrapped/wget.nix { };
  zsh = pkgs.pkgsStatic.callPackage ./wrapped/zsh.nix { };
  zsh-bundle = pkgs.pkgsStatic.callPackage ./wrapped/zsh-bundle.nix { };
  autoconf = pkgs.pkgsStatic.callPackage ./wrapped/autoconf.nix { };
  automake = pkgs.pkgsStatic.callPackage ./wrapped/automake.nix { };
  libtool = pkgs.pkgsStatic.callPackage ./wrapped/libtool.nix { };

  cacert = pkgs.pkgsStatic.callPackage ./cacert.nix { };
  nsight-systems = pkgs.pkgsStatic.callPackage ./nsight-systems.nix { };
  python311 = pkgs.pkgsStatic.callPackage ./python311.nix { };
  rime-extra = pkgs.pkgsStatic.callPackage ./rime-extra.nix { };
  tmux-extra = pkgs.pkgsStatic.callPackage ./tmux-extra.nix { };
  vim-extra = pkgs.callPackage ./vim-extra.nix { };
  zsh-extra = pkgs.pkgsStatic.callPackage ./zsh-extra.nix { };
}
