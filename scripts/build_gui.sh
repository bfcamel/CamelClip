#!/bin/sh
# Copyright 2026 bfcamel and contributors.
# SPDX-License-Identifier: Apache-2.0
set -eu
cd "$(dirname "$0")/.."
python3 scripts/check_public_content.py
mkdir -p dist
version=$(sed -n 's/^const version = "\([^"]*\)"/\1/p' internal/ui/terminal/cli.go)
case "$(uname -s)" in
 Darwin)
  app="${CAMELCLIP_APP_PATH:-dist/CamelClip.app}"
  arch="${CAMELCLIP_ARCH:-$(uname -m)}"
  case "$arch" in arm64) swiftarch=arm64 ;; amd64|x86_64) arch=amd64; swiftarch=x86_64 ;; *) printf 'Unsupported architecture\n' >&2; exit 1 ;; esac
  mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
  iconwork=$(mktemp -d "${TMPDIR:-/tmp}/camelclip-icon.XXXXXX")
  trap 'rm -rf "$iconwork"' EXIT HUP INT TERM
  mkdir -p "$iconwork/CamelClip.iconset"
  for size in 16 32 128 256 512; do
    sips -z "$size" "$size" internal/ui/gui/web/logo.png --out "$iconwork/CamelClip.iconset/icon_${size}x${size}.png" >/dev/null
    double=$((size * 2))
    sips -z "$double" "$double" internal/ui/gui/web/logo.png --out "$iconwork/CamelClip.iconset/icon_${size}x${size}@2x.png" >/dev/null
  done
  # Package PNG representations directly; no dependency on iconutil's GUI services.
  python3 - "$iconwork/CamelClip.iconset" "$app/Contents/Resources/CamelClip.icns" <<'ICNS'
import pathlib, struct, sys
root = pathlib.Path(sys.argv[1])
representations = [("icp4",16,1),("icp5",32,1),("ic07",128,1),("ic08",256,1),("ic09",512,1),("ic11",16,2),("ic12",32,2),("ic13",128,2),("ic14",256,2),("ic10",512,2)]
chunks = []
for tag, size, scale in representations:
    suffix = "@2x" if scale == 2 else ""
    data = (root / f"icon_{size}x{size}{suffix}.png").read_bytes()
    chunks.append(tag.encode("ascii") + struct.pack(">I", len(data) + 8) + data)
body = b"".join(chunks)
pathlib.Path(sys.argv[2]).write_bytes(b"icns" + struct.pack(">I", len(body) + 8) + body)
ICNS
  CGO_ENABLED=0 GOARCH="$arch" go build -trimpath -o "$app/Contents/MacOS/camelclip-gui" ./cmd/camelclip-gui
  swiftc -O -target "$swiftarch-apple-macos12.0" -module-cache-path "${TMPDIR:-/tmp}/camelclip-swift-cache" scripts/gui/Window.swift -o "$app/Contents/MacOS/CamelClipWindow"
  cat > "$app/Contents/MacOS/CamelClip" <<'LAUNCH'
#!/bin/sh
export PATH="/opt/homebrew/bin:/usr/local/bin:$PATH"
dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
# Finder has no terminal: retain startup errors, including the shared owner lock.
logdir="$HOME/Library/Logs/CamelClip"
mkdir -p "$logdir"
"$dir/camelclip-gui" 2>"$logdir/gui-startup.log"
status=$?
if [ "$status" -ne 0 ]; then
 /usr/bin/osascript - "$logdir/gui-startup.log" <<'APPLESCRIPT'
on run argv
 set detail to read POSIX file (item 1 of argv) as «class utf8»
 display alert "CamelClip" message detail as warning
end run
APPLESCRIPT
fi
exit "$status"
LAUNCH
  chmod +x "$app/Contents/MacOS/CamelClip"
  cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleExecutable</key><string>CamelClip</string>
<key>CFBundleIdentifier</key><string>com.bfcamel.camelclip.gui</string>
<key>CFBundleIconFile</key><string>CamelClip.icns</string>
<key>CFBundleName</key><string>CamelClip</string>
<key>CFBundleDisplayName</key><string>CamelClip</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleVersion</key><string>1</string>
<key>CFBundleShortVersionString</key><string>$version</string>
<key>NSHighResolutionCapable</key><true/>
<key>LSMinimumSystemVersion</key><string>12.0</string>
</dict></plist>
PLIST
  printf 'Built %s\n' "$app"
 ;;
 *) mkdir -p dist/build
    go build -trimpath -o dist/build/camelclip-gui ./cmd/camelclip-gui
    printf 'Built dist/build/camelclip-gui (opens the local UI in your browser)\n' ;;
esac
