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
  silver_searcher_static = old.pkgsStatic.silver-searcher.overrideAttrs (oldAttrs: rec {
    NIX_LDFLAGS = "";
  });
  diffutils_static = pkgs.pkgsStatic.diffutils.overrideAttrs (oldAttrs: rec {
    doCheck = false;
  });
  protobuf3_20_static = pkgs.pkgsStatic.protobuf3_20.overrideAttrs (oldAttrs: rec {
    postInstall = ''
      mv $out/bin/protoc $out/bin/protoc-${oldAttrs.version}
    '';
  });
  protobuf_3_8_0_static = old.pkgsStatic.protobuf3_20.overrideAttrs (oldAttrs: rec {
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
  protobuf_3_9_2_static = old.pkgsStatic.protobuf3_20.overrideAttrs (oldAttrs: rec {
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
  cmake_static = pkgs.pkgsStatic.cmakeMinimal.overrideAttrs (oldAttrs: rec {
    doCheck = false;
  });
  ruff_static = pkgs.pkgsStatic.ruff.overrideAttrs (oldAttrs: rec {
    doCheck = false;
    doInstallCheck = false;
  });
  git_static = pkgs.pkgsStatic.git.overrideAttrs (oldAttrs: rec {
    # buildInputs = builtins.filter (dep: dep != pkgs.curl)(oldAttrs.buildInputs) ++ [
    #buildInputs = oldAttrs.buildInputs ++ [
    #  (pkgs.pkgsStatic.curl.override {
    #    idnSupport = false;
    #    pslSupport = false;
    #  })
    #];
    withSsh = true;
    svnSupport = true;
    LDFLAGS = "-L${pkgs.pkgsStatic.curl.out}/lib -L${pkgs.pkgsStatic.openssl.out}/lib -L${pkgs.pkgsStatic.libunistring.out}/lib -L${pkgs.pkgsStatic.libpsl.out}/lib -L${pkgs.pkgsStatic.libssh2.out}/lib -L${pkgs.pkgsStatic.libidn2.out}/lib -L${pkgs.pkgsStatic.zlib.out}/lib -L${pkgs.pkgsStatic.zstd.out}/lib -L${pkgs.pkgsStatic.nghttp2.lib}/lib";
    LIBS = "-lzstd -lssl -lcrypto -lidn2 -lpsl -lnghttp2 -lssh2 -lunistring -lz";
    #
    #configureFlags = oldAttrs.configureFlags ++ [
    #  "ac_cv_prog_CURL_CONFIG="
    #];
    #configureFlags = oldAttrs.configureFlags ++ [
    #  "LDFLAGS='-L${pkgs.pkgsStatic.curl.out}/lib -L${pkgs.pkgsStatic.openssl.out}/lib'"
    #];

    preInstallCheck = oldAttrs.preInstallCheck + ''
      disable_test t0211-trace2-perf
      disable_test t2082-parallel-checkout-attributes
      disable_test t1517-outside-repo
    '';
  });
  dstat_static = pkgs.pkgsStatic.dstat.overrideAttrs (oldAttrs: rec {
    doCheck = false;
    pytestCheckPhase = "";
    preDistPhases = [];
    installCheckPhase = false;
  });

in
{
  inherit silver_searcher_static;
  inherit diffutils_static;
  inherit protobuf3_20_static;
  inherit dstat_static;
  inherit git_static;
  inherit protobuf_3_8_0_static;
  inherit protobuf_3_9_2_static;
  inherit ruff_static;

  rsync_static = pkgs.pkgsStatic.rsync.override {
    enableXXHash = false;
  };
  coreutils_static = pkgs.pkgsStatic.coreutils.override {
    singleBinary = false;
  };
  gnupg_minimal_static = pkgs.pkgsStatic.gnupg.override {
    enableMinimal = true;
  };
  glibcLocales = pkgs.glibcLocales.override {
    allLocales = false;
  };

  wget = old.pkgsStatic.callPackage ./wget.nix { };
  zsh = old.pkgsStatic.callPackage ./zsh.nix { };

  cacert = pkgs.pkgsStatic.callPackage ./cacert.nix { };
  dool = pkgs.pkgsStatic.callPackage ./dool.nix { };
  netron = pkgs.pkgsStatic.callPackage ./netron.nix { };
  git-filter-repo = pkgs.pkgsStatic.callPackage ./git-filter-repo.nix { };
  curl = pkgs.pkgsStatic.callPackage ./curl.nix { };
  file = pkgs.pkgsStatic.callPackage ./file.nix { };
  makeself = pkgs.pkgsStatic.callPackage ./makeself.nix { };
  nsight-systems = pkgs.pkgsStatic.callPackage ./nsight-systems.nix { };
  openssh_gssapi = pkgs.pkgsStatic.callPackage ./openssh_gssapi.nix { };
  python311 = pkgs.pkgsStatic.callPackage ./python311.nix { };
  rime-bundle = pkgs.pkgsStatic.callPackage ./rime-bundle.nix { };
  tmux-bundle = pkgs.pkgsStatic.callPackage ./tmux-bundle.nix { };
  vim = pkgs.pkgsStatic.callPackage ./vim.nix { };
  vim-bundle = pkgs.callPackage ./vim-bundle.nix { };
  zsh-bundle = pkgs.pkgsStatic.callPackage ./zsh-bundle.nix { };
}
