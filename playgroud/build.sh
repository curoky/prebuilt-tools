#!/usr/bin/env bash
set -xeuo pipefail

version=$1
package_name=$2

pod build . \
  --file Dockerfile.$version \
  --network=host \
  --build-arg PACKAGE_NAME=$package_name \
  --tag curoky/dotbox:prebuilt-tools
