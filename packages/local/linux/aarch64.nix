# aarch64-linux-only local packages.
#
# Currently empty: aarch64-linux uses only the shared set from common.nix.
# python311 is intentionally absent here (its musl-static cross build currently
# fails on aarch64-linux); it is built on x86_64-linux via x86_64.nix. See the
# docs regression table for the tracking entry.
{
  pkgs,
  pkgsStatic,
}:
{
}
