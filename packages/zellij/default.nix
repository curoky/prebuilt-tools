{
  zellij-unwrapped,
}:

# `zellij` (nixpkgs) is only a symlinkJoin/wrapper around `zellij-unwrapped`
# and adds nothing unless `extraPackages` is set (a PATH-injecting binary
# wrapper). We don't need that, so build `zellij-unwrapped` directly — one
# fewer derivation and no intermediate symlink layer.
#
# Under the musl64 *cross* static set build==host==x86-64, so `checkPhase` is
# not auto-skipped and cargoCheckHook would build the `zellij` test target —
# which statically links libcurl against libssh2 and fails on unresolved
# libssh2 1.11 symbols (link ordering). Disable the check to avoid it.
zellij-unwrapped.overrideAttrs {
  doCheck = false;
  doInstallCheck = false;
}
