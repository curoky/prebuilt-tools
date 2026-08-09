{ lua5_5 }:

lua5_5.overrideAttrs (oldAttrs: {
  postPatch = (oldAttrs.postPatch or "") + ''
    substituteInPlace src/luaconf.h \
      --replace-fail "$out/" "/usr/local/"
  '';
})
