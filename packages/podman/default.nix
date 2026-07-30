{
  lib,
  stdenv,
  fetchFromGitHub,
  # pkg-config,
  # installShellFiles,
  # buildGoModule,
  # buildPackages,
  gpgme,
  # gnupg,
  lvm2,
  btrfs-progs,
  libapparmor,
  libseccomp,
  libselinux,
  # systemd,
  # nixosTests,
  # python3,
  # makeBinaryWrapper,
  # symlinkJoin,
  replaceVars,
  # extraPackages ? [ ],
  crun,
  runc,
  conmon,
  # extraRuntimes ? lib.optionals stdenv.hostPlatform.isLinux [ runc ], # e.g.: runc, gvisor, youki
  # fuse-overlayfs,
  # util-linuxMinimal,
  # nftables,
  # iptables,
  # iproute2,
  catatonit,
  # gvproxy,
  # aardvark-dns,
  # netavark,
  # passt,
  # vfkit,
  # versionCheckHook,
  # writableTmpDirAsHomeHook,
  coreutils,
  # runtimeShell,
  podman,
}:
let
  podman_bin = ./bin;
  podman_conf = ./conf;

  # runc is pulled into podman's helpersBin via the upstream `extraRuntimes`
  # default. Under pkgsStatic the real runc binary is already fully static, but
  # the upstream installPhase runs `wrapProgram` on it, which renames the static
  # binary to `.runc-wrapped` and installs a small *dynamic* launcher named
  # `runc` (it references a /nix musl interpreter + rpath). podman's helpersBin
  # ships that launcher, so the copied `runc` ends up dynamic and depends on
  # /nix, tripping the standalone portability check. (The standalone `.#runc`
  # output avoids this because artifact assembly restores `.runc-wrapped` over
  # the launcher.) Drop the wrapper here and install the static binary
  # directly; the PATH prefix it adds is not needed for the shipped runtime.
  runcStatic = runc.overrideAttrs (_: {
    installPhase = ''
      runHook preInstall
      install -Dm755 runc $out/bin/runc
      installManPage man/*/*.[1-9]
      runHook postInstall
    '';
  });
in
(podman.override {
  conmon = conmon;
  catatonit = catatonit;
  crun = crun;
  runc = runcStatic;
}).overrideAttrs
  (oldAttrs: rec {
    propagatedBuildInputs = [ ];
    buildInputs = lib.optionals stdenv.hostPlatform.isLinux [
      btrfs-progs
      gpgme
      libapparmor
      libseccomp
      libselinux
      lvm2
      # systemd
    ];

    nativeInstallCheckInputs = [
      coreutils
    ];

    env = oldAttrs.env // {
      HELPER_BINARIES_DIR2 = "/opt/podmanx/libexec/podman";

      LDFLAGS = lib.concatStringsSep " " (
        lib.filter (s: s != "") [
          (oldAttrs.env.LDFLAGS or "")
          "-X go.podman.io/storage/pkg/configfile.adminOverrideConfigPath=/opt/podmanx/conf/"
        ]
      );
    };

    patches = [
      (replaceVars ./hardcode-paths.patch {
        bin_path = "/opt/podmanx/libexec/podman";
      })

      # we intentionally don't build and install the helper so we shouldn't display messages to users about it
      ./rm-podman-mac-helper-msg.patch
    ];

    postPatch = (oldAttrs.postPatch or "") + ''
      substituteInPlace Makefile \
        --replace-fail HELPER_BINARIES_DIR HELPER_BINARIES_DIR2
    '';

    postFixup = "";
    postInstall = "
      cp -Lf --remove-destination ${oldAttrs.passthru.helpersBin}/bin/* ${oldAttrs.env.HELPER_BINARIES_DIR}

      mv $out/bin/.podman-wrapped $out/bin/_podman
      rm -f $out/bin/podmansh

      mkdir -p $out/conf
      cp ${podman_bin}/* $out/bin/
      cp ${podman_conf}/* $out/conf/
    ";
  })
