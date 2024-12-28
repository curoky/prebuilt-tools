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
name=$1

rm -rf tmp
mkdir -p tmp/${name}.${arch}
curl -SL https://github.com/curoky/prebuilt-tools/releases/download/v1.0/${name}.${arch}.tar.gz \
  -o tmp/${name}.${arch}.tar.gz
tar -x --gunzip -f tmp/${name}.${arch}.tar.gz -C tmp/${name}.${arch} --strip-components=1
