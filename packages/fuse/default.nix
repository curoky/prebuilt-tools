# Local fuse (libfuse 2.x) override.
#
# Two stock deps break the musl-static build via `shadow -> libbsd`, whose
# checkPhase aborts on the `explicit_bzero` test:
#   1. util/mount.fuse.c substitutes `"su"` with `${shadow.su}/bin/su`, which
#      also bakes a /nix/store path into the portable `mount.fuse` binary.
#   2. fuse2's autotools build resolves `util-linux` to the full package, whose
#      `su`/login support pulls in shadow; fuse3 already uses util-linux-minimal.
# `mount.fuse` only needs `su`/`mount`/`umount` on its runtime PATH, so keep the
# bare `"su"` lookup and use util-linux-minimal for the mount/umount paths,
# dropping shadow entirely. fuse3 has no such issue and stays on the stock
# manifest build.
{
  fuse,
  runtimeShell,
  util-linuxMinimal,
  lib,
}:

fuse.overrideAttrs (oldAttrs: {
  preConfigure = ''
    substituteInPlace lib/mount_util.c \
      --replace-fail "/bin/mount" "${lib.getBin util-linuxMinimal}/bin/mount" \
      --replace-fail "/bin/umount" "${lib.getBin util-linuxMinimal}/bin/umount"
    substituteInPlace util/mount.fuse.c \
      --replace-fail "/bin/sh" "${runtimeShell}"

    export MOUNT_FUSE_PATH=$bin/bin
    export INIT_D_PATH=$TMPDIR/etc/init.d
    export UDEV_RULES_PATH=$TMPDIR/etc/udev/rules.d
  '';
})
