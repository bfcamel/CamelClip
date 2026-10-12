#!/bin/sh
# Finder may launch Terminal without Homebrew in PATH.
PATH="/opt/homebrew/bin:/usr/local/bin:$PATH"
export PATH
cd "$(dirname "$0")" || exit 1
case "$(uname -m)" in
  arm64) binary=macdown-macos-arm64 ;;
  x86_64) binary=macdown-macos-amd64 ;;
  *) printf 'Unsupported Mac architecture / Неизвестная архитектура Mac.\n'; exit 1 ;;
esac
result=0
if [ -x "./$binary" ]; then
  "./$binary" || result=$?
elif [ -x "./dist/$binary" ]; then
  "./dist/$binary" || result=$?
elif [ -x ./macdown ]; then
  ./macdown || result=$?
else
  printf 'Build first / Сначала соберите программу: go build -o macdown .\n'
  result=1
fi
printf '\nPress Enter to close / Нажмите Enter, чтобы закрыть.\n'
read -r answer
exit "$result"
