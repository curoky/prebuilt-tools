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
          artifactTool =
            pkgs.runCommand "standalone-artifact-tool"
              {
                nativeBuildInputs = [ pkgs.buildPackages.go ];
              }
              ''
                cp -R ${./cmd/artifact} source
                chmod -R u+w source
                cd source
                export CGO_ENABLED=0
                export GO111MODULE=off
                export GOCACHE=$TMPDIR/go-cache
                mkdir -p "$out/bin"
                go build -trimpath -ldflags="-s -w" -o "$out/bin/artifact"
              '';
          makeManifestPackages = import ./lib/make-manifest-packages.nix {
            inherit lib envs;
            allSystems = systems;
          };
          makeArtifacts = import ./lib/make-artifacts.nix {
            inherit pkgs artifactTool;
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

          artifacts = lib.mapAttrs makeArtifacts allPackages;
          standalonePackages = artifacts;
          tarballPackages = lib.mapAttrs (_: artifact: artifact.archive) artifacts;

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
                lib.filterAttrs (name: _: pred name) standalonePackages
              )
            );
        in
        {
          packages = standalonePackages // {
            # Convenience aggregate of all standalone packages.
            all = mkAll "all-standalone-tools" (_: true);

            # Same as `all` but skips slow LLVM toolchain packages; handy for
            # quick local verification: `nix build .#all-fast`.
            all-fast = mkAll "all-standalone-tools-fast" (name: !isSlowLLVM name);
          };
          tarballs = tarballPackages;
          # Pre-artifact upstream/local derivations (the `--source` inputs to
          # make-artifacts), keyed by package name. Standalone outputs relativize
          # their contents and drop all store references, so the source closure
          # is absent from the standalone closure. Exposing it lets CI push the
          # source closure to the cache so rebuilds substitute upstream deps
          # instead of rebuilding from source.
          sources = allPackages;
        };
    in
    let
      perSystemOutputs = lib.genAttrs systems perSystem;
    in
    {
      packages = lib.mapAttrs (_: o: o.packages) perSystemOutputs;
      tarballs = lib.mapAttrs (_: o: o.tarballs) perSystemOutputs;
      sources = lib.mapAttrs (_: o: o.sources) perSystemOutputs;
    };
}
