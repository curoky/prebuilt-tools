{
  libarchive,
  openssl,
}:

let
  portableOpenSSL = openssl.overrideAttrs (oldAttrs: {
    postPatch = oldAttrs.postPatch + ''
      substituteInPlace Configurations/unix-Makefile.tmpl \
        --replace-fail 'OPENSSLDIR="\"$(OPENSSLDIR)\""' 'OPENSSLDIR="\"/etc/ssl\""' \
        --replace-fail 'ENGINESDIR="\"$(ENGINESDIR)\""' 'ENGINESDIR="\"/usr/lib/engines\""' \
        --replace-fail 'MODULESDIR="\"$(MODULESDIR)\""' 'MODULESDIR="\"/usr/lib/ossl-modules\""'
    '';
  });
in
(libarchive.override {
  openssl = portableOpenSSL;
  xarSupport = false;
}).overrideAttrs
  (oldAttrs: {
    postFixup = (oldAttrs.postFixup or "") + ''
      for binary in "$out"/bin/*; do
        if grep -aqF /nix/store "$binary"; then
          echo "$binary contains a Nix store path" >&2
          exit 1
        fi
      done
    '';
  })
