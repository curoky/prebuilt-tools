{
  lib,
  stdenv,
  fetchurl,
  python315,
  writeText,
  termcap,
}:

let
  Modules_Setup_local = ./Setup.local;
in
python315.overrideAttrs (oldAttrs: rec {
  # https://wiki.python.org/moin/BuildStatically
  # https://github.com/python/cpython/blob/3.11/Modules/Setup
  configureFlags = oldAttrs.configureFlags ++ [
    "LDFLAGS=-L${termcap}/lib"
    #"--with-ensurepip=install"
  ];
  stripIdlelib = true;
  stripTests = true;
  stripTkinter = true;
  postPatch =
    oldAttrs.postPatch
    + ''
      cp ${Modules_Setup_local} Modules/Setup.local
    ''
    # musl's <bits/statx.h> misspells the struct statx member as
    # `stx_dio_offet_align` (missing the second `s`), while CPython 3.15's
    # posixmodule.c uses the correct kernel name `stx_dio_offset_align`.
    # configure only probes stx_dio_mem_align (spelled correctly in musl) and
    # then defines HAVE_STRUCT_STATX_STX_DIO_MEM_ALIGN, so the offset-align
    # references in the same #ifdef block fail to compile. Rewrite them to the
    # name musl actually ships.
    + lib.optionalString stdenv.hostPlatform.isMusl ''
      substituteInPlace Modules/posixmodule.c \
        --replace-fail stx_dio_offset_align stx_dio_offet_align
    '';
})
