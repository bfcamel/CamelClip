# Clipnaro

A local terminal video and audio downloader for macOS and Linux, written in Go. Uses [yt-dlp](https://github.com/yt-dlp/yt-dlp) for supported sites and FFmpeg/ffprobe for processing and verification. Apache-2.0 licensed.

## Getting started

Install yt-dlp, FFmpeg/ffprobe and a supported JavaScript runtime such as Deno. YouTube also needs yt-dlp's [EJS components](https://github.com/yt-dlp/yt-dlp/wiki/EJS). Dependencies must be on PATH.

On macOS:

```sh
brew install yt-dlp ffmpeg deno
```

On Linux, install FFmpeg and optionally xdg-utils from your distribution (for example `sudo apt install ffmpeg xdg-utils` on Debian/Ubuntu). Use current [yt-dlp](https://github.com/yt-dlp/yt-dlp#installation) and [Deno](https://docs.deno.com/runtime/getting_started/installation/) distributions. Distribution yt-dlp packages may lag behind site changes.

Extract the package for your OS/architecture, then:

```sh
sh install.sh --add-to-path
# Open a new terminal.
clipnaro
```

On first interactive launch, setup asks for language, output directory, video profile and optional browser access, then checks dependencies/storage. You can retry the checks or open the app and fix dependencies later. Settings are saved only after confirmation. CLI help, version, config and download commands never start the wizard. Existing saved settings bypass it.

The installer uses `~/.local/bin/clipnaro`. `--add-to-path` updates `~/.zprofile` for macOS/zsh, `~/.bashrc` for Linux bash, or `~/.profile` for Linux sh. Other shells need manual PATH setup. Use `--bin-dir /absolute/path` for a custom location and `--force` to replace an installed binary. To uninstall, remove the binary and the startup line marked `# Clipnaro CLI`; media and application data are retained.

## Commands

```sh
clipnaro                                  # Interactive setup/dashboard
clipnaro download 'VIDEO_URL'
clipnaro download 'VIDEO_URL' --quality 1080
clipnaro audio 'VIDEO_URL' --audio-format mp3
clipnaro formats 'VIDEO_URL'
clipnaro download --list links.txt
clipnaro download 'VIDEO_URL' --video-codec vp9 --video-format webm
clipnaro download 'VIDEO_URL' --cookies-browser firefox:default-release
clipnaro setup                             # Run guided setup again
clipnaro config                            # Effective settings as JSON
clipnaro doctor                            # Offline checks and guidance
clipnaro doctor --json > doctor.json        # Shareable diagnostic report
clipnaro version
clipnaro help download
```

Use an explicit command for downloads. URLs are positional; multiple URLs or a text list (one URL per line, `#` comments accepted) are supported. Flags may appear before/after URLs. `--` ends flag parsing. `download` always selects video; `audio` selects audio regardless of saved mode. `--lang auto|en|ru` selects the UI language.

Video codecs: `h264` (default), `hevc`, `vp9`, `av1`. Containers: `mp4` (default), `mkv`; `webm` for VP9/AV1 only. Video audio uses AAC, or Opus for WebM. Audio-only formats: `mp3`, `m4a`, `opus`, `flac`, `wav`, `best`. Each queued job captures its own settings; changing preferences affects new jobs.

## Terminal workspace

- **My files:** actual media in the output directory. Enter opens a file; `o` shows it in Finder on macOS or opens its containing folder on Linux. `f` opens the output directory.
- **Jobs:** inspection, quality selection, download, processing and verification. Enter selects quality or opens details; `x` cancels, `r` retries and `d` removes finished history without deleting files.
- **Event log:** recent progress and errors.

`Tab` / `Shift+Tab` switch status tabs; arrows or `j`/`k` select jobs. `a` adds video or audio, `v` quickly adds video, `/` searches titles and URLs, `Enter` opens details, `?` shows help. `s` opens Settings, `f` opens the download folder, `o` reveals the selected completed file, `l` opens the log, and `q` exits. `x` cancels, `r` retries, and `d` removes history only after confirmation. In nested settings, Esc/left/Backspace returns to the parent. The layout adapts from 40 × 10 to wide windows with details and logs. Numbered menus are used when terminal controls are unavailable. English and Russian are supported. Downloads continue while browsing settings.

Download and processing limits are independent: five network transfers and `max(cpu_count / 4, 2)` processing tasks by default (integer division). Processing includes merging, audio extraction, conversion and full verification. Configure these in Settings or override for one CLI invocation with `--max-downloads 1..32` and `--max-processing auto|1..32`.

`--rate-limit 2MiB/s` allocates a conservative global budget into fixed shares based on maximum downloads; unused shares are not redistributed. `--job-rate-limit 512KiB/s` caps each job. The smaller nonzero limit applies. `0` means unlimited. Changes apply to newly launched network processes; running transfers retain their existing limits. These are bytes/second; CLI overrides do not overwrite saved preferences.

Progress percentages belong to separate stages. Missing totals/speed/ETA display as an em dash; speeds older than three seconds are marked stale. Queue schema upgrades preserve existing history transactionally.

## Storage and integrity

Preferences (`settings.json`) and queue/history (`queue.sqlite3`) are separate:

- macOS: `~/Library/Application Support/Clipnaro/`
- Linux: `${XDG_CONFIG_HOME:-$HOME/.config}/Clipnaro/`
- Override: `CLIPNARO_CONFIG_DIR=/absolute/path`

The default output is `~/Downloads/Clipnaro`; Linux respects localized `XDG_DOWNLOAD_DIR`. Browser cookies are disabled by default and only read when explicitly selected. Firefox/Chrome profile selectors use yt-dlp syntax. Safari is macOS-only; Flatpak/Snap browser profiles may require explicit paths. Cookies cannot grant access your account lacks.

Queue writes are transactional and one session owns the queue at a time. Interrupted active work is restored on restart; processing/verification may restart from the beginning. Failed jobs can be retried; manually canceled jobs stay canceled. Corrupt databases are reported, never silently reset.

Files remain in hidden request-specific staging directories while downloading, converting and verifying. Missing fragments abort the download. FFprobe checks streams/duration and FFmpeg fully decodes the result before atomic publication. Existing files are never overwritten. A failed job retains staged data for retry. A subprocess-held staging lock prevents simultaneous writers after an abrupt parent exit; cancellation terminates the process group.

Doctor checks tools, selected encoders, writable storage, free space, settings and SQLite integrity. It does not read cookies or test site authentication. JSON reports exclude URLs, private paths, profile selectors and raw dependency output. Normal download logs may contain paths/URLs; review them before sharing.

## Build and validate

Go 1.26+ and Python 3 are needed for building/packaging; CGO is not required for the application. FFmpeg/ffprobe are required for media tests.

```sh
go test -race -count=1 -timeout=5m ./...
go vet ./...
go build -o clipnaro ./cmd/clipnaro
python3 scripts/smoke.py ./clipnaro
sh build.sh
python3 scripts/package.py
python3 scripts/test_packages.py
```

`dist/` contains `clipnaro-{macos,linux}-{arm64,amd64}`, versioned archives and SHA256SUMS. On macOS, `Start.command` launches the terminal interface. Verify checksums with `shasum -a 256 -c SHA256SUMS` (macOS) or `sha256sum -c SHA256SUMS` (Linux). Builds are unsigned; macOS binaries are not notarized. Published release files must never be replaced.

## Project documentation

- [Troubleshooting](docs/troubleshooting.md)
- [Supported environments](docs/support.md)
- [Architecture and adding a GUI](docs/architecture.md)
- [Failure tests](docs/testing.md)
- [Local release process](docs/releases.md)
- [Contributing](CONTRIBUTING.md), [Security](SECURITY.md), [Community conduct](CODE_OF_CONDUCT.md)
- [Changelog](CHANGELOG.md), [License](LICENSE), [Notices](NOTICE), [Third-party licenses](THIRD_PARTY_LICENSES.txt)

Windows and a GUI are not currently supported. Site access depends on yt-dlp, the current site behavior and your account/network. No hosted automation is required for local checks or release packaging.

## Queue, library and S3 workspace

Use `1`–`4` for jobs/library/uploads/profiles and `c` for every logical CPU and system RAM. Queue ordering, pending pause, group actions, a persistent media index and locally encrypted S3 destinations are available in TUI and CLI. S3 is optional, uses separate slots and never changes a successful local download into a failure. [Controls, commands, secret handling and transfer guarantees](docs/s3.md).
