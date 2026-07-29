{
  stdenv,
  mold,
}:
{
  llvmPackages,
  version,
}:
let
  clang = llvmPackages.clang-unwrapped.overrideAttrs (oldAttrs: {
    nativeBuildInputs = (oldAttrs.nativeBuildInputs or [ ]) ++ [ mold ];
    env = {
      NIX_CFLAGS_COMPILE =
        (oldAttrs.NIX_CFLAGS_COMPILE or "") + " -g0 -ffunction-sections -fdata-sections";
      NIX_LDFLAGS = (oldAttrs.NIX_LDFLAGS or "") + " --gc-sections -s";
    };

    cmakeFlags = (oldAttrs.cmakeFlags or [ ]) ++ [
      "-DCMAKE_BUILD_TYPE=MinSizeRel"
      "-DLLVM_USE_LINKER=mold"
    ];
  });
in
stdenv.mkDerivation {
  pname = "clang-tools";
  inherit version;

  unpackPhase = ":";
  buildPhase = ":";

  installPhase = ''
    mkdir -p $out/bin
    cp ${clang}/bin/clang-format $out/bin/
  '';
}
