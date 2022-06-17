#!/usr/bin/env bash

set -ueo pipefail

root_d="$(cd -P -- "$(dirname -- "$0")" && pwd -P)"
bin_d="${root_d}/bin"

latest_tag="$(git tag -l | sort -V | tail -r | head -n 1)"

echo "Latest tag is ${latest_tag}"
echo "---"
echo "binaries:"

versions=(
cf-conduit.darwin.amd64
cf-conduit.darwin.arm64
cf-conduit.windows.386
cf-conduit.windows.amd64
cf-conduit.windows.arm64
cf-conduit.linux.386
cf-conduit.linux.amd64
cf-conduit.linux.arm64
)

for v in "${versions[@]}"; do
  url="https://github.com/alphagov/paas-cf-conduit/releases/download/${latest_tag}/${v}"
  checksum="$(curl -sfL "${url}.sha1")"

  case $v in
  cf-conduit.darwin*)
    platform="osx"
    ;;
  cf-conduit.windows.386)
    platform="win32"
    ;;
  cf-conduit.windows.amd64)
    platform="win64"
    ;;
  cf-conduit.linux.386)
    platform="linux32"
    ;;
  cf-conduit.linux.amd64)
    platform="linux64"
    ;;
  esac

cat <<EOF
- checksum: ${checksum}
  platform: ${platform}
  url: ${url}
EOF

done
