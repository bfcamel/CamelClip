# MacDown

A small, terminal-only video and audio downloader for macOS, written in Go.
It uses [yt-dlp](https://github.com/yt-dlp/yt-dlp) for YouTube, VK and other supported sites, and FFmpeg to merge or convert media. Site support depends on your yt-dlp version and access to the particular video.

## Features

- Fetch the video's title, duration and available resolutions before downloading.
- Choose video with audio, or audio only.
- Navigate interactive menus with **↑ / ↓ and Enter**. Numbered menus are used when terminal controls are unavailable.
- Download one link or process a queue sequentially; a failed download does not stop subsequent links.
- Keep a fixed output directory and save preferences from the Settings menu.
- Stay in a persistent dashboard between downloads and browse files already on disk.
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

## Interactive dashboard

Run `macdown` without URLs to open a persistent terminal dashboard. The screen shows your fixed output directory and the number and total size of media files in it. Use **Up/Down + Enter** to choose an action:

- **Download video** — paste one or more URLs, choose a quality for each video, and download as MP4.
- **Download audio** — use the audio format selected in Settings.
- **My files** — browse the actual media files currently stored in your output directory. Files are sorted newest first and show size and modification date. Open a file in the default macOS app, reveal it in Finder, refresh the list, or navigate between pages.
- **Settings** — change the output directory, language, preferred video quality and audio format. Changes are saved immediately, even if you do not download anything.
- **Diagnostics** — check installed dependencies without leaving the program.
- **Exit** — close the dashboard.

After a download queue finishes or fails, the dashboard opens again with a status message. You can download more files in the same session. The output directory is no longer requested before each download: change it in Settings. New folders are created and checked for write access before the setting is saved.

Press **q** in a submenu to go back; in the main menu it exits. Leave the URL input blank to go back without downloading. **Ctrl+C** stops the program and its active downloader. Terminal settings and the previous screen are restored on exit. When terminal controls are unavailable, numbered menus are used instead; `q` still goes back.

The file browser scans the selected folder itself, not a download-history database, so it also sees supported media placed there manually. It lists regular video/audio files directly inside that directory; hidden files, subdirectories, symlinks, partial downloads and recognizable temporary format files are excluded. File actions never delete your downloads.

Explicit commands such as `macdown download URL` remain single-shot operations: they finish and return to your shell without opening a menu.

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
| `dashboard.go` | Persistent home screen and settings editor |
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
