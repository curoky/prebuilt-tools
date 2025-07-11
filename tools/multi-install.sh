#!/usr/bin/env bash
# Copyright (c) 2024-2025 curoky(cccuroky@gmail.com).
#
# This file is part of prebuilt-tools.
# See https://github.com/curoky/prebuilt-tools for further info.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

# layout
# install_path
#   - profile/default
#   - store
#     - curl
#     - wget
#   - downloads

function link() {
  local src_dir=$1
  local dst_dir=$2
  find "$src_dir" \( -type f -o -type l \) | while read -r file; do
    rel_path="${file#$src_dir/}"
    dest_file="$dst_dir/$rel_path"
    mkdir -p "$(dirname "$dest_file")"
    if [[ -L $dest_file ]] || [[ -f $dest_file ]]; then
      rm "$dest_file"
    fi
    ln -s -r "$file" "$dest_file"
  done
}

arch=$(echo $(uname -s)-$(uname -m) | tr '[:upper:]' '[:lower:]') # linux_amd64/darwin_arm64

# download_path=tmp
install_path=tmp
is_link=false
link_path=-

while getopts "i:n:a:lp:" opt; do
  case "$opt" in
    n)
      name="$OPTARG"
      ;;
    i)
      install_path="$OPTARG"
      ;;
    a)
      arch="$OPTARG"
      ;;
    l)
      is_link=true
      ;;
    p)
      link_path="$OPTARG"
      ;;
    \?)
      echo "Usage: $0 [-n name] [-i install_path] [-d download_path] [-a arch]"
      exit 1
      ;;
  esac
done

download_path=$install_path/downloads
store_path=$install_path/store
profile_path=$install_path/profile
profile=default

mkdir -p $download_path $store_path/$name $profile_path/$profile
curl -sSL https://github.com/curoky/static-binaries/releases/download/v1.0/${name}.${arch}.tar.gz \
  -o $download_path/${name}.tar.gz
tar -x --gunzip -f $download_path/${name}.tar.gz -C $store_path/${name} --strip-components=1

if [[ $is_link == true ]]; then
  if [[ $link_path == "-" ]]; then
    link $store_path/$name $profile_path/$profile
  else
    link $store_path/$name $link_path
  fi
fi
