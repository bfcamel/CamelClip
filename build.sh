#!/bin/sh
set -eu
cd "$(dirname "$0")"
mkdir -p dist
commit=$(git rev-parse --short=12 HEAD 2>/dev/null || printf development)
go test ./...
go vet ./...
for target in darwin linux; do
  case "$target" in darwin) platform=macos ;; linux) platform=linux ;; esac
  for arch in arm64 amd64; do
    CGO_ENABLED=0 GOOS="$target" GOARCH="$arch" go build -trimpath -ldflags="-s -w -X github.com/bfcamel/video-downloader/internal/ui/terminal.buildCommit=$commit" -o "dist/clipnaro-$platform-$arch" ./cmd/clipnaro
  done
done
cp Start.command install.sh README.md LICENSE NOTICE THIRD_PARTY_LICENSES.txt dist/
printf 'Done: macOS and Linux builds (arm64, amd64) are in dist/\n'
