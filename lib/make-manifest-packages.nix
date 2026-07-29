# make-manifest-packages.nix
#
# Build a set of upstream nixpkgs packages from a declarative manifest for a
# given system.
#
# Manifest schema (see manifests/default.nix for the full reference):
#   <pkg> = {
#     platforms = [ "x86_64-linux" "aarch64-darwin" ];  # omitted => allSystems
#     version  = "unstable";   # which nixpkgs env (default "unstable")
#     isStatic = true;         # pkgsStatic (true, default) or pkgs (false)
#     output   = [ "out" ];    # outputs to expose (default [ "out" ])
#     alias    = "name";       # rename the exported attribute
#     "<system>" = { ... };    # per-platform override of the fields above
#   };
#
# The effective config for `system` is the package-level shared config merged
# with the matching platform key (platform wins). Packages whose `platforms`
# list does not contain `system` are skipped.
{
  lib,
  envs,
  allSystems,
}:
system: manifest:
let
  reserved = [ "platforms" ] ++ allSystems;
in
lib.concatMapAttrs (
  name: raw:
  let
    platforms = raw.platforms or allSystems;
    enabled = lib.elem system platforms;

    shared = lib.removeAttrs raw reserved;
    perPlatform = raw.${system} or { };
    conf = shared // perPlatform;

    targetVer = conf.version or "unstable";
    env = envs.${targetVer};
    base = if conf.isStatic or true then env.pkgsStatic else env.pkgs;

    rawPkg = base.${name} or (lib.attrByPath (lib.splitString "." name) null base);

    selectedOutputs = conf.output or [ "out" ];
    finalName = conf.alias or name;

    finalDrv = env.pkgs.symlinkJoin {
      name = finalName;
      paths = map (o: lib.getOutput o rawPkg) selectedOutputs;
    };
  in
  lib.optionalAttrs enabled {
    "${finalName}" = finalDrv;
  }
) manifest
