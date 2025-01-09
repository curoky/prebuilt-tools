#!/usr/bin/env bash
# Copyright (c) 2018-2024 curoky(cccuroky@gmail.com).
#
# This file is part of dotbox.
# See https://github.com/curoky/dotbox for further info.
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

set -xeuo pipefail

arch=$(echo $(uname -s)_$(uname -m) | tr '[:upper:]' '[:lower:]') # linux_amd64/darwin_arm64

download_path=tmp
install_path=tmp

while getopts "i:d:n:a:" opt; do
  case "$opt" in
    n)
      name="$OPTARG"
      ;;
    i)
      install_path="$OPTARG"
      ;;
    d)
      download_path="$OPTARG"
      ;;
    a)
      arch="$OPTARG"
      ;;
    \?)
      echo "Usage: $0 [-n name] [-i install_path] [-d download_path] [-a arch]"
      exit 1
      ;;
  esac
done

mkdir -p $install_path
mkdir -p $download_path
curl -sSL https://github.com/curoky/prebuilt-tools/releases/download/v1.0/${name}.${arch}.tar.gz \
  -o $download_path/${name}.tar.gz
tar -x --gunzip -f $download_path/${name}.tar.gz -C $install_path --strip-components=1
