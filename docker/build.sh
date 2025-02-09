#!/usr/bin/env bash

set -xeuo pipefail

PACKAGE_NAME=${1:-"curl"}
ARCH_NAME=${2:-"linux-x86_64"}

docker buildx build . \
  --file docker/Dockerfile \
  --network=host \
  --build-arg PACKAGE_NAME=$PACKAGE_NAME \
  --build-arg ARCH_NAME=$ARCH_NAME \
  --tag curoky/dotbox:prebuilt-tools

id=$(docker create curoky/dotbox:prebuilt-tools)
docker cp $id:/tmp/${PACKAGE_NAME}.${ARCH_NAME}.tar.gz - >${PACKAGE_NAME}.${ARCH_NAME}.tar.gz.tar
docker rm -v $id
tar -xvf ${PACKAGE_NAME}.${ARCH_NAME}.tar.gz.tar
rm -rf ${PACKAGE_NAME}.${ARCH_NAME}.tar.gz.tar
