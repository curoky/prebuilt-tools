{
  pkgs,
  pkgsStatic,
}:
let
  mkClangTools = pkgsStatic.callPackage ../../clang-tools { };
  mkPython = pkgsStatic.callPackage ../../python { };
  nodejs-slim26 = pkgsStatic.callPackage ../../nodejs/26/linux.nix {
    inherit (pkgs) python3;
  };
  nodeTools = import ../node-tools.nix {
    inherit pkgs pkgsStatic nodejs-slim26;
  };
in
rec {
  # Prebuilt glibc-dynamic exception.
  nsight-systems = pkgsStatic.callPackage ../../nsight-systems { };

  # C / autotools.
  cmake_3_27_9 = pkgsStatic.callPackage ../../cmake/3_27_9 { };
  cmake_4_1_2 = pkgsStatic.callPackage ../../cmake/4_1_2 { };
  diffutils = pkgsStatic.callPackage ../../diffutils { };
  git = pkgsStatic.callPackage ../../git { };
  gnutar = pkgsStatic.callPackage ../../gnutar { };
  openssh_gssapi = pkgsStatic.callPackage ../../openssh_gssapi { };
  poppler = pkgsStatic.callPackage ../../poppler { };
  postgresql = pkgsStatic.callPackage ../../postgresql { };
  wget = pkgsStatic.callPackage ../../wget/linux.nix { };

  # Rust.
  miniserve = pkgsStatic.callPackage ../../miniserve { };
  # zellij builds against unstable but keeps checks disabled (the test target
  # statically links libcurl against libssh2 and fails on unresolved symbols);
  # see docs/package-strategies/rust.md.
  zellij = pkgsStatic.callPackage ../../zellij { };

  # Perl.
  perl = pkgsStatic.callPackage ../../perl/linux.nix { };
  exiftool = pkgs.callPackage ../../exiftool/linux.nix { };

  # LLVM / clang tooling.
  clang-tools-18 = mkClangTools {
    llvmPackages = pkgsStatic.llvmPackages_18;
    version = "18.0.0";
  };
  clang-tools-19 = mkClangTools {
    llvmPackages = pkgsStatic.llvmPackages_19;
    version = "19.0.0";
  };
  clang-tools-20 = mkClangTools {
    llvmPackages = pkgsStatic.llvmPackages_20;
    version = "20.0.0";
  };
  clang-tools-21 = mkClangTools {
    llvmPackages = pkgsStatic.llvmPackages_21;
    version = "21.0.0";
  };
  clang-tools-22 = mkClangTools {
    llvmPackages = pkgsStatic.llvmPackages_22;
    version = "22.0.0";
  };

  # Python. python311 is x86_64-only (its musl-static cross build currently
  # fails on aarch64-linux); see ./x86_64.nix and the docs regression table.
  python312 = mkPython {
    python = pkgsStatic.python312;
    setupLocal = ../../python/312/Setup.local;
  };
  python313 = mkPython {
    python = pkgsStatic.python313;
    setupLocal = ../../python/313/Setup.local;
  };
  python314 = mkPython {
    python = pkgsStatic.python314;
    setupLocal = ../../python/314/Setup.local;
  };
  python315 = mkPython {
    python = pkgsStatic.python315;
    setupLocal = ../../python/315/Setup.local;
    patchMuslStatx = true;
  };
  dool = pkgs.callPackage ../../dool { };

  # s6 stack.
  execline = pkgsStatic.callPackage ../../execline { };
  s6 = pkgsStatic.callPackage ../../s6 {
    inherit execline;
  };
  s6-linux-init = pkgsStatic.callPackage ../../s6-linux-init {
    inherit s6 execline;
  };
  s6-rc = pkgsStatic.callPackage ../../s6-rc {
    inherit s6 execline;
  };

  # Podman / container stack.
  catatonit = pkgsStatic.callPackage ../../catatonit { };
  conmon = pkgsStatic.callPackage ../../conmon { };
  crun = pkgsStatic.callPackage ../../crun { };
  gpgme = pkgsStatic.callPackage ../../gpgme { };
  podman = pkgsStatic.callPackage ../../podman {
    inherit
      catatonit
      crun
      conmon
      gpgme
      ;
  };

  # Node.js runtime and sibling-runtime tools. On Linux `pkgsStatic` is the
  # musl64 cross static set (see flake.nix / mkEnv), so node's Rust deps reuse
  # the cached glibc toolchain; see docs/package-strategies/nodejs.md.
  nodejs-slim24 = pkgsStatic.callPackage ../../nodejs/24 {
    inherit (pkgs) python3;
  };
  inherit nodejs-slim26;
  inherit (nodeTools)
    markdownlint-cli2
    opencommit
    pnpm
    prettier
    ;

  # Native glibc data.
  glibcLocales = pkgs.glibcLocales.override {
    allLocales = false;
  };
}
