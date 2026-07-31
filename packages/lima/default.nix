# lima, darwin-only, hostside limactl + helpers + bundled guest agents.
#
# Upstream nixpkgs wrapProgram bakes qemu's /nix/store path into limactl's PATH
# (see pkgs/by-name/li/lima/package.nix installPhase). That would retain a
# /nix/store reference and violate invariant #1. Per this repo's model the
# runtime deps are installed separately, and darwin lima defaults to the VZ
# backend (Virtualization.framework) rather than qemu, so we drop the wrapper
# and let limactl resolve any external tools from the user's ambient PATH.
#
# The real binary is built into $out/bin/limactl directly (no wrapProgram), so
# the sibling *.lima helper scripts, shell completions and the bundled guest
# agents / templates under share/lima all stay intact and relocatable.
#
# CGO stays on; like colima the only non-system dependency is a nix-store
# libresolv stub, pulled into every Mach-O binary lima ships (limactl plus the
# libexec helpers limactl-mcp and lima-driver-krunkit). libresolv.9.dylib ships
# in macOS /usr/lib, so rewrite that load command to the system copy in each to
# keep the output /nix/store-free.
#
# limactl is adhoc-codesigned with vz.entitlements (com.apple.security.
# virtualization) so the VZ backend works; that is also why upstream sets
# dontStrip on darwin. install_name_tool voids the signature, so after each
# rewrite we re-sign adhoc: limactl with the source's vz.entitlements (the same
# file upstream's Makefile signs with; postInstall still runs in the unpacked
# source dir), the helpers with a plain adhoc signature. codesign comes from
# darwin.sigtool (nativeBuildInputs).
{
  lima,
}:

lima.overrideAttrs (oldAttrs: {
  installPhase = ''
    runHook preInstall
    mkdir -p $out
    cp -r _output/* $out
    installShellCompletion --cmd limactl \
      --bash <($out/bin/limactl completion bash) \
      --fish <($out/bin/limactl completion fish) \
      --zsh <($out/bin/limactl completion zsh)
    runHook postInstall
  '';

  postInstall = (oldAttrs.postInstall or "") + ''
    entitlements="$PWD/vz.entitlements"
    for bin in "$out/bin/limactl" "$out/libexec/lima/limactl-mcp" "$out/libexec/lima/lima-driver-krunkit"; do
      [ -f "$bin" ] || continue
      oldResolv=$(otool -L "$bin" | awk '/\/nix\/store\/.*libresolv/ {print $1}')
      [ -n "$oldResolv" ] || continue
      install_name_tool -change "$oldResolv" /usr/lib/libresolv.9.dylib "$bin"
      if [ "$bin" = "$out/bin/limactl" ]; then
        codesign -f --entitlements "$entitlements" -s - "$bin"
      else
        codesign -f -s - "$bin"
      fi
    done
  '';
})
