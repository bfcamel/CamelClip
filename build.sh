#!/bin/sh
set -eu
cd "$(dirname "$0")"
python3 scripts/check_public_content.py
mkdir -p dist/terminal
commit=$(git rev-parse --short=12 HEAD 2>/dev/null || printf development)
go test ./...
go vet ./...
for target in darwin linux; do
  case "$target" in darwin) platform=macos ;; linux) platform=linux ;; esac
  for arch in arm64 amd64; do
    CGO_ENABLED=0 GOOS="$target" GOARCH="$arch" go build -trimpath -ldflags="-s -w -X github.com/bfcamel/CamelClip/internal/ui/terminal.buildCommit=$commit" -o "dist/terminal/camelclip-$platform-$arch" ./cmd/camelclip
  done
done
cp Start.command install.sh README.md LICENSE NOTICE THIRD_PARTY_LICENSES.txt dist/terminal/
printf 'Done: macOS and Linux builds (arm64, amd64) are in dist/terminal/\n'
