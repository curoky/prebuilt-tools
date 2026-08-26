{
  lib,
  stdenv,
  gpgme,
  lvm2,
  btrfs-progs,
  libapparmor,
  libseccomp,
  libselinux,
  crun,
  runc,
  conmon,
  catatonit,
  coreutils,
  podman,
}:
let
  podman_bin = ./bin;
  podman_conf = ./conf;

  # runc is pulled into podman's helpersBin via the upstream `extraRuntimes`
  # default. Under pkgsStatic the real runc binary is already fully static, but
  # the upstream installPhase runs `wrapProgram` on it, which renames the static
  # binary to `.runc-wrapped` and installs a small *dynamic* launcher named
  # `runc` (it references a /nix musl interpreter + rpath). podman's helpersBin
  # ships that launcher, so the copied `runc` ends up dynamic and depends on
  # /nix, tripping the standalone portability check. (The standalone `.#runc`
  # output avoids this because artifact assembly restores `.runc-wrapped` over
  # the launcher.) Drop the wrapper here and install the static binary
  # directly; the PATH prefix it adds is not needed for the shipped runtime.
  runcStatic = runc.overrideAttrs (_: {
    installPhase = ''
      runHook preInstall
      install -Dm755 runc $out/bin/runc
      installManPage man/*/*.[1-9]
      runHook postInstall
    '';
  });
in
(podman.override {
  conmon = conmon;
  catatonit = catatonit;
  crun = crun;
  runc = runcStatic;
}).overrideAttrs
  (oldAttrs: {
    # Track the podman 5.x release that upstream nixpkgs pins; do not override
    # version/src/vendorHash so we reuse nixpkgs' fetched source and module set.
    propagatedBuildInputs = [ ];
    buildInputs = lib.optionals stdenv.hostPlatform.isLinux [
      btrfs-progs
      gpgme
      libapparmor
      libseccomp
      libselinux
      lvm2
      # systemd
    ];

    nativeInstallCheckInputs = [
      coreutils
    ];

    patches = [
      # we intentionally don't build and install the helper so we shouldn't display messages to users about it
      ./rm-podman-mac-helper-msg.patch

      ./strict-helper-search.patch
    ];

    postPatch = (oldAttrs.postPatch or "") + ''
      # Restrict helpers, conmon, and OCI runtimes such as crun and runc to
      # the sibling libexec/podman directory resolved from $BINDIR at runtime.
      substituteInPlace Makefile \
        --replace-fail '$(HELPER_BINARIES_DIR)' '$$BINDIR/../libexec/podman'
    '';

    postFixup = "";
    postInstall = "
      cp -Lf --remove-destination ${oldAttrs.passthru.helpersBin}/bin/* ${oldAttrs.env.HELPER_BINARIES_DIR}

      mv $out/bin/.podman-wrapped $out/bin/_podman
      rm -f $out/bin/podmansh

      mkdir -p $out/conf
      cp ${podman_bin}/* $out/bin/
      cp ${podman_conf}/* $out/conf/
    ";

    doInstallCheck = true;
    installCheckPhase = ''
      runHook preInstallCheck

      check_dir=$(mktemp -d)
      relocated="$check_dir/relocated"
      host_bin="$check_dir/host-bin"
      state="$check_dir/state"
      mkdir -p "$relocated" "$host_bin" \
        "$state/home"
      cp -aL $out/. "$relocated/"
      chmod -R u+w "$relocated"

      # External copies prove each resolver class cannot escape the relocated
      # sibling directory through config, environment, or PATH.
      cp "$relocated/libexec/podman/conmon" "$host_bin/conmon"
      cp "$relocated/libexec/podman/conmon" "$host_bin/conmonrs"
      cp "$relocated/libexec/podman/crun" "$host_bin/crun"
      cp "$relocated/libexec/podman/pasta" "$host_bin/pasta"
      cp "$relocated/libexec/podman/conmon" "$relocated/libexec/podman/conmonrs"

      cat > "$check_dir/resolver-test.go" <<'EOF'
      package main

      import (
              "fmt"
              "os"
              "path/filepath"

              "go.podman.io/common/pkg/config"
              storage "go.podman.io/storage/types"
      )

      func main() {
              if os.Args[1] == "storage" {
                      opts, err := storage.DefaultStoreOptions()
                      if err != nil {
                              fmt.Fprintln(os.Stderr, err)
                              os.Exit(1)
                      }
                      fmt.Println(filepath.Clean(os.Getenv("PODMAN_DATA_DIR")))
                      fmt.Println(filepath.Clean(os.Getenv("TMPDIR")))
                      fmt.Println(opts.GraphRoot)
                      fmt.Println(opts.RunRoot)
                      return
              }

              cfg := new(config.Config)
              var path string
              var err error
              switch os.Args[1] {
              case "conmon":
                      path, err = cfg.FindConmon()
              case "conmonrs":
                      path, err = cfg.FindConmonRs()
              case "runtime":
                      path, err = config.FindOCIRuntime(os.Args[2], []string{os.Args[3]})
                      if err == nil && path == "" {
                              err = fmt.Errorf("could not find OCI runtime %q", os.Args[2])
                      }
              default:
                      path, err = cfg.FindHelperBinary(os.Args[1], true)
              }
              if err != nil {
                      fmt.Fprintln(os.Stderr, err)
                      os.Exit(1)
              }
              fmt.Println(path)
      }
      EOF
      HOME="$state/home" \
        GOCACHE="$state/go-cache" \
        GOENV=off \
        GOPATH="$state/go" \
        GOMODCACHE="$state/go/pkg/mod" \
        go build -mod=vendor \
          -ldflags '-X go.podman.io/common/pkg/config.additionalHelperBinariesDir=$BINDIR/../libexec/podman' \
          -o "$relocated/bin/resolver-test" \
          "$check_dir/resolver-test.go"

      assert_contains() {
        case "$1" in
          *"$2"*) ;;
          *)
            echo "missing expected Podman output: $2" >&2
            echo "$1" >&2
            return 1
            ;;
        esac
      }

      output=$(PATH="$host_bin" "$relocated/bin/resolver-test" conmon)
      test "$output" = "$relocated/libexec/podman/conmon"

      mv "$relocated/libexec/podman/conmon" "$relocated/libexec/podman/conmon.disabled"
      if output=$(PATH="$host_bin" "$relocated/bin/resolver-test" conmon 2>&1); then
        echo "Podman unexpectedly used conmon outside the sibling directory" >&2
        exit 1
      fi
      assert_contains "$output" "could not find a working conmon binary"
      mv "$relocated/libexec/podman/conmon.disabled" "$relocated/libexec/podman/conmon"

      output=$(PATH="$host_bin" "$relocated/bin/resolver-test" conmonrs)
      test "$output" = "$relocated/libexec/podman/conmonrs"

      mv "$relocated/libexec/podman/conmonrs" "$relocated/libexec/podman/conmonrs.disabled"
      if output=$(PATH="$host_bin" "$relocated/bin/resolver-test" conmonrs 2>&1); then
        echo "Podman unexpectedly found conmonrs outside the sibling directory" >&2
        exit 1
      fi
      assert_contains "$output" "could not find a working conmon binary"

      output=$(
        PATH="$host_bin" \
          "$relocated/bin/resolver-test" runtime crun "$host_bin/crun"
      )
      test "$output" = "$relocated/libexec/podman/crun"

      mv "$relocated/libexec/podman/crun" "$relocated/libexec/podman/crun.disabled"
      if output=$(
        PATH="$host_bin" \
          "$relocated/bin/resolver-test" runtime crun "$host_bin/crun" 2>&1
      ); then
        echo "Podman unexpectedly used an OCI runtime outside the sibling directory" >&2
        exit 1
      fi
      assert_contains "$output" 'could not find OCI runtime "crun"'
      mv "$relocated/libexec/podman/crun.disabled" "$relocated/libexec/podman/crun"

      output=$(
        CONTAINERS_HELPER_BINARY_DIR="$host_bin" \
          PATH="$host_bin" \
          "$relocated/bin/resolver-test" pasta
      )
      test "$output" = "$relocated/libexec/podman/pasta"

      mv "$relocated/libexec/podman/pasta" "$relocated/libexec/podman/pasta.disabled"
      if output=$(
        CONTAINERS_HELPER_BINARY_DIR="$host_bin" \
          PATH="$host_bin" \
          "$relocated/bin/resolver-test" pasta 2>&1
      ); then
        echo "Podman unexpectedly found a helper outside the sibling directory" >&2
        exit 1
      fi
      assert_contains "$output" "could not find \"pasta\" in packaged helper directory"

      install_root="$check_dir/installed podman"
      systemd_unit_dir="$check_dir/systemd"
      systemctl_bin="$check_dir/systemctl-bin"
      systemctl_log="$check_dir/systemctl.log"
      mv "$relocated" "$install_root"
      mkdir -p "$install_root/data" "$systemctl_bin"
      touch "$install_root/data/keep"
      cat > "$systemctl_bin/systemctl" <<'EOF'
      #!/bin/sh
      printf '%s\n' "$*" >>"$SYSTEMCTL_LOG"
      EOF
      chmod +x "$systemctl_bin/systemctl"
      PODMANX_SYSTEMD_UNIT_DIR="$systemd_unit_dir" \
        SYSTEMCTL_LOG="$systemctl_log" \
        PATH="$systemctl_bin:$PATH" \
        "$install_root/bin/install.sh"

      test -e "$install_root/data/keep"
      test -x "$install_root/bin/podman"
      test -x "$install_root/bin/podman-server"
      test -x "$install_root/libexec/podman/crun"
      test "$(cat "$systemctl_log")" = "$(printf '%s\n' \
        "daemon-reload" \
        "enable podmanxd.service" \
        "start podmanxd.service" \
        "status podmanxd.service")"
      grep -F "ExecStart=\"$install_root/bin/podman-server\"" \
        "$systemd_unit_dir/podmanxd.service"
      grep -F "ExecStop=\"$install_root/bin/podman\" stop --all" \
        "$systemd_unit_dir/podmanxd.service"
      if grep -F '@PODMANX_ROOT@' "$systemd_unit_dir/podmanxd.service"; then
        echo "installed systemd unit contains an unresolved path template" >&2
        exit 1
      fi

      mv "$install_root/bin/_podman" "$install_root/bin/_podman.real"
      cat > "$install_root/bin/_podman" <<'EOF'
      #!/bin/sh
      exec "$(dirname "$0")/resolver-test" storage
      EOF
      chmod +x "$install_root/bin/_podman"
      output=$(
        PODMAN_DATA_DIR="$check_dir/external-data" \
          TMPDIR="$check_dir/external-tmp" \
          "$install_root/bin/podman"
      )
      test "$output" = "$(printf '%s\n' \
        "$install_root/data" \
        "$install_root/data/tmpdir" \
        "$install_root/data/graphroot" \
        "$install_root/data/runroot")"
      test -d "$install_root/data/tmpdir"
      grep -F 'export PODMAN_DATA_DIR=$root/../data' "$install_root/bin/podman"
      grep -F 'export PODMAN_DATA_DIR=$root/../data' "$install_root/bin/podman-server"
      grep -F 'graphroot = "$PODMAN_DATA_DIR/graphroot"' "$install_root/conf/storage.conf"
      grep -F 'runroot = "$PODMAN_DATA_DIR/runroot"' "$install_root/conf/storage.conf"
      if grep -F '/opt/podmanx/data' "$install_root/conf/storage.conf"; then
        echo "storage.conf contains a fixed installation path" >&2
        exit 1
      fi
    '';
  })
