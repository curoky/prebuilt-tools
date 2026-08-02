{
  lib,
  stdenv,
  fetchurl,
  openssh_gssapi,
  writeText,
}:

let
  wrapperScriptScp = writeText "wrapper.sh" ''
    #!/usr/bin/env bash

    script_path="$(readlink -f "$0")"
    root=$(cd "$(dirname "$script_path")" && pwd)/..

    exec -a "$0" "$root/bin/_scp" -S $root/bin/ssh "$@"

  '';
  wrapperScriptSshd = writeText "wrapper.sh" ''
    #!/usr/bin/env bash

    script_path="$(readlink -f "$0")"
    root=$(cd "$(dirname "$script_path")" && pwd)/..

    exec -a "$0" "$root/bin/_sshd" \
      -o SshdSessionPath="$root/libexec/sshd-session" \
      -o SshdAuthPath="$root/libexec/sshd-auth" \
      "$@"
  '';
in

openssh_gssapi.overrideAttrs (oldAttrs: {
  configureFlags = (oldAttrs.configureFlags or [ ]) ++ [
    # QEMU user-mode cannot run OpenSSH's seccomp or rlimit pre-authentication
    # sandboxes. The service is restricted to the host loopback by devspace.
    "--with-sandbox=none"
  ];

  postInstall = (oldAttrs.postInstall or "") + ''
    mv $out/bin/scp $out/bin/_scp
    cp ${wrapperScriptScp} $out/bin/scp
    chmod +x $out/bin/scp

    mv $out/bin/sshd $out/bin/_sshd
    cp ${wrapperScriptSshd} $out/bin/sshd
    chmod +x $out/bin/sshd
  '';
})
