{
  lib,
  poppler,
  fontconfig,
  freetype,
  expat,
  bzip2,
  brotli,
  openjpeg,
}:
let
  popplerLibsOld = "set(poppler_LIBS \${poppler_LIBS} Fontconfig::Fontconfig)";
  popplerLibsNew =
    "set(poppler_LIBS\n"
    + "  \${poppler_LIBS}\n"
    + "  Fontconfig::Fontconfig\n"
    + "  ${fontconfig.lib}/lib/libfontconfig.a\n"
    + "  ${freetype}/lib/libfreetype.a\n"
    + "  ${expat}/lib/libexpat.a\n"
    + "  ${bzip2.out}/lib/libbz2.a\n"
    + "  ${brotli.lib}/lib/libbrotlidec.a\n"
    + "  ${brotli.lib}/lib/libbrotlicommon.a\n"
    + ")\n";
in
(poppler.override {
  suffix = "utils";
  utils = true;
  minimal = true;
}).overrideAttrs
  (oldAttrs: {
    # Linux musl-static stock poppler-utils first pulls in nss -> p11-kit, then
    # openjpeg -> libtiff -> giflib. The utils we ship do not need either chain.
    propagatedBuildInputs = lib.subtractLists [ openjpeg ] oldAttrs.propagatedBuildInputs;
    doCheck = false;
    cmakeFlags = (oldAttrs.cmakeFlags or [ ]) ++ [
      "-DENABLE_LIBOPENJPEG=none"
      "-DBUILD_TESTING=OFF"
    ];
    postPatch =
      (oldAttrs.postPatch or "")
      + "substituteInPlace CMakeLists.txt \\\n"
      + "  --replace-fail ${lib.escapeShellArg popplerLibsOld} ${lib.escapeShellArg popplerLibsNew}\n";
  })
