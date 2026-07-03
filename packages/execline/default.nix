{
  lib,
  execline,
}:

# execline programs (e.g. backtick, forbacktickx, if) exec into sibling
# execline tools (multisubstitute, pipeline, importas, exit, ...) via the
# EXECLINE_BINPREFIX macro. The shared skaware builder's `--enable-absolute-paths`
# flag makes that prefix an absolute /nix/store path. Drop it so the binaries
# rely on $PATH instead of an absolute store path.
execline.overrideAttrs (oldAttrs: {
  configureFlags = lib.filter (f: f != "--enable-absolute-paths") oldAttrs.configureFlags;

  # EXECLINE_SHEBANGPREFIX is always set to the absolute $shebangdir (hardcoded
  # by execline's configure regardless of --enable-absolute-paths). It is baked
  # into <execline/config.h> and used by s6-rc-compile to emit "#!<prefix>execlineb"
  # shebang lines. A shebang must be an absolute path (the kernel does not search
  # $PATH), so point it at /usr/bin/env and let env resolve execlineb via $PATH.
  #
  # Use "-S" so env splits the interpreter and its options: s6-rc-compile emits
  # "#!<prefix>execlineb -P", and Debian 10's /usr/bin/env does not split the
  # shebang argument, so without -S it looks for a program literally named
  # "execlineb -P" and fails. Baking -S here means the generated run scripts are
  # correct at build time (no post-compile shebang rewrite needed).
  postConfigure = ''
    sed -i 's|^#define EXECLINE_SHEBANGPREFIX .*|#define EXECLINE_SHEBANGPREFIX "/usr/bin/env -S "|' \
      src/include/execline/config.h
  '';
})
