#!/bin/sh
# Copyright 2026 bfcamel and contributors.
# SPDX-License-Identifier: Apache-2.0
set -eu
cd "$(dirname "$0")/.."
python3 scripts/check_public_content.py
mkdir -p dist/build
# Optional build filter: ./scripts/build_installers.sh linux arm64
systems="${1:-darwin linux}"
architectures="${2:-arm64 amd64}"
for system in $systems; do
 case "$system" in darwin) label=macos ;; linux) label=linux ;; *) echo 'Supported targets: darwin linux' >&2; exit 2 ;; esac
 for arch in $architectures; do
  case "$arch" in arm64|amd64) ;; *) echo 'Supported architectures: arm64 amd64' >&2; exit 2 ;; esac
  for name in camelclip camelclip-gui camelclip-setup; do
   CGO_ENABLED=0 GOOS="$system" GOARCH="$arch" go build -trimpath -ldflags="-s -w -X github.com/bfcamel/CamelClip/internal/ui/terminal.buildCommit=${RELEASE_COMMIT:-development}" -o "dist/build/$name-$label-$arch" "./cmd/$name"
  done
  if [ "$system" = darwin ]; then
   [ "$(uname -s)" = Darwin ] || { echo 'Build macOS app and native packages on macOS.' >&2; exit 1; }
   CAMELCLIP_ARCH="$arch" CAMELCLIP_APP_PATH="dist/build/CamelClip-macos-$arch.app" sh scripts/build_gui.sh
   python3 scripts/package_setup.py --os "$system" --arch "$arch" --pkg
  else
   python3 scripts/package_setup.py --os "$system" --arch "$arch"
  fi
 done
done
printf 'Installers and SHA256SUMS: dist/installers/<version>/\n'
