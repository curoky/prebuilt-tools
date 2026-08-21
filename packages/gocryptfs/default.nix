{
  lib,
  gocryptfs,
  openssl,
}:

# gocryptfs links openssl via CGO for its crypto backend; that builds fine under
# musl-static. The only obstacle to the stock pkgsStatic build is the
# `libfido2` propagatedBuildInput, whose transitive `pcsclite` dependency fails
# to build its `doc` output under musl-static.
#
# libfido2 is NOT a link-time dependency: gocryptfs' FIDO2 support shells out to
# the `fido2-assert` / `fido2-cred` CLI tools at runtime (see
# internal/fido2/fido2.go using os/exec). nixpkgs propagates libfido2 only so
# those CLI tools land on PATH. In a standalone, relocatable build we resolve
# fusermount and the fido2 tools from the host $PATH anyway, so drop the
# propagated input entirely.
gocryptfs.overrideAttrs (_: {
  propagatedBuildInputs = [ ];
  # cgo resolves the openssl crypto backend via `#cgo pkg-config: libcrypto`.
  # Point PKG_CONFIG_PATH at the static openssl dev output so the target
  # pkg-config wrapper can locate libcrypto.pc during the cross build.
  PKG_CONFIG_PATH = "${lib.getDev openssl}/lib/pkgconfig";
})
