#!/bin/bash

set -eo pipefail

mkdir -p bin

for platform in ${ALL_GOOS}; do
  if [[ ${platform} = darwin ]]; then
    for arch in amd64 arm64; do
        printf "Building bin/${NAME}.${platform}.${arch}... "
        CGO_ENABLED=0 GOOS=${platform} GOARCH=${arch} ${GOBUILD} -o bin/${NAME}.${platform}.${arch}
        shasum -a 1 bin/${NAME}.${platform}.${arch} | cut -d ' ' -f 1 > bin/${NAME}.${platform}.${arch}.sha1
        cat bin/${NAME}.${platform}.${arch}.sha1
      done
  else
    for arch in ${ALL_GOARCH}; do
      printf "Building bin/${NAME}.${platform}.${arch}... "
      CGO_ENABLED=0 GOOS=${platform} GOARCH=${arch} ${GOBUILD} -o bin/${NAME}.${platform}.${arch}
      shasum -a 1 bin/${NAME}.${platform}.${arch} | cut -d ' ' -f 1 > bin/${NAME}.${platform}.${arch}.sha1
      cat bin/${NAME}.${platform}.${arch}.sha1
    done
  fi
done
