{
  lib,
  stdenv,
  fetchurl,
  writeText,
  python3Packages,
  python314,
}:

let
  pythonVersion = "3.14";
  sitePackages = "lib/python${pythonVersion}/site-packages";
  pythonDependencies = with python3Packages; [
    jinja2
    markupsafe
    partftpy
    pyasyncore
    pyasynchat
    pyftpdlib
  ];
  wrapperScript = writeText "copyparty-wrapper.sh" ''
    #!/usr/bin/env bash

    script_path="$(readlink -f "$0")"
    root=$(cd "$(dirname "$script_path")" && pwd)/..
    store=$root/..

    export PYTHONHOME=$store/python314
    export PYTHONDONTWRITEBYTECODE=1
    export PYTHONNOUSERSITE=1
    export PYTHONSAFEPATH=1
    export PYTHONPATH=$PYTHONHOME/lib/python${pythonVersion}
    export PYTHONPATH=$PYTHONPATH:$PYTHONHOME/lib/python${pythonVersion}/site-packages
    export PYTHONPATH=$PYTHONPATH:$PYTHONHOME/lib/python${pythonVersion}/lib-dynload
    export PYTHONPATH=$PYTHONPATH:$root/${sitePackages}

    export PRTY_NO_ARGON2=1
    export PRTY_NO_CFSSL=1
    export PRTY_NO_CTYPES=1
    export PRTY_NO_DCRAW=1
    export PRTY_NO_FFMPEG=1
    export PRTY_NO_FFPROBE=1
    export PRTY_NO_IFADDR=1
    export PRTY_NO_MAGIC=1
    export PRTY_NO_MUTAGEN=1
    export PRTY_NO_PARAMIKO=1
    export PRTY_NO_PIL=1
    export PRTY_NO_PSUTIL=1
    export PRTY_NO_RAW=1
    export PRTY_NO_VIPS=1

    exec "$PYTHONHOME/bin/python${pythonVersion}" -m copyparty "$@"
  '';
in

stdenv.mkDerivation rec {
  pname = "copyparty";
  version = "1.20.20";

  src = fetchurl {
    url = "https://github.com/9001/copyparty/releases/download/v${version}/copyparty-${version}.tar.gz";
    hash = "sha256-ox+eSkPlZgh2XhCDUxQHhNUBgcN+cF6c6O2fOJn1ffo=";
  };

  dontBuild = true;
  dontPatchShebangs = true;

  installPhase = ''
    runHook preInstall

    mkdir -p "$out/${sitePackages}"
    cp -RL copyparty "$out/${sitePackages}/"

    for dependency in ${lib.concatStringsSep " " pythonDependencies}; do
      dependency_site="$dependency/${python3Packages.python.sitePackages}"
      for module in "$dependency_site"/*; do
        case "$(basename "$module")" in
          *.dist-info | *.egg-info | __pycache__)
            continue
            ;;
        esac
        cp -R "$module" "$out/${sitePackages}/"
      done
    done

    chmod -R u+w "$out/${sitePackages}"
    find "$out/${sitePackages}" -type d -name __pycache__ -prune -exec rm -rf {} +
    find "$out/${sitePackages}" -type f \( -name '*.pyc' -o -name '*.pyo' -o -name '*.so' \) -delete

    mkdir -p "$out/bin"
    cp ${wrapperScript} "$out/bin/copyparty"
    chmod +x "$out/bin/copyparty"

    runHook postInstall
  '';

  doInstallCheck = true;
  nativeInstallCheckInputs = [
    python3Packages.python
  ];
  installCheckPhase = ''
    runHook preInstallCheck

    test ! -e "$out/${sitePackages}/markupsafe/_speedups.so"
    test -z "$(find "$out" -type f \( -name '*.so' -o -name '*.dylib' \) -print -quit)"
    ! grep -R -a -l -F /nix/store "$out"

    export PYTHONDONTWRITEBYTECODE=1
    export PYTHONNOUSERSITE=1
    export PYTHONSAFEPATH=1
    export PYTHONPATH="$out/${sitePackages}"
    ${python3Packages.python.interpreter} -S - <<'PY'
    import importlib.util
    import jinja2
    import partftpy
    import pyftpdlib

    assert jinja2.__version__ == "3.1.6"
    assert partftpy.__version__ == "0.4.0"
    assert pyftpdlib.__ver__ == "2.2.0"
    assert importlib.util.find_spec("markupsafe._speedups") is None
    PY

    ${python3Packages.python.interpreter} -S -m copyparty --version \
      | grep -F "copyparty v${version}"

    check_store=$TMPDIR/copyparty-store
    check_config=$TMPDIR/copyparty-config
    check_runtime=$TMPDIR/copyparty-runtime
    check_volume=$TMPDIR/copyparty-volume
    mkdir -p \
      "$check_store/copyparty/bin" \
      "$check_config" \
      "$check_runtime" \
      "$check_volume"
    cp "$out/bin/copyparty" "$check_store/copyparty/bin/"
    ln -s "$out/lib" "$check_store/copyparty/lib"
    ln -s ${python314} "$check_store/python314"

    export XDG_CONFIG_HOME=$check_config
    export TMPDIR=$check_runtime
    unset PYTHONPATH

    ${python3Packages.python.interpreter} - \
      "$check_store/copyparty/bin/copyparty" \
      -i 127.0.0.1 \
      -p 40130 \
      --ftp 40131 \
      -v "$check_volume:/share:r" \
      -e2d \
      --no-ansi \
      >"$check_runtime/server.log" 2>&1 <<'PY' &
    import os
    import sys

    os.setsid()
    os.execv(sys.argv[1], sys.argv[1:])
    PY
    server_pid=$!

    cleanup_server() {
      kill -- "-$server_pid" 2>/dev/null || true
      wait "$server_pid" 2>/dev/null || true
    }
    trap cleanup_server EXIT

    ready=
    for _ in $(seq 1 200); do
      if ${python3Packages.python.interpreter} - >/dev/null 2>&1 <<'PY'
    import ftplib
    import urllib.request

    with urllib.request.urlopen("http://127.0.0.1:40130/share/", timeout=1) as response:
        assert response.status == 200

    with ftplib.FTP() as ftp:
        ftp.connect("127.0.0.1", 40131, timeout=1)
        ftp.login()
        ftp.nlst()
    PY
      then
        ready=1
        break
      fi
      sleep 0.1
    done

    if [ -z "$ready" ] || [ ! -f "$check_volume/.hist/up2k.db" ]; then
      cat "$check_runtime/server.log"
      exit 1
    fi

    cleanup_server
    trap - EXIT

    check_runtime_mp=$TMPDIR/copyparty-runtime-mp
    mkdir -p "$check_runtime_mp"
    export TMPDIR=$check_runtime_mp

    ${python3Packages.python.interpreter} - \
      "$check_store/copyparty/bin/copyparty" \
      -j2 \
      -i 127.0.0.1 \
      -p 40132 \
      -v "$check_volume:/share:r" \
      --no-ansi \
      >"$check_runtime_mp/server.log" 2>&1 <<'PY' &
    import os
    import sys

    os.setsid()
    os.execv(sys.argv[1], sys.argv[1:])
    PY
    server_pid=$!
    trap cleanup_server EXIT

    ready=
    for _ in $(seq 1 200); do
      if ${python3Packages.python.interpreter} - >/dev/null 2>&1 <<'PY'
    import urllib.request

    with urllib.request.urlopen("http://127.0.0.1:40132/share/", timeout=1) as response:
        assert response.status == 200
    PY
      then
        ready=1
        break
      fi
      sleep 0.1
    done

    if [ -z "$ready" ]; then
      cat "$check_runtime_mp/server.log"
      exit 1
    fi
    grep -F "booting 2 subprocesses" "$check_runtime_mp/server.log"

    cleanup_server
    trap - EXIT

    runHook postInstallCheck
  '';

  meta = {
    description = "Portable file server";
    homepage = "https://github.com/9001/copyparty";
    license = with lib.licenses; [
      asl20
      bsd3
      cc-by-40
      isc
      mit
      mpl20
      ofl
      psfl
    ];
    mainProgram = "copyparty";
    platforms = lib.platforms.linux;
  };
}
