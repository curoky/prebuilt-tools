{
  pkgs,
  pkgsStatic,
}:
let
  nodejs-slim26 = pkgsStatic.callPackage ../nodejs/26/darwin.nix {
    inherit (pkgs) python3 cctools;
  };
  nodeTools = import ./node-tools.nix {
    inherit pkgs pkgsStatic nodejs-slim26;
  };
in
{
  # Partial-static C packages.
  ffmpeg = pkgsStatic.callPackage ../ffmpeg/darwin.nix { };
  krb5 = pkgsStatic.callPackage ../krb5/darwin.nix { };
  wget = pkgsStatic.callPackage ../wget/darwin-static.nix {
    inherit (pkgs) perlPackages;
  };

  # Perl.
  perl = pkgs.callPackage ../perl/darwin.nix {
    libxcryptStatic = pkgsStatic.libxcrypt;
  };
  exiftool = pkgs.callPackage ../exiftool/darwin.nix {
    inherit pkgsStatic;
  };

  # Node.js runtime and sibling-runtime tools.
  inherit nodejs-slim26;
  inherit (nodeTools)
    markdownlint-cli2
    opencommit
    pnpm
    prettier
    ;
}
