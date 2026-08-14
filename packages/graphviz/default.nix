{
  lib,
  graphviz,
  gd,
  dejavu_fonts,
  python3,
}:

let
  gdMinimal =
    (gd.override {
      withXorg = false;
      libwebp = null;
      libtiff = null;
      libavif = null;
      fontconfig = null;
    }).overrideAttrs
      (oldAttrs: {
        buildInputs = builtins.filter (input: input != null) oldAttrs.buildInputs;
        configureFlags = (oldAttrs.configureFlags or [ ]) ++ [
          "--without-xpm"
          "--without-tiff"
          "--without-webp"
          "--without-avif"
          "--without-fontconfig"
        ];
      });
  wrapper = ./dot-wrapper.sh;
  engines = [
    "dot"
    "neato"
    "twopi"
    "fdp"
    "circo"
    "sfdp"
    "osage"
    "patchwork"
  ];
  fontAliases = [
    "times"
    "Times"
    "TIMES"
    "timesroman"
    "TimesRoman"
    "TIMESROMAN"
    "arial"
    "Arial"
    "ARIAL"
    "helvetica"
    "Helvetica"
    "HELVETICA"
    "courier"
    "Courier"
    "COURIER"
  ];
in

(graphviz.override {
  withXorg = false;
  gd = gdMinimal;
  pango = null;
  fontconfig = null;
  inherit python3;
}).overrideAttrs
  (oldAttrs: {
    buildInputs = builtins.filter (input: input != null) oldAttrs.buildInputs;
    configureFlags = (oldAttrs.configureFlags or [ ]) ++ [
      "--disable-ltdl"
      "--without-pangocairo"
      "--without-gdk"
      "--without-gdk-pixbuf"
      "--without-gtk"
      "--without-webp"
      "--without-poppler"
      "--without-rsvg"
    ];
    postInstall = (oldAttrs.postInstall or "") + ''
      cd "$out/bin"
      mv dot_static _dot
      cp ${wrapper} dot
      chmod 0555 dot
      ${lib.concatMapStringsSep "\n" (engine: "ln -s dot ${engine}") (lib.remove "dot" engines)}
      rm -f gvmap.sh

      rm -rf "$out/include" "$out/lib" "$out/nix-support"
      mkdir -p "$out/share/fonts"
      cp ${dejavu_fonts.minimal}/share/fonts/truetype/DejaVuSans.ttf "$out/share/fonts/"
      cd "$out/share/fonts"
      ${lib.concatMapStringsSep "\n" (name: "ln -s DejaVuSans.ttf ${name}.ttf") fontAliases}
    '';
    nativeInstallCheckInputs = [ ];
    doInstallCheck = true;
    installCheckPhase = ''
      runHook preInstallCheck

      export GDFONTPATH="$out/share/fonts"
      cd "$TMPDIR"
      printf 'digraph { a [label="Hello world"]; a -> b }\n' |
        "$out/bin/dot" -Tpng > graph.png
      printf 'graph { a -- b }\n' |
        "$out/bin/sfdp" -Tsvg > graph.svg
      test -s graph.png
      grep -q '<svg' graph.svg

      runHook postInstallCheck
    '';
  })
