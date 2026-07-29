# Shared Node CLI tool wiring (Linux + Darwin), each bound to the caller's
# platform `nodejs-slim26`. Two build strategies, don't conflate them:
#   - pnpm / prettier: interpreter overridden at BUILD time (built against the
#     static node).
#   - markdownlint-cli2 / opencommit: built with the regular node (their
#     buildNpmPackage needs npm, which nodejs-slim lacks) and only switch to the
#     static node at RUNTIME via the wrapper.
{
  pkgs,
  pkgsStatic,
  nodejs-slim26,
}:
{
  pnpm = pkgsStatic.callPackage ../pnpm {
    pnpm = pkgs.pnpm.override { nodejs-slim = nodejs-slim26; };
  };
  prettier = pkgsStatic.callPackage ../prettier {
    prettier = pkgs.prettier.override { nodejs = nodejs-slim26; };
  };
  markdownlint-cli2 = pkgsStatic.callPackage ../markdownlint-cli2 {
    inherit nodejs-slim26;
    inherit (pkgs) markdownlint-cli2;
  };
  opencommit = pkgsStatic.callPackage ../opencommit {
    inherit nodejs-slim26;
    inherit (pkgs) opencommit;
  };
}
