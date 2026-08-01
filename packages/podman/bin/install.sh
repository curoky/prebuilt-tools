#!/usr/bin/env bash
set -xeuo pipefail

script_path="$(readlink -f "$0")"
root=$(cd "$(dirname "$script_path")/.." && pwd)
systemd_unit_dir=${PODMANX_SYSTEMD_UNIT_DIR:-/etc/systemd/system}

mkdir -p "$root/data" "$systemd_unit_dir"
escaped_root=$(printf '%s' "$root" | sed 's/[\\&|]/\\&/g')
sed "s|@PODMANX_ROOT@|$escaped_root|g" \
  "$root/conf/podmanxd.service" >"$systemd_unit_dir/podmanxd.service"

systemctl daemon-reload
systemctl enable podmanxd.service
systemctl start podmanxd.service
systemctl status podmanxd.service

# echo 'nvidia-ctk cdi generate --output=/etc/cdi/nvidia.yaml'
