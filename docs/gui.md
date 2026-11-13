# Graphical interface

The GUI is an additional frontend for the same CamelClip workspace. TUI and CLI remain available. It uses the same settings, queue/history, library, encrypted S3 vault, downloader and verification pipeline.

## macOS

The locally built application is `dist/CamelClip.app`. Double-click it in Finder, or run from the project directory:

```sh
open dist/CamelClip.app
```

Close any running CamelClip TUI/GUI first. The existing exclusive database lock prevents a second owner; startup shows the error in a macOS alert. Startup details are retained in `~/Library/Logs/CamelClip/gui-startup.log`.

The first run asks for language, output directory and download preferences. You can run offline diagnostics before saving. Existing TUI settings bypass setup. Dependencies (`yt-dlp`, `ffmpeg`, `ffprobe`, and Deno when needed by the source) must be installed. Finder launch includes Homebrew's `/opt/homebrew/bin` and `/usr/local/bin` in PATH.

Downloads contains the queue, playlists and clips. Library contains local files. Cloud storage contains uploads and S3 profiles. Settings includes diagnostics.

Add links uses automatic quality selection by default. A numeric video height is an upper limit. CamelClip selects the highest available height within that limit. If all available heights exceed it, the download fails. Enable “Choose quality after inspection” to choose manually.

Click the CPU/RAM button for each logical CPU. Download progress is separated into download, processing and verification. Select rows for bulk actions. Pending jobs may be paused; active processes can be canceled and retried. History removal keeps files; physical library deletion has a separate confirmation.

S3 uses the existing locally encrypted vault. Initialize/unlock it in Profiles, add a destination and credentials, and review computed object keys from Library before submitting uploads. Custom endpoints require the existing conditional-write test. Credentials are never returned to the frontend after saving.

Use Quit to stop the application and save resumable work. Closing the native window also shuts down the workspace. Recovery follows the same rules as TUI. This build is local, unsigned and not notarized.

## Build

```sh
sh scripts/build_gui.sh
```

Go and Python 3 are required; macOS additionally requires Xcode Command Line Tools (`swiftc` and the macOS SDK). No new Go dependencies are added. The desktop window uses system AppKit/WebKit. The UI assets are embedded in the Go executable. The transparent logo in `internal/ui/gui/web/logo.png` is also packaged as the macOS Finder/Dock icon during the build.

## Browser mode / Linux

```sh
mkdir -p dist/build
go build -o dist/build/camelclip-gui ./cmd/camelclip-gui
./dist/build/camelclip-gui --browser
```

The installer build places Linux GUI binaries in `dist/build/`; the user installer exposes `camelclip-gui` and uses the default browser. Keep the process running and choose Quit inside the UI when finished; closing a browser tab alone does not terminate the backend. File/folder paths can be entered manually; macOS's native host also provides system pickers.

The server listens only on a random IPv4 loopback port and uses a random per-process session token. API requests check the session token, Host and Origin. Nothing is hosted externally. `--no-open` prints the private local launch URL for debugging; do not share it. `CAMELCLIP_CONFIG_DIR` permits an isolated workspace, just as with TUI.

## Validation

```sh
go test -race -count=1 -timeout=5m ./...
go vet ./...
python3 scripts/test_gui.py dist/CamelClip.app/Contents/MacOS/camelclip-gui
```

The GUI smoke test uses a temporary config and a disposable local HTTP video fixture. It covers real yt-dlp/FFmpeg download and verification, quality choice, second-instance rejection, library retention after history removal, encrypted test credentials, vault rotation and restart. It requires FFmpeg, yt-dlp and permission to bind local sockets. Real S3 providers and Linux desktop integration require platform/provider acceptance checks.

## Playlist monitoring and progress

The Playlists section adds/edit subscriptions, shows first-scan state and errors, pauses/resumes monitoring, requests a scan and backfills existing entries. Details separate local task state from each S3 delivery. Monitoring requires CamelClip to remain running; see [playlist documentation](playlists.md).

Download progress displays an approximate aggregate across separate video/audio tracks as soon as one track reports progress. The approximation marker avoids presenting equal-track weighting as a known byte total. Downloaded bytes and track count remain visible. If no percentage can be inferred, an indeterminate activity bar is shown. Speed and ETA remain separate; completed jobs reach 100% only after verification/publication.

## Appearance and folder access

Each completed download has a **Reveal** button directly beside the video, selecting its actual output file in macOS Finder. Library rows expose the same action for available files. Video detail dialogs retain **Reveal** too. There is no global output-folder action in sidebar navigation. If the file has disappeared, the application reports it instead of revealing an unrelated folder.

In **Settings → Appearance**, choose **System**, **Light** or **Dark**, then save. System follows operating-system appearance changes automatically while the application is open. Both palettes cover forms, dialogs, status badges, tables and progress controls. The macOS title bar follows the selected appearance too. The preference persists across restarts and is retained when settings are saved by TUI/CLI. Existing settings without a theme use System.

## Clip by Link

The **Clips** workspace creates reviewed batches from a URL or library asset. Use **Create clips** in library details, enter manual ranges or inspect source chapters, then review the plan before admitting it. Batch/source progress and independent fragment actions use the same Core service as CLI/TUI. See [clip usage and current boundaries](clips.md).

## Media companions (source/main)

[Posters and subtitles](companions.md) use an interface-independent application
service, scoped content-addressed files and immutable caption snapshots. See the
linked guide for commands, desktop/terminal workflows, limits and remaining stages.
