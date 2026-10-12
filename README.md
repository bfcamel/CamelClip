# MacDown

A small, terminal-only video and audio downloader for macOS, written in Go.
It uses [yt-dlp](https://github.com/yt-dlp/yt-dlp) for YouTube, VK and other supported sites, and FFmpeg to merge or convert media. Site support depends on your yt-dlp version and access to the particular video.

## Features

- Fetch the video's title, duration and available resolutions before downloading.
- Choose video with audio, or audio only.
- Navigate interactive menus with **↑ / ↓ and Enter**. Numbered menus are used when terminal controls are unavailable.
- Download one link or process a queue sequentially; a failed download does not stop subsequent links.
- Keep a fixed output directory and save preferences from the Settings menu.
- Work in independent file, job and event-log panels while downloads run in the background.
- Update download progress on the same terminal line.
- Show explanations for common access, network, format and disk errors.

**English and Russian interfaces are available.** The first-run default is detected from `LC_ALL`, `LC_MESSAGES`, `LANG`, then macOS preferred languages. Other languages fall back to English. Choose a language in the first menu or pass `-lang en`, `-lang ru`, or `-lang auto`. Changes in Settings are saved immediately. yt-dlp diagnostics and operating-system errors may remain in their original language. There is no graphical interface or background server.

## CLI commands

```sh
macdown                                # Interactive menu
macdown download 'VIDEO_URL'            # Video with audio, saved as MP4
macdown download 'VIDEO_URL' --quality 1080
macdown audio 'VIDEO_URL' --audio-format mp3
macdown formats 'VIDEO_URL'             # List available formats
macdown download --list links.txt       # Sequential queue
macdown config                         # Effective settings and file path as JSON
macdown doctor                         # Check dependency paths and versions
macdown version                        # Program version
macdown help                           # Help
macdown help download                  # Command help
```

Both `--long` and the original `-long` flags are accepted. Download options can appear before or after URLs. Use `--` to stop option parsing. The `download` command always selects video, and `audio` always selects audio, regardless of saved mode. Commands with missing URLs fail immediately; the interactive menu is available when no command or URL is provided. Existing `macdown -url URL -mode audio` commands still work.

## Install as a command

After cloning the repository or extracting a compiled package, run:

```sh
sh install.sh --add-to-path
```

The installer builds from source when Go is available, otherwise uses the packaged binary for your Mac. It installs `macdown` into `~/.local/bin` and adds one PATH line to `~/.zprofile` for the default macOS zsh shell. **Open a new Terminal window**, then run `macdown doctor` from any directory. Dependencies still need to be installed separately:

```sh
brew install yt-dlp ffmpeg deno
```

For a custom location, use `sh install.sh --bin-dir /absolute/path/to/bin` and add that directory to your shell's PATH yourself. Without `--add-to-path`, shell configuration is not modified. Use `--force` to replace an existing installation when upgrading.

To uninstall, remove `~/.local/bin/macdown` and, if no longer needed, the line marked `# MacDown CLI` from `~/.zprofile`. Downloaded files and saved settings are unaffected.

## Install and build

Install [Homebrew](https://brew.sh/) if needed, then:

```sh
brew install go yt-dlp ffmpeg deno
git clone https://github.com/bfcamel/video-downloader.git
cd video-downloader
go build -o macdown .
./macdown
```

Go 1.22 or newer is required to build. The Go program uses only the standard library. yt-dlp, FFmpeg/ffprobe and Deno must be available in your `PATH`; Go is not required to run a compiled binary.

For full YouTube support, yt-dlp also needs its EJS components. If your installation reports missing JavaScript components, follow the [official EJS setup instructions](https://github.com/yt-dlp/yt-dlp/wiki/EJS).

Build and check both Mac architectures:

```sh
sh build.sh
```

The `dist/` directory contains:

| File | Purpose |
| --- | --- |
| `macdown-macos-arm64` | Apple Silicon Macs |
| `macdown-macos-amd64` | Intel Macs |
| `Start.command` | Double-click in Finder to open the terminal interface |
| `install.sh` | Install the `macdown` command |
| `README.md` | This guide |

You can also double-click the repository's `Start.command` after building. The launcher finds a binary next to itself or inside `dist/`. Compiled binaries are excluded from Git.

## Multitasking terminal workspace

Run `macdown` without URLs to open a full-screen workspace with three independent panels:

- **My files** (left): actual media files in the configured folder, refreshed every second. Open the selected file or reveal it in Finder.
- **Jobs** (right): queued, active, completed, failed and canceled jobs with status/progress. Up to **two workers** can fetch metadata or download media concurrently.
- **Event log** (bottom): recent activity and error explanations, with independent scrolling.

Downloads continue while you browse files, scroll the log, add more URLs, or edit settings. New jobs capture a copy of their settings, so changing the output folder or audio format does not move or alter jobs already in the queue. The UI owns all screen rendering; child-process output is captured rather than printed over the panels.

### Keyboard controls

| Key | Action |
| --- | --- |
| `Tab` / `Shift+Tab` | Switch panels |
| `↑` / `↓`, `PageUp` / `PageDown` | Move selection or scroll the focused panel |
| `v` | Add one or more video URLs |
| `a` | Add one or more audio URLs |
| `Enter` in Files | Open the selected file |
| `Enter` in Jobs | Choose an available quality, or inspect job details |
| `x` in Jobs | Cancel the selected queued or active job |
| `r` in Jobs | Retry a failed or canceled job |
| `r` elsewhere | Refresh the file list |
| `o` in Files | Reveal the selected file in Finder |
| `f` | Open the configured output folder |
| `s` | Edit output folder, language, video quality or audio format |
| `Esc` | Close a dialog |
| `Ctrl+U` in a text field | Clear the input |
| `q` | Exit; confirm first if any jobs are pending or active |
| `Ctrl+C` | Stop the program and its active downloader processes |

For video downloads, metadata is fetched in the background. When a job says **Choose quality: Enter**, select it in Jobs, press Enter, and choose the resolution. It then starts when a worker is free. Audio jobs start automatically using the configured audio format. Progress is per stream; the percentage can restart when yt-dlp switches from video to audio.

Text inputs accept multiple space-separated URLs and bracketed paste. Paste does not trigger keyboard shortcuts. Settings are saved immediately. The UI supports English and Russian and adapts to terminal resizing; a window of at least **73 columns × 20 rows** is required. Larger windows show more files and jobs. The cursor and previous terminal screen are restored on exit.

The file browser scans the configured folder rather than a download-history database. It also sees media placed there manually. Hidden files, subdirectories, symlinks and recognizable temporary files are excluded. File actions never delete downloads. Completed job history and the event log are kept for the current session only; partial downloads may be resumed by yt-dlp when a job is retried.

In non-interactive terminals, a sequential numbered-menu interface is used instead. `macdown doctor` remains available for dependency diagnostics. Explicit commands such as `macdown download URL` remain single-shot operations and return to your shell when finished.

## Command-line examples

Flags may appear before or after positional URLs. Alternatively, repeat `--url`.

Video with audio, up to 1080 pixels in height:

```sh
./macdown -mode video -quality 1080 -url 'https://www.youtube.com/watch?v=VIDEO_ID'
```

Best available quality:

```sh
./macdown -mode video -quality best -url 'https://vk.com/video-OWNER_ID_VIDEO_ID'
```

Audio only, converted to MP3:

```sh
./macdown -mode audio -audio-format mp3 -url 'VIDEO_URL'
```

Keep the source audio format when possible:

```sh
./macdown -mode audio -audio-format best -url 'VIDEO_URL'
```

Custom output directory:

```sh
./macdown -out "$HOME/Movies/Downloads" -url 'VIDEO_URL'
```

Multiple links with the same settings, without interactive prompts:

```sh
./macdown -mode video -quality 720 -url 'FIRST_URL' -url 'SECOND_URL'
```

Or:

```sh
./macdown -mode video -quality 720 'FIRST_URL' 'SECOND_URL'
```

Read links from a text file (one URL per line; empty lines and lines beginning with `#` are ignored):

```sh
./macdown -mode video -quality best -list links.txt
```

Inspect all yt-dlp formats without downloading:

```sh
./macdown -formats -url 'VIDEO_URL'
```

Use an existing browser login for a video that requires authentication:

```sh
./macdown -cookies-browser firefox -url 'VIDEO_URL'
```

Supported browser names: `safari`, `chrome`, `firefox`, `edge`, `brave`. Browser access is opt-in for each run and is **not saved**. macOS may ask for permission to read browser data. Cookies do not grant access to content your account cannot view.

Show flags: `./macdown -h`.

## Saved settings

On macOS, settings are stored in:

```text
~/Library/Application Support/MacDown/settings.json
```

Changes made in the dashboard's Settings screen are saved immediately. Successful interactive downloads also remember the selected mode and video quality. These values become the next run's defaults, including command-line runs. Explicit CLI flags override them for that command only and do not change the saved output directory or other preferences. URL lists and browser choices are never saved.

Rename or remove that file to reset the defaults. Missing settings use built-in defaults; malformed settings produce a warning and are ignored. Settings are written atomically with owner-only file permissions.

For a separate settings profile, set `MACDOWN_CONFIG_DIR` to a directory of your choice. Its `settings.json` will be used instead.

## Output and limitations

- Default output directory: `~/Downloads/MacDown`.
- Video is saved with audio in **MP4**, without forced re-encoding. The original codecs are retained: playback support (including QuickTime) depends on those codecs. If a source codec cannot be remuxed into MP4, the download reports an error rather than silently re-encoding.
- Quality is a **maximum source frame height**, not an exact resolution or an upscaling target. This also applies to portrait videos. If no suitable format with audio is available, the item fails.
- Audio formats: `mp3`, `m4a`, `opus`, `flac`, `wav`, `best`. Converting lossy audio to FLAC/WAV does not improve its original quality.
- File sizes are not estimated. Available resolutions are based on site metadata and may change before the download starts.
- Single-video URLs only: playlists, multi-video pages and ongoing livestreams are not processed by the download queue.
- Existing completed files are not overwritten. yt-dlp attempts to resume partial downloads when you repeat the same request. The queue itself is not persisted across runs.
- Browser authentication, regional restrictions and DRM are not bypassed.
- yt-dlp configuration files are ignored to keep behavior predictable.
- Exit status: `0` on success/help/version/config, `1` for operational failures, `2` for command-line syntax errors, `130` when canceled. `doctor` returns `1` when a required dependency cannot be found or run; a missing optional Deno runtime is reported as a warning.

## Troubleshooting

Update the download engine when sites change:

```sh
brew upgrade yt-dlp ffmpeg deno
```

| Problem | Suggested action |
| --- | --- |
| Missing yt-dlp / FFmpeg | Install dependencies using the command above |
| Sign-in, age restriction or private video | Check browser access, then pass `-cookies-browser` |
| HTTP 429 | Wait before retrying; the site is limiting requests |
| HTTP 403 | Update yt-dlp and check whether authentication is required |
| Connection or DNS failure | Check connectivity and retry |
| Requested quality unavailable | Select a different resolution or use `-quality best` |
| Not enough disk space / permission denied | Free space or choose a writable output directory |
| Corrupt settings | Rename or remove `settings.json` |

Unknown failures retain the original yt-dlp diagnostic. Metadata retrieval has a 90-second timeout. Failed items are reported and the remaining queue continues, unless you cancel.

## Source layout

| File | Responsibility |
| --- | --- |
| `main.go` | Entry point and exit status |
| `app.go` | Workflow and sequential queue |
| `cli.go` | Commands, argument parsing, help, version and diagnostics |
| `options.go` | Validation and yt-dlp arguments |
| `menu.go` | Scrollable arrow-key and numbered menus |
| `dashboard.go` | Sequential fallback and settings editor |
| `panels.go` | Full-screen event loop, input and panel actions |
| `panels_render.go` | Bounded screen layout and dialogs |
| `jobs.go` | Background jobs, cancellation and captured progress |
| `library.go` | Folder scanning, file list and Finder actions |
| `media.go` | Metadata, available qualities, child processes and readable errors |
| `progress.go` | In-place progress rendering and plain-text log output |
| `settings.go` | Loading and atomically saving preferences |
| `i18n.go` | Language detection and English/Russian messages |
| `*_test.go` | Unit tests |

Run checks:

```sh
go test ./...
go vet ./...
```

URLs are passed as individual process arguments, without a shell. On cancellation, the downloader's process group is stopped so its FFmpeg children do not continue working in the background.
