# gnutar — fully-static musl build.
#
# The stock `pkgsStatic.gnutar` (1.35) fails to link here with a duplicate-symbol
# error:
#
#   libtar.a(xattr-at.o): multiple definition of `setxattrat' (also `getxattrat',
#   `listxattrat'); acl-static/libacl.a(xattrat.o): first defined here
#
# gnutar bundles an old gnulib `xattr-at` module that provides its own
# `*xattrat` wrappers (via gnulib's `at-func.c`). Newer libacl (2.4.0, pulled in
# statically here) now ships real `*xattrat` symbols too, so linking `tar`
# statically pulls both definitions in and GCC 15's default `-fno-common`
# surfaces the collision. Both implementations are equivalent fallbacks, so tell
# the linker to keep the first and drop the duplicate — this preserves ACL and
# xattr support instead of disabling them.
{
  gnutar,
}:

gnutar.overrideAttrs (old: {
  # Pass the flag only to `make` (not to configure's compiler-works check, which
  # breaks under the musl cross toolchain if NIX_LDFLAGS is overridden). LDFLAGS
  # is an automake user variable, appended after AM_LDFLAGS, so it only augments
  # the final link.
  makeFlags = (old.makeFlags or [ ]) ++ [ "LDFLAGS=-Wl,--allow-multiple-definition" ];
})