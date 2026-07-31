# colima, darwin-only, colima binary only.
#
# Upstream nixpkgs wrapProgram bakes lima-full/qemu/docker /nix/store paths into
# colima's PATH (see pkgs/by-name/co/colima/package.nix postInstall). That would
# retain /nix/store references and violate invariant #1. Per this repo's model
# the runtime deps (lima, docker, ...) are installed separately, so we drop the
# wrapper and let colima resolve them from the user's ambient PATH. Shell
# completions are preserved.
#
# CGO stays on (upstream env.CGO_ENABLED = 1). The build links only against
# system libraries except one: the CGO net resolver pulls in a nix-store
# libresolv stub. libresolv.9.dylib ships in macOS /usr/lib, so rewrite that
# load command to the system copy to keep the binary /nix/store-free.
{
  colima,
}:

colima.overrideAttrs (_oldAttrs: {
  postInstall = ''
    installShellCompletion --cmd colima \
      --bash <($out/bin/colima completion bash) \
      --fish <($out/bin/colima completion fish) \
      --zsh <($out/bin/colima completion zsh)

    oldResolv=$(otool -L "$out/bin/colima" | awk '/libresolv/ {print $1}')
    if [ -n "$oldResolv" ]; then
      install_name_tool -change "$oldResolv" /usr/lib/libresolv.9.dylib "$out/bin/colima"
    fi
  '';
})
