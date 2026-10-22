#!/bin/sh
# Copyright 2026 bfcamel and contributors.
# SPDX-License-Identifier: Apache-2.0
set -eu
cd "$(dirname "$0")/.."
mkdir -p dist
case "$(uname -s)" in
 Darwin)
  app="dist/CamelClip.app"
  mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
  go build -trimpath -o "$app/Contents/MacOS/camelclip-gui" ./cmd/camelclip-gui
  swiftc -O -module-cache-path "${TMPDIR:-/tmp}/camelclip-swift-cache" scripts/gui/Window.swift -o "$app/Contents/MacOS/CamelClipWindow"
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
  cat > "$app/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleExecutable</key><string>CamelClip</string>
<key>CFBundleIdentifier</key><string>com.bfcamel.camelclip.gui</string>
<key>CFBundleName</key><string>CamelClip</string>
<key>CFBundleDisplayName</key><string>CamelClip</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleVersion</key><string>1</string>
<key>CFBundleShortVersionString</key><string>0.12.1</string>
<key>NSHighResolutionCapable</key><true/>
<key>LSMinimumSystemVersion</key><string>12.0</string>
</dict></plist>
PLIST
  printf 'Built %s\n' "$app"
 ;;
 *) go build -trimpath -o dist/camelclip-gui ./cmd/camelclip-gui
    printf 'Built dist/camelclip-gui (opens the local UI in your browser)\n' ;;
esac
