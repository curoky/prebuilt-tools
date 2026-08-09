# manifests/default.nix
#
# Single declarative manifest of upstream nixpkgs packages.
#
# Schema (first-level key = package attr name in nixpkgs):
#
#   <pkg> = {
#     # Optional list of systems this package is built for.
#     # Omitted => all systems (see `allSystems` in make-manifest-packages.nix).
#     platforms = [ "x86_64-linux" "aarch64-darwin" ];
#
#     # Package-level shared config, inherited by every platform:
#     version  = "unstable";   # which pinned nixpkgs env (default "unstable")
#     isStatic = true;         # pkgsStatic (true, default) or pkgs (false)
#     output   = [ "out" ];    # derivation outputs to expose (default [ "out" ])
#     alias    = "name";       # rename exported attribute
#
#     # Per-platform overrides. The effective config for a system is
#     # (package-level shared config) // (platform key config), platform wins.
#     "aarch64-darwin" = { version = "24.11"; };
#   };
{
  ## ---- common (all platforms) -------------------------------------------

  # bash/coreutils temporarily skip aarch64-linux: the musl-static cross build
  # currently fails there (see docs regression table). x86_64-linux and darwin
  # stay on the zero-customization manifest build.
  bash = {
    platforms = [
      "x86_64-linux"
      "aarch64-darwin"
    ];
  };
  binutils-unwrapped = {
    alias = "binutils";
  };
  bison = { };
  bzip2 = {
    output = [ "bin" ];
  };
  cacert = { };
  connect = { };
  coreutils = {
    platforms = [
      "x86_64-linux"
      "aarch64-darwin"
    ];
  };
  # diffutils on Linux keeps a local override (packages/diffutils): unstable
  # 3.12's gnulib checkPhase fails 9 multithread/setlocale tests under
  # musl-static. Darwin uses the stock manifest build.
  diffutils = {
    platforms = [ "aarch64-darwin" ];
  };
  findutils = { };
  flac = {
    output = [ "bin" ];
  };
  flex = { };
  gawk = { };
  gdb = {
    version = "25.11";
  };
  getopt = { };
  gettext = { };
  git-extras = { };
  gnugrep = { };
  gnumake = { };
  gnupatch = { };
  gnused = { };
  # Linux gnutar is a local package (packages/gnutar): the fully-static musl
  # build hits a duplicate-symbol link error against static libacl and needs a
  # linker workaround. Darwin has no static libacl collision, so it stays on the
  # stock manifest build.
  gnutar = {
    platforms = [ "aarch64-darwin" ];
  };
  gzip = { };
  inetutils = { };
  jq = {
    output = [ "bin" ];
  };
  less = { };
  lsof = { };
  m4 = { };
  ncdu_1 = { };
  netcat = { };
  ninja = { };
  openssl = {
    output = [ "bin" ];
  };
  # patchelf skips aarch64-linux (musl-static cross build currently fails). The
  # x86_64-linux pin to 25.05 predates that: unstable's make check hits an
  # R_X86_64_32 relocation error under musl-static.
  patchelf = {
    platforms = [
      "x86_64-linux"
      "aarch64-darwin"
    ];
    "x86_64-linux" = {
      version = "25.05";
    };
  };
  pkg-config-unwrapped = {
    alias = "pkg-config";
  };
  snappy = {
    output = [ "bin" ];
  };
  sqlite = {
    output = [ "bin" ];
  };
  tree = { };
  tzdata = {
    output = [ "out" ];
  };
  unzip = { };
  util-linux = { };
  xxd = { };
  xz = {
    output = [ "bin" ];
  };
  zip = { };
  zlib = {
    output = [ "bin" ];
  };
  zlib-ng = {
    output = [ "bin" ];
  };
  zstd = {
    output = [ "bin" ];
  };

  # fonts
  fira-code = {
    isStatic = false;
  };
  lxgw-wenkai = {
    isStatic = false;
  };
  "nerd-fonts.fira-code" = {
    isStatic = false;
    alias = "nerd-fonts-fira-code";
  };
  "nerd-fonts.ubuntu-mono" = {
    isStatic = false;
    alias = "nerd-fonts-ubuntu-mono";
  };

  # rust pkgs
  atuin = { };
  bat = { };
  # biome = { };
  dprint = { };
  eza = { };
  fd = { };
  git-absorb = { };
  mcfly = { };
  procs = { };
  ripgrep = { };
  ruff = { };
  starship = { };
  tokei = { };
  yazi-unwrapped = {
    alias = "yazi";
  };

  ## ---- linux only -------------------------------------------------------

  cronie = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  ethtool = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  indent = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  iproute2 = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  iptables = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  iputils = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  krb5 = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  libcap = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  lsb-release = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  lua = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  man = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  nettools = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  nil = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  nixfmt = { };
  numactl = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  procps = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  # qemu-user skips aarch64-linux (musl-static cross build currently fails).
  qemu-user = {
    platforms = [
      "x86_64-linux"
    ];
  };
  rsync = { };
  strace = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  tmux = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };

  # protobuf
  protobuf_23 = {
    version = "24.05";
  };
  protobuf_24 = {
    version = "25.05";
  };
  protobuf_25 = { };
  protobuf_26 = {
    version = "25.05";
  };
  protobuf_27 = { };
  protobuf_28 = {
    version = "25.05";
  };
  protobuf_29 = { };
  protobuf3_20 = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
    version = "24.05";
  };
  protobuf3_21 = {
    version = "24.05";
  };

  # s6 stack
  s6-dns = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
    output = [ "bin" ];
  };
  s6-linux-utils = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
    output = [ "bin" ];
  };
  s6-networking = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
    output = [ "bin" ];
  };
  s6-portable-utils = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
    output = [ "bin" ];
  };
  skalibs = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };

  # go pkgs
  #
  # Cross-platform Go tools below carry an `aarch64-darwin` override with
  # `isStatic = false` (native pkgs, CGO on). Their upstream CGO build already
  # links only /usr/lib + system frameworks (no /nix dylib), so no
  # CGO_ENABLED=0 override is needed on macOS; forcing it off would only make
  # the pure-Go binary retain a go-compiler store path and trip
  # buildGoModule's disallowedReferences check. Linux keeps the default
  # (unstable, pkgsStatic musl static). runc stays Linux-only because it is a
  # Linux container runtime (namespaces/cgroups) with no macOS build.
  age = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  sops = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  bazelisk = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  buildifier = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  croc = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  # docker-compose is a CLI plugin under libexec/docker/cli-plugins. Linux keeps
  # the default (unstable, pkgsStatic musl static); darwin uses native pkgs
  # because pkgsStatic fails to build the Go toolchain (missing static
  # libresolv) and the native binary already links only /usr/lib + frameworks.
  docker-compose = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  # docker-buildx: Linux only here (pkgsStatic musl static). darwin uses the
  # local package (packages/docker-buildx/darwin.nix) which redirects the Nix
  # libresolv load command to /usr/lib.
  docker-buildx = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  cmakeMinimal = {
    alias = "cmake";
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  delve = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  # dive skips aarch64-linux (musl-static cross build currently fails). The
  # x86_64-linux pin to 25.11 predates that: unstable's static dependency chain
  # (gpgme->gnupg->openldap) fails to locate Cyrus SASL. darwin uses native
  # pkgs.
  dive = {
    platforms = [
      "x86_64-linux"
      "aarch64-darwin"
    ];
    "x86_64-linux" = {
      version = "25.11";
    };
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  fzf = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  gdu = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  gh = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  git-lfs = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  go-outline = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  go-task = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  go-tools = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  gofumpt = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  golangci-lint = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  gomodifytags = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  gopls = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  gost = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  gotests = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  gotools = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  impl = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  lefthook = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  oras = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  # rclone: Linux only here (pkgsStatic musl static). darwin uses the local
  # package (packages/rclone/darwin.nix) which redirects the Nix libresolv
  # load command to /usr/lib.
  rclone = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  runc = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  scc = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  shfmt = {
    "aarch64-darwin" = {
      isStatic = false;
    };
  };
  lark-cli = {
    # Linux: default isStatic = true -> pkgsStatic musl static.
    # darwin: native pkgs (CGO on). The upstream CGO build already links only
    # /usr/lib + system frameworks (no /nix dylib); forcing CGO_ENABLED=0 would
    # make the pure-Go binary retain a reference to the go compiler's store path
    # and trip buildGoModule's disallowedReferences check.
    "aarch64-darwin" = {
      isStatic = false;
    };
  };

  # llvm pkgs
  lld_18 = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  lld_19 = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  lld_20 = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  lld_21 = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  lld_22 = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
  "llvmPackages_18.clang-unwrapped" = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
    alias = "clang18";
  };
  "llvmPackages_19.clang-unwrapped" = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
    alias = "clang19";
  };
  "llvmPackages_20.clang-unwrapped" = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
    alias = "clang20";
  };
  "llvmPackages_21.clang-unwrapped" = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
    alias = "clang21";
  };
  "llvmPackages_22.clang-unwrapped" = {
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
    alias = "clang22";
  };

  ## ---- cross-platform with per-platform overrides -----------------------
  # linux uses default version; darwin pins a specific version.
  aria2 = {
    "aarch64-darwin" = {
      version = "24.11";
    };
  };
  shellcheck = {
    "aarch64-darwin" = {
      version = "25.11";
    };
  };
  uv = {
    "aarch64-darwin" = {
      version = "25.11";
    };
  };
}
