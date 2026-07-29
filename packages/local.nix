# local.nix
#
# Locally-defined packages: pinned versions, patched builds, wrapped bundles,
# and platform-specific (Linux-only) tooling. These are the packages where the
# repo does manual work (patch + bundle) rather than just selecting an upstream
# nixpkgs derivation via the manifest.
#
# Returns an attrset:
#   { common = { ... }; linux = { ... }; darwin = { ... }; }
# The caller merges the relevant platform set into the final package set.
{
  lib,
  pkgs,
  pkgsStatic,
  pkgs2605Static,
}:
{
  # Cross-platform local packages.
  common = {
    # unclassified (resource bundles, no compilation)
    cacert = pkgsStatic.callPackage ./cacert { };
    rime-plugins = pkgsStatic.callPackage ./rime-plugins { };
    tmux-plugins = pkgsStatic.callPackage ./tmux-plugins { };
    vim-plugins = pkgs.callPackage ./vim-plugins { };
    zsh-plugins = pkgsStatic.callPackage ./zsh-plugins { };

    # C / autotools (stdenv)
    autoconf = pkgsStatic.callPackage ./autoconf { };
    automake = pkgsStatic.callPackage ./automake { };
    coreutils = pkgsStatic.coreutils.override {
      singleBinary = false;
    };
    curl = pkgsStatic.callPackage ./curl { };
    diffutils = pkgsStatic.callPackage ./diffutils { };
    # ls-compatible front-end backed by eza (ships its own `ls` binary + eza).
    eza-ls = pkgsStatic.callPackage ./eza-ls { };
    file = pkgsStatic.callPackage ./file { };
    gettext = pkgsStatic.callPackage ./gettext { };
    gnupg = pkgsStatic.gnupg.override {
      enableMinimal = true;
      guiSupport = false;
    };
    libtool = pkgsStatic.callPackage ./libtool { };
    makeself = pkgsStatic.callPackage ./makeself { };
    p7zip = pkgsStatic.callPackage ./p7zip { };
    protobuf_3_8_0 = pkgsStatic.callPackage ./protobuf/3_8_0 { };
    protobuf_3_9_2 = pkgsStatic.callPackage ./protobuf/3_9_2 { };
    rsync = pkgsStatic.callPackage ./rsync { };
    vim = pkgsStatic.callPackage ./vim { };
    zsh = pkgsStatic.callPackage ./zsh { };

    # Python (sibling-wrapper against the static python314; see docs/package-strategies/python.md)
    git-filter-repo = pkgs.callPackage ./git-filter-repo { };
    netron = pkgs.callPackage ./netron { };

    # Perl (sibling-wrapper against the static perl; see docs/package-strategies/perl.md)
    cloc = pkgs.callPackage ./cloc { };
    parallel = pkgs.callPackage ./parallel { };

    # .NET (glibc-dynamic AOT exception; see docs/package-strategies/special-cases.md)
    music-decrypto = pkgs.callPackage ./music-decrypto { };
  };

  # Linux-only local packages (patched tooling, container stack, multiple
  # clang-tools versions, static Python variants, extra wrapped tools).
  linux = rec {
    # unclassified (repackaged prebuilt native binary; glibc-dynamic prebuilt
    # exception, see docs/package-strategies/special-cases.md)
    nsight-systems = pkgsStatic.callPackage ./nsight-systems { };

    # C / autotools (stdenv)
    cmake = pkgsStatic.callPackage ./cmake/default { };
    cmake_3_27_9 = pkgsStatic.callPackage ./cmake/3_27_9 { };
    cmake_4_1_2 = pkgsStatic.callPackage ./cmake/4_1_2 { };
    git = pkgsStatic.callPackage ./git { };
    # gnutar: linker workaround for a duplicate `*xattrat` symbol collision
    # against static libacl; see docs/package-strategies/special-cases.md.
    gnutar = pkgsStatic.callPackage ./gnutar { };
    openssh_gssapi = pkgsStatic.callPackage ./openssh_gssapi { };
    # postgresql: psql client + static libpq only (gccAsClang workaround); see
    # docs/package-strategies/special-cases.md.
    postgresql = pkgsStatic.callPackage ./postgresql { };
    wget = pkgsStatic.callPackage ./wget/linux.nix { };

    # Rust (see docs/package-strategies/rust.md)
    miniserve = pkgsStatic.callPackage ./miniserve { };

    # Perl (static perl interpreter, sibling-wrapper base; see
    # docs/package-strategies/perl.md)
    perl = pkgsStatic.callPackage ./perl/linux.nix { };
    # exiftool: ships only the pure-Perl pieces because the sibling static perl
    # (-Uusedl) has the XS compression modules compiled in; see
    # docs/package-strategies/perl.md.
    exiftool = pkgs.callPackage ./exiftool/linux.nix { };

    # LLVM / clang tooling (only clang-format extracted; see
    # docs/package-strategies/c-autotools.md)
    clang-tools-18 = pkgsStatic.callPackage ./clang-tools/18 { };
    clang-tools-19 = pkgsStatic.callPackage ./clang-tools/19 { };
    clang-tools-20 = pkgsStatic.callPackage ./clang-tools/20 { };
    clang-tools-21 = pkgsStatic.callPackage ./clang-tools/21 { };
    clang-tools-22 = pkgsStatic.callPackage ./clang-tools/22 { };

    # Python (static interpreters + sibling-wrapper tools; see
    # docs/package-strategies/python.md)
    python311 = pkgsStatic.callPackage ./python/311 { };
    python312 = pkgsStatic.callPackage ./python/312 { };
    python313 = pkgsStatic.callPackage ./python/313 { };
    python314 = pkgsStatic.callPackage ./python/314 { };
    python315 = pkgsStatic.callPackage ./python/315 { };
    dool = pkgs.callPackage ./dool { };

    # s6 stack (baked /nix path removal; see docs/package-strategies/c-autotools.md)
    execline = pkgsStatic.callPackage ./execline { };
    s6 = pkgsStatic.callPackage ./s6 {
      inherit execline;
    };
    s6-linux-init = pkgsStatic.callPackage ./s6-linux-init {
      inherit s6 execline;
    };
    s6-rc = pkgsStatic.callPackage ./s6-rc {
      inherit s6 execline;
    };

    # podman / container stack (podman is Go, see docs/package-strategies/go.md;
    # crun/conmon/catatonit/gpgme are C, see docs/package-strategies/c-autotools.md)
    catatonit = pkgsStatic.callPackage ./catatonit { };
    conmon = pkgsStatic.callPackage ./conmon { };
    crun = pkgsStatic.callPackage ./crun { };
    gpgme = pkgsStatic.callPackage ./gpgme { };
    podman = pkgsStatic.callPackage ./podman {
      inherit
        catatonit
        crun
        conmon
        gpgme
        ;
    };

    # Node.js stack: standalone static Node.js runtimes plus Node CLI tools that
    # run on them via a sibling relative-path wrapper (invoking
    # $store/nodejs-slim26/bin/node). pnpm/prettier build against the static node;
    # markdownlint-cli2/opencommit build with the regular node (they need npm) and
    # switch to the static node at runtime. See docs/package-strategies/nodejs.md.
    # nodejs-slim24 is retained as a standalone runtime; the CLI tools target
    # nodejs-slim26.
    nodejs-slim24 = pkgsStatic.callPackage ./nodejs/24 {
      inherit (pkgs) python3;
    };
    # node 26 links temporal_capi (a Rust dep); on Linux the `pkgsStatic` here is
    # the musl64 *cross* set (see flake.nix / mkEnv) which reuses the cached glibc
    # rustc/LLVM via rust's `fastCross` path, so it doesn't rebuild the toolchain.
    # The node output is still a fully-static musl binary.
    nodejs-slim26 = pkgsStatic.callPackage ./nodejs/26/linux.nix {
      inherit (pkgs) python3;
    };
    pnpm = pkgsStatic.callPackage ./pnpm {
      pnpm = pkgs.pnpm.override { nodejs-slim = nodejs-slim26; };
    };
    prettier = pkgsStatic.callPackage ./prettier {
      prettier = pkgs.prettier.override { nodejs = nodejs-slim26; };
    };
    markdownlint-cli2 = pkgsStatic.callPackage ./markdownlint-cli2 {
      inherit nodejs-slim26;
      inherit (pkgs) markdownlint-cli2;
    };
    opencommit = pkgsStatic.callPackage ./opencommit {
      inherit nodejs-slim26;
      inherit (pkgs) opencommit;
    };

    # Rust (26.05 pinned static env; see docs/package-strategies/rust.md)
    zellij = pkgs2605Static.callPackage ./zellij { };

    # C (native pkgs, non-static)
    glibcLocales = pkgs.glibcLocales.override {
      allLocales = false;
    };
  };

  # Darwin-only local packages.
  darwin = rec {
    # C (partial-static via pkgsStatic; only system libs stay dynamic on macOS)
    # macOS ffmpeg (headless): feature reduction over pkgsStatic.ffmpeg-headless;
    # see docs/package-strategies/special-cases.md.
    ffmpeg = pkgsStatic.callPackage ./ffmpeg/darwin.nix { };
    # macOS krb5: fully-static pkgsStatic with two darwin static-link defects
    # patched; see docs/package-strategies/special-cases.md. On Linux krb5 comes
    # straight from the manifest.
    krb5 = pkgsStatic.callPackage ./krb5/darwin.nix { };
    # macOS wget: fully-static pkgsStatic.wget with only its build-time perl
    # overridden to native (darwin static perl fails to build); see
    # docs/package-strategies/special-cases.md.
    wget = pkgsStatic.callPackage ./wget/darwin-static.nix {
      inherit (pkgs) perlPackages;
    };
    # Alternative (kept, not active): build native pkgs.wget and swap each
    # non-system dep for its pkgsStatic archive (see ./wget/darwin.nix).
    # wget = pkgs.callPackage ./wget/darwin.nix {
    #   inherit pkgsStatic;
    # };

    # Perl (native perl; darwin static perl fails to build; see
    # docs/package-strategies/perl.md)
    perl = pkgs.callPackage ./perl/darwin.nix {
      libxcryptStatic = pkgsStatic.libxcrypt;
    };
    # macOS exiftool: darwin sibling perl can dlopen XS, so compression modules
    # ship as .bundle with statically-linked compression libs; see
    # docs/package-strategies/perl.md.
    exiftool = pkgs.callPackage ./exiftool/darwin.nix {
      inherit pkgsStatic;
    };

    # Node.js stack (macOS partial-static counterpart of the Linux runtime + CLI
    # tools; see docs/package-strategies/nodejs.md)
    nodejs-slim26 = pkgsStatic.callPackage ./nodejs/26/darwin.nix {
      inherit (pkgs) python3 cctools;
    };
    pnpm = pkgsStatic.callPackage ./pnpm {
      pnpm = pkgs.pnpm.override { nodejs-slim = nodejs-slim26; };
    };
    prettier = pkgsStatic.callPackage ./prettier {
      prettier = pkgs.prettier.override { nodejs = nodejs-slim26; };
    };
    markdownlint-cli2 = pkgsStatic.callPackage ./markdownlint-cli2 {
      inherit nodejs-slim26;
      inherit (pkgs) markdownlint-cli2;
    };
    opencommit = pkgsStatic.callPackage ./opencommit {
      inherit nodejs-slim26;
      inherit (pkgs) opencommit;
    };
  };
}
