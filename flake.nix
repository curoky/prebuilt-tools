{
  description = "Standalone, portable prebuilt tool binaries built with Nix";

  inputs = {
    nixpkgs-unstable.url = "github:NixOS/nixpkgs/nixos-unstable";
    nixpkgs-2605.url = "github:NixOS/nixpkgs/nixos-26.05";
    nixpkgs-2511.url = "github:NixOS/nixpkgs/nixos-25.11";
    nixpkgs-2505.url = "github:NixOS/nixpkgs/nixos-25.05";
    nixpkgs-2411.url = "github:NixOS/nixpkgs/nixos-24.11";
    nixpkgs-2405.url = "github:NixOS/nixpkgs/nixos-24.05";
  };

  outputs =
    inputs:
    let
      lib = inputs.nixpkgs-unstable.lib;

      systems = [
        "x86_64-linux"
        "aarch64-darwin"
      ];

      # Build the per-system package set.
      perSystem =
        system:
        let
          isDarwin = lib.hasSuffix "darwin" system;

          # One env per pinned nixpkgs input, exposing both regular and static
          # package sets. The manifest selects which env + variant to use.
          #
          # On Linux, `pkgsStatic` is the musl64 *cross* set
          # (pkgsCross.musl64.pkgsStatic: build = glibc, host == target =
          # musl-static) rather than the native-static set (build == host ==
          # target == musl). Both are x86-64, so `buildPlatform.canExecute
          # hostPlatform` stays true and checkPhases are not disabled. The
          # reason for cross: packages that link Rust deps (e.g. node 26's
          # temporal_capi) otherwise rebuild the entire musl LLVM + rustc
          # toolchain from source; the cross set takes rust's `fastCross` path,
          # reusing the cached glibc rustc/LLVM instead. On Darwin the native
          # pkgsStatic is kept (pkgsCross.musl64 there means cross-to-Linux).
          mkEnv =
            input:
            let
              base = import input { inherit system; };
            in
            {
              pkgs = base;
              pkgsStatic = if isDarwin then base.pkgsStatic else base.pkgsCross.musl64.pkgsStatic;
            };
          envs = {
            "unstable" = mkEnv inputs.nixpkgs-unstable;
            "26.05" = mkEnv inputs.nixpkgs-2605;
            "25.11" = mkEnv inputs.nixpkgs-2511;
            "25.05" = mkEnv inputs.nixpkgs-2505;
            "24.11" = mkEnv inputs.nixpkgs-2411;
            "24.05" = mkEnv inputs.nixpkgs-2405;
          };

          pkgs = envs.unstable.pkgs;
          pkgsStatic = envs.unstable.pkgsStatic;

          # --- helpers -----------------------------------------------------
          makeManifestPackages = import ./lib/make-manifest-packages.nix {
            inherit lib envs;
            allSystems = systems;
          };
          makeStandalone = import ./lib/make-standalone.nix {
            inherit pkgs;
            normalizeScript = ./scripts/normalize.sh;
          };
          makeTarball = import ./lib/make-tarball.nix {
            inherit pkgs;
          };

          # --- upstream packages (manifest-driven) -------------------------
          manifest = import ./manifests/default.nix;
          upstreamPackages = makeManifestPackages system manifest;

          # --- local packages (patched / wrapped / pinned) -----------------
          localPackages = import ./packages/local.nix {
            inherit pkgs pkgsStatic;
          };

          allPackages =
            upstreamPackages
            // localPackages.common
            // lib.optionalAttrs isDarwin localPackages.darwin
            // lib.optionalAttrs (!isDarwin) localPackages.linux;

          # Normalize every derivation into a standalone payload.
          standalonePackages = lib.mapAttrs (
            name: drv: if lib.isDerivation drv then makeStandalone name drv else drv
          ) allPackages;

          # Pack each standalone payload into its published `<name>.<arch>.tar.gz`
          # artifact. Exposed under the separate `tarballs` flake output so the
          # flat `packages` namespace (which `discover` enumerates) is unchanged.
          tarballPackages = lib.mapAttrs (
            name: drv: if lib.isDerivation drv then makeTarball name standalonePackages.${name} else drv
          ) allPackages;

          # Slow-to-build LLVM toolchain packages (clang-tools / clang / lld).
          # Excluded from `all-fast` so local `nix build .#all-fast` is quick.
          # CI still builds these via the dedicated build-llvm-tools workflow.
          isSlowLLVM =
            name:
            lib.hasPrefix "clang-tools-" name
            || lib.hasPrefix "lld_" name
            || (lib.match "clang[0-9]+" name) != null;

          mkAll =
            label: pred:
            pkgs.linkFarm label (
              lib.mapAttrsToList (name: path: { inherit name path; }) (
                lib.filterAttrs (n: v: lib.isDerivation v && pred n) standalonePackages
              )
            );
        in
        {
          packages =
            standalonePackages
            // {
              # Convenience aggregate of all standalone packages.
              all = mkAll "all-standalone-tools" (_: true);

              # Same as `all` but skips slow LLVM toolchain packages; handy for
              # quick local verification: `nix build .#all-fast`.
              all-fast = mkAll "all-standalone-tools-fast" (name: !isSlowLLVM name);
            };
          tarballs = tarballPackages;
        };
    in
    let
      perSystemOutputs = lib.genAttrs systems perSystem;
    in
    {
      packages = lib.mapAttrs (_: o: o.packages) perSystemOutputs;
      tarballs = lib.mapAttrs (_: o: o.tarballs) perSystemOutputs;
    };
}
