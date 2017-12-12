#!/bin/bash

set -eo pipefail

mkdir -p bin

for arch in ${ALL_GOARCH}; do
  for platform in ${ALL_GOOS}; do
    printf "Building bin/${NAME}.${platform}.${arch}... "
    CGO_ENABLED=0 GOOS=${platform} GOARCH=${arch} ${GOBUILD} -o bin/${NAME}.${platform}.${arch}
    shasum -a 1 bin/${NAME}.${platform}.${arch} | cut -d ' ' -f 1 > bin/${NAME}.${platform}.${arch}.sha1
    cat bin/${NAME}.${platform}.${arch}.sha1
  done
done
