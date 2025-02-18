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
set -xeuo pipefail

package_name=${1}
output=${2}

if [[ $package_name == "curl" ]]; then
  mv ${output}/bin/curl ${output}/bin/_curl
  cp scripts/wrapper/curl ${output}/bin/curl
elif [[ $package_name == "file" ]]; then
  mv ${output}/bin/file ${output}/bin/_file
  cp scripts/wrapper/file ${output}/bin/file
elif [[ $package_name == "openssh_gssapi" ]]; then
  mv ${output}/bin/scp ${output}/bin/_scp
  cp scripts/wrapper/scp ${output}/bin/scp
elif [[ $package_name == "vim" ]]; then
  mv ${output}/bin/vim ${output}/bin/_vim
  cp scripts/wrapper/vim ${output}/bin/vim
elif [[ $package_name == "wget" ]]; then
  mv ${output}/bin/wget ${output}/bin/_wget
  cp scripts/wrapper/wget ${output}/bin/wget
elif [[ $package_name == "zsh" ]]; then
  mv ${output}/bin/zsh ${output}/bin/_zsh
  cp scripts/wrapper/zsh ${output}/bin/zsh
fi
