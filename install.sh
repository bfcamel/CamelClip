#!/bin/sh
set -eu
cd "$(dirname "$0")"
bin_dir="$HOME/.local/bin"
case "$(uname -s)" in
  Darwin) platform=macos; profile="$HOME/.zprofile" ;;
  Linux)
    platform=linux
    case "${SHELL:-}" in
      */zsh) profile="$HOME/.zprofile" ;;
      */bash) profile="$HOME/.bashrc" ;;
      */sh|*/dash|"") profile="$HOME/.profile" ;;
      *) profile= ;;
    esac ;;
  *) printf 'Supported systems: macOS and Linux.\n' >&2; exit 1 ;;
esac
add_path=false
force=false
while [ "$#" -gt 0 ]; do
  case "$1" in
    --bin-dir) [ "$#" -ge 2 ] || { printf 'Missing --bin-dir value\n' >&2; exit 2; }; bin_dir="$2"; shift 2 ;;
    --add-to-path) add_path=true; shift ;;
    --force) force=true; shift ;;
    -h|--help) printf 'Usage: sh install.sh [--bin-dir DIR] [--add-to-path] [--force]\nDefault: ~/.local/bin/camelclip. --add-to-path updates ~/.zprofile (zsh) or ~/.bashrc (Linux bash) or ~/.profile (sh).\n'; exit 0 ;;
    *) printf 'Unknown option: %s\n' "$1" >&2; exit 2 ;;
  esac
done
case "$bin_dir" in /*) ;; *) printf 'Use an absolute --bin-dir path.\n' >&2; exit 2 ;; esac
if [ "$add_path" = true ] && [ "$bin_dir" != "$HOME/.local/bin" ]; then
  printf '%s\n' '--add-to-path only supports the default ~/.local/bin directory.' >&2
  exit 2
fi
if [ -e "$bin_dir/camelclip" ] && [ "$force" != true ]; then
  printf 'Already exists: %s/camelclip. Use --force to replace it.\n' "$bin_dir" >&2
  exit 1
fi
if [ "$add_path" = true ] && [ -z "$profile" ]; then
  printf 'Add ~/.local/bin to your shell PATH manually, then run without --add-to-path.\n' >&2
  exit 1
fi
scratch=$(mktemp -d "${TMPDIR:-/tmp}/camelclip-install.XXXXXX")
trap 'rm -rf "$scratch"' EXIT HUP INT TERM
if [ -f go.mod ] && command -v go >/dev/null 2>&1; then
  go build -trimpath -ldflags='-s -w' -o "$scratch/camelclip" ./cmd/camelclip
else
  case "$(uname -m)" in arm64|aarch64) arch=arm64 ;; x86_64|amd64) arch=amd64 ;; *) printf 'Unsupported architecture\n' >&2; exit 1 ;; esac
  binary="camelclip-$platform-$arch"
  if [ -f "$binary" ]; then cp "$binary" "$scratch/camelclip"
  elif [ -f "dist/terminal/$binary" ]; then cp "dist/terminal/$binary" "$scratch/camelclip"
  elif [ -f "dist/$binary" ]; then cp "dist/$binary" "$scratch/camelclip"
  elif [ -f camelclip ]; then cp camelclip "$scratch/camelclip"
  else printf 'Install Go to build from source, or use a compiled release.\n' >&2; exit 1
  fi
fi
mkdir -p "$bin_dir"
install -m 755 "$scratch/camelclip" "$bin_dir/camelclip"
if [ "$add_path" = true ]; then
  path_line='export PATH="$HOME/.local/bin:$PATH" # CamelClip CLI'
  if ! grep -Fqx "$path_line" "$profile" 2>/dev/null; then
    printf '\n%s\n' "$path_line" >> "$profile"
  fi
fi
printf 'Installed: %s/camelclip\n' "$bin_dir"
if [ "$add_path" = true ]; then
  printf 'Open a new Terminal window, then run: camelclip doctor\n'
else
  printf 'Ensure %s is in PATH, then run: camelclip doctor\n' "$bin_dir"
fi
