{
  aardvark-dns,
}:

# aardvark-dns 2.1.0 calls `libc::close_range` unconditionally, but the musl
# libc bindings only expose `SYS_close_range` (not the wrapper function), so the
# stock package fails to compile under the musl-static toolchain. The patch only
# touches the crate's own `src/main.rs`, not any vendored dependency, so the
# upstream cargo vendor hash stays valid and no cargoHash override is needed.
aardvark-dns.overrideAttrs (oldAttrs: {
  patches = (oldAttrs.patches or [ ]) ++ [
    ./musl-close-range-syscall.patch
  ];
})
