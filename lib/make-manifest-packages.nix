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
  configFields = [
    "version"
    "isStatic"
    "output"
    "alias"
  ];
  reserved = [ "platforms" ] ++ allSystems;
  fail = name: message: throw "manifest entry ${name}: ${message}";
  require =
    name: condition: message: value:
    if condition then value else fail name message;
  validateConfig =
    name: config:
    let
      checkedConfig =
        require name (builtins.isAttrs config) "configuration must be an attribute set"
          config;
      unknownFields = lib.subtractLists configFields (builtins.attrNames checkedConfig);
      version = checkedConfig.version or "unstable";
      outputs = checkedConfig.output or [ "out" ];
      alias = checkedConfig.alias or name;
    in
    require name (unknownFields == [ ]) "unknown fields: ${lib.concatStringsSep ", " unknownFields}" (
      require name (builtins.isString version && builtins.hasAttr version envs)
        "unknown nixpkgs version ${builtins.toJSON version}"
        (
          require name (builtins.isBool (checkedConfig.isStatic or true)) "isStatic must be a boolean" (
            require name (builtins.isList outputs && outputs != [ ] && builtins.all builtins.isString outputs)
              "output must be a non-empty list of strings"
              (
                require name (lib.length outputs == lib.length (lib.unique outputs))
                  "output must not contain duplicates"
                  (
                    require name (
                      builtins.isString alias && alias != ""
                    ) "alias must be a non-empty string" checkedConfig
                  )
              )
          )
        )
    );
  validateEntry =
    name: raw:
    let
      checkedRaw = require name (builtins.isAttrs raw) "entry must be an attribute set" raw;
      allowedFields = [ "platforms" ] ++ configFields ++ allSystems;
      unknownFields = lib.subtractLists allowedFields (builtins.attrNames checkedRaw);
      platforms = checkedRaw.platforms or allSystems;
      shared = lib.removeAttrs checkedRaw reserved;
      platformConfigs = map (
        platform:
        if builtins.hasAttr platform checkedRaw then
          validateConfig "${name}.${platform}" checkedRaw.${platform}
        else
          { }
      ) allSystems;
    in
    require name (unknownFields == [ ]) "unknown fields: ${lib.concatStringsSep ", " unknownFields}" (
      require name
        (
          builtins.isList platforms
          && platforms != [ ]
          && builtins.all builtins.isString platforms
          && builtins.all (platform: lib.elem platform allSystems) platforms
        )
        "platforms must be a non-empty list containing only: ${lib.concatStringsSep ", " allSystems}"
        (
          require name (lib.length platforms == lib.length (lib.unique platforms))
            "platforms must not contain duplicates"
            (builtins.deepSeq [ (validateConfig name shared) platformConfigs ] checkedRaw)
        )
    );
  checkedManifest =
    require "<manifest>" (builtins.isAttrs manifest) "manifest must be an attribute set"
      manifest;
  checkedSystem =
    require "<system>" (lib.elem system allSystems) "unsupported system ${builtins.toJSON system}"
      system;
in
lib.concatMapAttrs (
  name: raw:
  let
    checkedRaw = validateEntry name raw;
    platforms = checkedRaw.platforms or allSystems;
    enabled = lib.elem checkedSystem platforms;

    shared = lib.removeAttrs checkedRaw reserved;
    perPlatform = checkedRaw.${checkedSystem} or { };
    conf = shared // perPlatform;

    targetVer = conf.version or "unstable";
    env = envs.${targetVer};
    base = if conf.isStatic or true then env.pkgsStatic else env.pkgs;

    rawPkg = lib.getAttrFromPath (lib.splitString "." name) base;

    selectedOutputs = conf.output or [ "out" ];
    selectedPaths = map (output: lib.getOutput output rawPkg) selectedOutputs;
    finalName = conf.alias or name;

    # A single output is already a complete tree; only pay for an lndir-based
    # symlinkJoin when multiple outputs actually need to be merged.
    finalDrv =
      if lib.length selectedPaths == 1 then
        lib.head selectedPaths
      else
        env.pkgs.symlinkJoin {
          name = finalName;
          paths = selectedPaths;
        };
  in
  lib.optionalAttrs enabled {
    "${finalName}" = finalDrv;
  }
) checkedManifest
