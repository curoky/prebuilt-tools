{
  lib,
  stdenv,
  termcap,
}:
{
  python,
  setupLocal,
  patchMuslStatx ? false,
}:
python.overrideAttrs (oldAttrs: {
  configureFlags = oldAttrs.configureFlags ++ [
    "LDFLAGS=-L${termcap}/lib"
  ];
  stripIdlelib = true;
  stripTests = true;
  stripTkinter = true;
  postPatch =
    oldAttrs.postPatch
    + ''
      cp ${setupLocal} Modules/Setup.local
    ''
    + lib.optionalString (patchMuslStatx && stdenv.hostPlatform.isMusl) ''
      substituteInPlace Modules/posixmodule.c \
        --replace-fail stx_dio_offset_align stx_dio_offet_align
    '';
})
