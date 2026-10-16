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
- Show explanations for common access, network, format and disk errors while preserving the original ERROR line; distinguish YouTube bot checks from account restrictions.

**English and Russian interfaces are available.** The first-run default is detected from `LC_ALL`, `LC_MESSAGES`, `LANG`, then macOS preferred languages. Other languages fall back to English. Choose a language in the first menu or pass `-lang en`, `-lang ru`, or `-lang auto`. Changes in Settings are saved immediately. yt-dlp diagnostics and operating-system errors may remain in their original language. There is no graphical interface or background server.

## CLI commands

```sh
macdown                                # Interactive menu
macdown download 'VIDEO_URL'            # Video with audio, using the saved video profile
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
go build -o macdown ./cmd/macdown
./macdown
```

Go 1.26 or newer is required to build. SQLite uses a pure-Go driver; CGO is not required. yt-dlp, FFmpeg/ffprobe and Deno must be available in your `PATH`; Go is not required to run a compiled binary.

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
| `d` in Jobs | Remove a finished, failed or canceled entry; keep downloaded files |
| `o` in Files | Reveal the selected file in Finder |
| `f` | Open the configured output folder |
| `s` | Edit output folder, language, video quality or audio format |
| `Esc` | Close a dialog |
| `Ctrl+U` in a text field | Clear the input |
| `q` | Exit; confirm first if any jobs are pending or active |
| `Ctrl+C` | Stop the program and its active downloader processes |

For video downloads, metadata is fetched in the background. When a job says **Choose quality: Enter**, select it in Jobs, press Enter, and choose the resolution. It then starts when a worker is free. Audio jobs start automatically using the configured audio format. Progress is per stream; the percentage can restart when yt-dlp switches from video to audio.

Text inputs accept multiple space-separated URLs and bracketed paste. Paste does not trigger keyboard shortcuts. Settings are saved immediately. The UI supports English and Russian and adapts to terminal resizing; a window of at least **73 columns × 20 rows** is required. Larger windows show more files and jobs. The cursor and previous terminal screen are restored on exit.

The file browser scans the configured folder rather than a download-history database. It also sees media placed there manually. Hidden files, subdirectories, symlinks and recognizable temporary files are excluded. File actions never delete downloads. Jobs and their settings are saved in `queue.sqlite3` beside `settings.json`. Unfinished jobs resume on the next dashboard launch; a pending quality selection remains pending. Failed and canceled jobs wait for `r`. Completed history remains available; `d` removes a finished queue entry without deleting files. The event log is session-only. Partial downloads can be resumed by yt-dlp.

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

Supported browser names: `safari`, `chrome`, `firefox`, `edge`, `brave`. Browser access is opt-in. Choose **Browser cookies** in Settings to remember a browser and profile, or pass `--cookies-browser` for a command. Clear the setting to disable browser access. Firefox profile paths and containers use yt-dlp syntax, for example `firefox:default-release` or `firefox:/path/to/profile::Personal`. macOS may ask for permission to read browser data. Cookies do not grant access to content your account cannot view.

Show flags: `./macdown -h`.

## Saved settings

On macOS, settings are stored in:

```text
~/Library/Application Support/MacDown/settings.json
```

Changes made in the dashboard's Settings screen are saved immediately. Successful interactive downloads also remember the selected mode and video quality. These values become the next run's defaults, including command-line runs. Explicit CLI flags override them for that command only and do not change the saved output directory or other preferences. URL lists are never saved. Browser choices made in Settings are saved; CLI flags override the current run.

Rename or remove that file to reset the defaults. Missing settings use built-in defaults; malformed settings produce a warning and are ignored. Settings are written atomically with owner-only file permissions.

For a separate settings profile, set `MACDOWN_CONFIG_DIR` to a directory of your choice. Its `settings.json` will be used instead.

## Output and limitations

- Default output directory: `~/Downloads/MacDown`.
- Default video profile: **H.264 / AAC in MP4**, with `yuv420p` pixels and fast-start metadata. Settings let you choose H.264, H.265/HEVC, VP9 or AV1, and MP4, MKV or WebM. WebM requires VP9/AV1 and uses Opus audio; MP4/MKV use AAC. Matching streams are copied; other codecs are converted, which takes extra time and can reduce quality. A chosen encoder must be available in your FFmpeg build; errors are reported without silently changing the profile.
- Quality is a **maximum source frame height**, not an exact resolution or an upscaling target. This also applies to portrait videos. If no suitable format with audio is available, the item fails.
- Audio formats: `mp3`, `m4a`, `opus`, `flac`, `wav`, `best`. Converting lossy audio to FLAC/WAV does not improve its original quality.
- File sizes are not estimated. Available resolutions are based on site metadata and may change before the download starts.
- Single-video URLs only: playlists, multi-video pages and ongoing livestreams are not processed by the download queue.
- Downloads use hidden, request-specific staging folders and a filesystem lock. Missing fragments abort the download. Before publication, FFprobe checks audio/video streams and duration, and FFmpeg decodes the entire result. Only verified files appear in the library. Existing files are never overwritten; an invalid existing file produces an error. Failed downloads retain staged data for retry. Small lock folders remain hidden after success. Different quality/audio choices have separate filenames. The queue and completed history persist across runs.
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

```text
cmd/macdown/                 Composition root for the console executable
internal/core/               UI-independent requests, events and Downloader interface
internal/downloader/ytdlp/   Isolated yt-dlp/FFmpeg backend
internal/application/        Queue interface and job lifecycle service
internal/config/             Configuration types and Repository interface
internal/storage/sqlite/     Transactional queue/history repository and migration
internal/storage/jsonconfig/ Atomic JSON settings repository
internal/bootstrap/          Adapter construction and dependency injection
internal/ui/terminal/        Terminal frontend, menus and presentation
internal/architecture/       Dependency boundary regression tests
```

See [docs/architecture.md](docs/architecture.md) for interface contracts and how to add a GUI. Core packages do not import terminal UI, SQLite, localization, or configuration adapters. The terminal frontend cannot import concrete downloader/storage implementations; an architecture test enforces this direction.

Run checks:

```sh
go test ./...
go vet ./...
```

URLs are passed as individual process arguments, without a shell. On cancellation, the downloader's process group is stopped so its FFmpeg children do not continue working in the background.

## 0.6.0 reliability update

Browser cookies can be selected in both Settings interfaces, including Firefox profiles and containers. This stores only the browser selector, never cookie values. Jobs already queued keep their previous settings. Retrying a failed job picks up the current browser selection.

A successful dependency check does not test browser authentication or EJS. To inspect the original YouTube response without downloading, run:

```sh
yt-dlp --ignore-config --verbose --cookies-from-browser firefox --skip-download --no-playlist 'VIDEO_URL'
```

The output pipeline now avoids concurrent writes to shared audio intermediates, rejects missing fragments and validates completed media before reporting success. Existing AV1/VP9/Opus files are left unchanged; an MP4 filename alone does not ensure player compatibility.

## 0.7.0: stages, video profiles and persistent queue

Video settings apply to newly added jobs. Existing jobs keep the profile captured when added. Choose **Video codec** and **Video container** in Settings in either interface. Incompatible WebM combinations are hidden; selecting H.264/HEVC while WebM is selected switches the container to MKV. Existing settings automatically default to H.264/MP4.

CLI overrides do not change saved preferences:

```sh
macdown download --video-codec hevc --video-format mp4 'VIDEO_URL'
macdown download --video-codec vp9 --video-format webm 'VIDEO_URL'
macdown download --video-codec av1 --video-format mkv 'VIDEO_URL'
```

Jobs show separate **Downloading → Processing → Verifying → Done** stages. Percentages refer to the current stage; downloading reaching 100% does not mean conversion or verification has finished. Processing and verification percentages use FFmpeg timestamps when a duration is known. The final filename includes quality, codec and container so different profiles do not overwrite one another.

The persistent queue is stored in `~/Library/Application Support/MacDown/queue.sqlite3`, or under `MACDOWN_CONFIG_DIR` when set. It includes URLs, per-job settings, metadata, status and error details, but never browser cookie values. Queue files use owner-only permissions, atomic replacement and disk synchronization. One MacDown session owns the queue at a time; another session gets a clear error rather than overwriting it.

CLI download lists are saved before starting the first download. Interrupted CLI jobs can be resumed by opening the dashboard; the numbered interface has a **Saved queue** menu. Restored downloads, conversions and checks resume using staged files; conversion/checking may restart from the beginning. Manually canceled jobs stay canceled. Invalid queue files are retained unchanged and reported; automatic replacement with an empty queue is forbidden. `d` removes selected finished history in the panel interface, while **Clear finished history** does the same in the numbered menu. Neither action deletes downloaded files.

Subprocesses inherit the staging lock so that, even after an abrupt parent exit, a restarted job waits for an orphan downloader/converter to release its files. Normal cancellation still terminates the entire subprocess group.

## License

MacDown project source is licensed under the **Apache License 2.0**. See [LICENSE](LICENSE) and [NOTICE](NOTICE). Third-party runtime dependencies retain their own licenses, reproduced in [THIRD_PARTY_LICENSES.txt](THIRD_PARTY_LICENSES.txt). External yt-dlp/FFmpeg distributions are installed separately and keep their respective license obligations.

## 0.8.0: isolated engine, SQLite and menu navigation

Queue/history data now live in **queue.sqlite3**, with WAL journaling, full synchronization and transactional updates. Application configuration remains a separate **settings.json** file. Per-job parameters and metadata live in database rows; preferences are not written into a second global configuration store.

On first use, a valid legacy `queue.json` is imported exactly once in a transaction, including saved IDs, states, metadata and video profiles. The original file remains unchanged as a backup. Failed imports do not overwrite the legacy file or mark the migration as complete. Run `macdown migrate` to migrate without starting any downloads. All MacDown instances use the same queue lock, including older JSON-based versions.

Settings menus now retain their parent screen. **Esc**, **←**, **Backspace**, or the visible **Back** item returns one level; choosing a setting also returns to Settings. The numbered interface supports **0** or **b** for back. The bottom status line begins with **s: Settings**, including at the minimum terminal width.

Source builds require **Go 1.26+**. Compiled arm64 and amd64 distributions remain self-contained Go executables; SQLite does not require an external server or SQLite installation.
