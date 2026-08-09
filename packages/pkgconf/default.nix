{ pkgconf-unwrapped }:

pkgconf-unwrapped.overrideAttrs (oldAttrs: {
  configureFlags = (oldAttrs.configureFlags or [ ]) ++ [
    "--with-pkg-config-dir=/usr/local/lib/pkgconfig:/usr/local/share/pkgconfig:/usr/lib/pkgconfig:/usr/share/pkgconfig"
    "--with-system-libdir=/usr/lib:/lib"
    "--with-system-includedir=/usr/include"
    "--with-personality-dir=/usr/local/share/pkgconfig/personality.d:/usr/share/pkgconfig/personality.d"
  ];
})
