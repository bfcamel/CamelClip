#!/bin/sh
set -eu
cd "$(dirname "$0")"
mkdir -p dist
go test ./...
go vet ./...
for arch in arm64 amd64; do
  CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "dist/macdown-macos-$arch" .
done
cp Start.command README.md dist/
printf 'Done: Apple Silicon and Intel builds are in dist/\n'
