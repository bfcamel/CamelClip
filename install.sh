#!/bin/sh
set -eu
cd "$(dirname "$0")"
bin_dir="$HOME/.local/bin"
add_path=false
force=false
while [ "$#" -gt 0 ]; do
  case "$1" in
    --bin-dir) [ "$#" -ge 2 ] || { printf 'Missing --bin-dir value\n' >&2; exit 2; }; bin_dir="$2"; shift 2 ;;
    --add-to-path) add_path=true; shift ;;
    --force) force=true; shift ;;
    -h|--help) printf 'Usage: sh install.sh [--bin-dir DIR] [--add-to-path] [--force]\nDefault: ~/.local/bin/macdown. --add-to-path updates ~/.zprofile.\n'; exit 0 ;;
    *) printf 'Unknown option: %s\n' "$1" >&2; exit 2 ;;
  esac
done
case "$bin_dir" in /*) ;; *) printf 'Use an absolute --bin-dir path.\n' >&2; exit 2 ;; esac
if [ "$add_path" = true ] && [ "$bin_dir" != "$HOME/.local/bin" ]; then
  printf '%s\n' '--add-to-path only supports the default ~/.local/bin directory.' >&2
  exit 2
fi
if [ -e "$bin_dir/macdown" ] && [ "$force" != true ]; then
  printf 'Already exists: %s/macdown. Use --force to replace it.\n' "$bin_dir" >&2
  exit 1
fi
scratch=$(mktemp -d "${TMPDIR:-/tmp}/macdown-install.XXXXXX")
trap 'rm -rf "$scratch"' EXIT HUP INT TERM
if [ -f go.mod ] && command -v go >/dev/null 2>&1; then
  go build -trimpath -ldflags='-s -w' -o "$scratch/macdown" ./cmd/macdown
else
  case "$(uname -m)" in arm64) arch=arm64 ;; x86_64) arch=amd64 ;; *) printf 'Unsupported architecture\n' >&2; exit 1 ;; esac
  binary="macdown-macos-$arch"
  if [ -f "$binary" ]; then cp "$binary" "$scratch/macdown"
  elif [ -f "dist/$binary" ]; then cp "dist/$binary" "$scratch/macdown"
  else printf 'Install Go to build from source, or use a compiled release.\n' >&2; exit 1
  fi
fi
mkdir -p "$bin_dir"
install -m 755 "$scratch/macdown" "$bin_dir/macdown"
if [ "$add_path" = true ]; then
  path_line='export PATH="$HOME/.local/bin:$PATH" # MacDown CLI'
  if ! grep -Fqx "$path_line" "$HOME/.zprofile" 2>/dev/null; then
    printf '\n%s\n' "$path_line" >> "$HOME/.zprofile"
  fi
fi
printf 'Installed: %s/macdown\n' "$bin_dir"
if [ "$add_path" = true ]; then
  printf 'Open a new Terminal window, then run: macdown doctor\n'
else
  printf 'Ensure %s is in PATH, then run: macdown doctor\n' "$bin_dir"
fi
