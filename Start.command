#!/bin/sh
# Finder may launch Terminal without Homebrew in PATH.
PATH="/opt/homebrew/bin:/usr/local/bin:$PATH"
export PATH
cd "$(dirname "$0")" || exit 1
case "$(uname -m)" in
  arm64) binary=camelclip-macos-arm64 ;;
  x86_64) binary=camelclip-macos-amd64 ;;
  *) printf 'Unsupported Mac architecture.\n'; exit 1 ;;
esac
result=0
if [ -x "./$binary" ]; then
  "./$binary" || result=$?
elif [ -x "./dist/terminal/$binary" ]; then
  "./dist/terminal/$binary" || result=$?
elif [ -x "./dist/build/$binary" ]; then
  "./dist/build/$binary" tui || result=$?
elif [ -x "./dist/$binary" ]; then
  "./dist/$binary" || result=$?
elif [ -x ./camelclip ]; then
  ./camelclip || result=$?
else
  printf 'Build first: go build -o camelclip ./cmd/camelclip\n'
  result=1
fi
printf '\nPress Enter to close.\n'
read -r answer
exit "$result"
