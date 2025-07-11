{ lib, stdenv, fetchurl, gitMinimal, nghttp2, libpsl, gettext}:

let
  mygettext = gettext.overrideAttrs (oldAttrs: rec {
    configureFlags = oldAttrs.configureFlags ++ [
      "--with-included-gettext"
      "--with-included-libintl"
    ];
});
in
gitMinimal.overrideAttrs (oldAttrs: rec {
  buildInputs = oldAttrs.buildInputs ++ [ nghttp2 libpsl mygettext ];
  #configureFlags = oldAttrs.configureFlags or [] ++ [ "LDFLAGS='-lnghttp2 -lpsl'" ];
  configureFlags = oldAttrs.configureFlags or [] ++ [ "--with-gettext=${mygettext}" ];
  NIX_LDFLAGS = ["-lnghttp2" "-lpsl" "-lssl" "-lcrypto" "-lssh2" "-lidn2" "-lzstd" "-lz" "-lunistring" "-lintl"];
  doCheck = false;
makeFlags =
    [
      "prefix=\${out}"
      "ZLIB_NG=1"
    ]
    # Git does not allow setting a shell separately for building and run-time.
    # Therefore lets leave it at the default /bin/sh when cross-compiling
    ++ lib.optional (stdenv.buildPlatform == stdenv.hostPlatform) "SHELL_PATH=${stdenv.shell}"
    #++ (if perlSupport then [ "PERL_PATH=${perlPackages.perl}/bin/perl" ] else [ "NO_PERL=1" ])
    #++ (if pythonSupport then [ "PYTHON_PATH=${python3}/bin/python" ] else [ "NO_PYTHON=1" ])
    ++ lib.optionals stdenv.hostPlatform.isSunOS [
      "INSTALL=install"
      "NO_INET_NTOP="
      "NO_INET_PTON="
    ]
    ++ (if stdenv.hostPlatform.isDarwin then [ "NO_APPLE_COMMON_CRYPTO=1" ] else [ "sysconfdir=/etc" ])
   # ++ lib.optionals stdenv.hostPlatform.isMusl [
    #  "NO_SYS_POLL_H=1"
    #  "NO_GETTEXT=YesPlease"
    #]
    #++ lib.optional withpcre2 "USE_LIBPCRE2=1"
    #++ lib.optional (!nlsSupport) "NO_GETTEXT=1"
    # git-gui refuses to start with the version of tk distributed with
    # macOS Catalina. We can prevent git from building the .app bundle
    # by specifying an invalid tk framework. The postInstall step will
    # then ensure that git-gui uses tcl/tk from nixpkgs, which is an
    # acceptable version.
    #
    # See https://github.com/Homebrew/homebrew-core/commit/dfa3ccf1e7d3901e371b5140b935839ba9d8b706
    ++ lib.optional stdenv.hostPlatform.isDarwin "TKFRAMEWORK=/nonexistent";



  #buildPhase = ''
  #  echo "==> Skipping buildPhase intentionally"
  #'';
  #installPhase = ''
  #  echo "==> Skipping installPhase intentionally"
  #'';
  checkPhase = "echo skipping tests";

  patchPhase = ''
    echo "Patching usage.c: rename error() to git_error()"
    substituteInPlace usage.c \
      --replace " error(" " git_error("
    substituteInPlace usage.c \
      --replace "void error(" "void git_error("

    #substituteInPlace configure.ac \
    #  --replace 'LIBC_CONTAINS_LIBINTL=YesPlease' 'LIBC_CONTAINS_LIBINTL='
    #substituteInPlace configure.ac \
    #  --replace 'NO_GETTEXT=YesPlease' 'NO_GETTEXT='
    #substituteInPlace configure \
    #  --replace 'LIBC_CONTAINS_LIBINTL=YesPlease' 'LIBC_CONTAINS_LIBINTL='
    #substituteInPlace configure \
    #  --replace 'NO_GETTEXT=YesPlease' 'NO_GETTEXT='

    #substituteInPlace configure.ac \
    #  --replace 'GIT_CONF_SUBST([LIBC_CONTAINS_LIBINTL])' '#23'
    #substituteInPlace configure.ac \
    #  --replace 'GIT_CONF_SUBST([NO_GETTEXT])' '#23'
  '';
})
