{
  lib,
  s6,
  execline,
  symlinkJoin,
  s6-linux-init,
}:

let
  # s6 is passed in already patched (see pkgs/patched/s6.nix). It MUST be wired
  # back into the build via `override`, because s6-linux-init bakes the s6 store
  # path (s6's S6_EXTBINPREFIX) into its binaries at compile time via <s6/config.h>.
  # Just receiving `s6` as an argument without `override` has no effect.
  #
  # execline is likewise passed in already patched and MUST be wired back in via
  # `override`: s6-linux-init-maker bakes execline's EXECLINE_EXTBINPREFIX (from
  # <execline/config.h>) into every generated init script (bin/init, the
  # .s6-svscan/* handlers, run-image scripts) as the absolute path to
  # foreground/fdmove/eltest/... Without the patched execline it would bake the
  # default execline's /nix/store path, forcing a post-generation rewrite. The
  # patched execline has EXECLINE_EXTBINPREFIX="" so those tools are resolved via
  # $PATH instead.
  #
  # s6-linux-init also bakes its own $bin store path (e.g. .../bin/s6-linux-init-telinit)
  # into its binaries via the shared skaware builder's `--enable-absolute-paths`
  # flag. Drop it here too so it relies on $PATH instead.
  patched = (s6-linux-init.override { inherit s6 execline; }).overrideAttrs (oldAttrs: {
    configureFlags = lib.filter (f: f != "--enable-absolute-paths") oldAttrs.configureFlags;
  });
in
symlinkJoin {
  name = "s6-linux-init";
  paths = [
    (lib.getOutput "out" patched)
    (lib.getOutput "bin" patched)
  ];
}
