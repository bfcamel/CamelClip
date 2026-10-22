# Graphical interface

The GUI is an additional frontend for the same CamelClip workspace. TUI and CLI remain available. It uses the same settings, queue/history, library, encrypted S3 vault, downloader and verification pipeline.

## macOS

The locally built application is `dist/CamelClip.app`. Double-click it in Finder, or run from the project directory:

```sh
open /Applications/CamelClip/dist/CamelClip.app
```

Close any running CamelClip TUI/GUI first. The existing exclusive database lock prevents a second owner; startup shows the error in a macOS alert. Startup details are retained in `~/Library/Logs/CamelClip/gui-startup.log`.

The first run asks for language, output directory and download preferences. You can run offline diagnostics before saving. Existing TUI settings bypass setup. Dependencies (`yt-dlp`, `ffmpeg`, `ffprobe`, and Deno when needed by the source) must be installed. Finder launch includes Homebrew's `/opt/homebrew/bin` and `/usr/local/bin` in PATH.

The window contains downloads, library, uploads, S3 profiles, settings and diagnostics. Click the CPU/RAM button for each logical CPU. Download progress is separated into download, processing and verification. Select rows for bulk actions. Pending jobs may be paused; active processes can be canceled and retried. History removal keeps files; physical library deletion has a separate confirmation.

S3 uses the existing locally encrypted vault. Initialize/unlock it in Profiles, add a destination and credentials, and review computed object keys from Library before submitting uploads. Custom endpoints require the existing conditional-write test. Credentials are never returned to the frontend after saving.

Use Quit to stop the application and save resumable work. Closing the native window also shuts down the workspace. Recovery follows the same rules as TUI. This build is local, unsigned and not notarized.

## Build

```sh
sh scripts/build_gui.sh
```

Go and Python 3 are required; macOS additionally requires Xcode Command Line Tools (`swiftc` and the macOS SDK). No new Go dependencies are added. The desktop window uses system AppKit/WebKit. The UI assets are embedded in the Go executable. The transparent logo in `internal/ui/gui/web/logo.png` is also packaged as the macOS Finder/Dock icon during the build.

## Browser mode / Linux

```sh
go build -o camelclip-gui ./cmd/camelclip-gui
./camelclip-gui --browser
```

Linux's build script produces `dist/camelclip-gui` and uses the default browser. Keep the process running and choose Quit inside the UI when finished; closing a browser tab alone does not terminate the backend. File/folder paths can be entered manually; macOS's native host also provides system pickers.

The server listens only on a random IPv4 loopback port and uses a random per-process session token. API requests check the session token, Host and Origin. Nothing is hosted externally. `--no-open` prints the private local launch URL for debugging; do not share it. `CAMELCLIP_CONFIG_DIR` permits an isolated workspace, just as with TUI.

## Validation

```sh
go test -race -count=1 -timeout=5m ./...
go vet ./...
python3 scripts/test_gui.py dist/CamelClip.app/Contents/MacOS/camelclip-gui
```

The GUI smoke test uses a temporary config and a disposable local HTTP video fixture. It covers real yt-dlp/FFmpeg download and verification, quality choice, second-instance rejection, library retention after history removal, encrypted test credentials, vault rotation and restart. It requires FFmpeg, yt-dlp and permission to bind local sockets. Real S3 providers and Linux desktop integration require platform/provider acceptance checks.
