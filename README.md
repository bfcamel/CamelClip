<p align="center">
  <img src="internal/ui/gui/web/logo.png" width="104" height="104" alt="CamelClip logo">
</p>

# CamelClip

**A local video and audio workspace for macOS and Linux — GUI, TUI and CLI.**

Download with yt-dlp, process and verify with FFmpeg, organize a persistent media library, and optionally upload to S3-compatible storage. Written in Go and licensed under Apache-2.0.

[Download the preview](https://github.com/bfcamel/CamelClip/releases/tag/v0.16.0) · [Installation](docs/installers.md) · [CLI reference](docs/cli.md) · [Contributing](CONTRIBUTING.md)

## Download and install

Version **0.16.0** includes dependency-aware setup for macOS, Linux and Windows via WSL 2.

Choose your operating system and architecture. Each installer lets you select **GUI**, **TUI**, **CLI**, or any combination.

| Platform | Recommended download |
| --- | --- |
| macOS — Apple Silicon | [Graphical installer (.pkg)](https://github.com/bfcamel/CamelClip/releases/download/v0.16.0/camelclip_setup_0.16.0_macos_arm64.pkg) |
| macOS — Intel | [Graphical installer (.pkg)](https://github.com/bfcamel/CamelClip/releases/download/v0.16.0/camelclip_setup_0.16.0_macos_amd64.pkg) |
| Linux — x86_64 | [Setup archive (.tar.gz)](https://github.com/bfcamel/CamelClip/releases/download/v0.16.0/camelclip_setup_0.16.0_linux_amd64.tar.gz) |
| Linux — ARM64 | [Setup archive (.tar.gz)](https://github.com/bfcamel/CamelClip/releases/download/v0.16.0/camelclip_setup_0.16.0_linux_arm64.tar.gz) |
| Windows via WSL 2 — x64 | [Windows setup (.exe)](https://github.com/bfcamel/CamelClip/releases/download/v0.16.0/camelclip_setup_0.16.0_windows_amd64.exe) |
| Windows via WSL 2 — ARM64 | [Windows setup (.exe)](https://github.com/bfcamel/CamelClip/releases/download/v0.16.0/camelclip_setup_0.16.0_windows_arm64.exe) |

macOS user-scoped archives and [SHA256SUMS](https://github.com/bfcamel/CamelClip/releases/download/v0.16.0/SHA256SUMS) are available on the release page. Installer text and project documentation are English-only; the application supports English and Russian.

**Preview status:** builds are unsigned and not notarized by Apple. Linux desktop and macOS Intel acceptance and system-wide macOS installation acceptance remain pending. See [platform support](docs/support.md) before use. Windows setup installs the Linux application in WSL 2. Interactive Windows acceptance remains pending.

### macOS

Open the `.pkg`, select components, and follow the installer. GUI is installed to `/Applications/CamelClip.app`. TUI has a Finder launcher and the `camelclip-tui` command; CLI provides `camelclip`.

### Linux

Extract the setup archive, enter the extracted directory, and run:

```sh
./setup.sh                                # Interactive component menu
./setup.sh --components gui,tui,cli --yes  # Install all components
```

Application installation is user-scoped; installing dependencies may require `sudo`. Add `~/.local/bin` to PATH if your shell does not already include it. Linux GUI opens in your default browser.

See [installer instructions](docs/installers.md) for custom prefixes, updates, component removal and the legacy terminal-only installer.

### Runtime dependencies

Setup checks **yt-dlp, FFmpeg, ffprobe and Deno** before installing CamelClip. If all four run, it installs only the selected components. Otherwise it offers to provision missing dependencies; Linux uses APT, DNF or Pacman first and Homebrew for unavailable packages. macOS uses Homebrew. See [installer details and recovery](docs/installers.md).

## Choose your interface

| Interface | Best for | Launch |
| --- | --- | --- |
| GUI | A graphical workspace for downloads, library and S3 | Open CamelClip on macOS; `camelclip-gui` on Linux |
| TUI | Keyboard-driven use in a terminal or SSH session | `camelclip-tui` or `camelclip tui` |
| CLI | Explicit commands and scripts | `camelclip help` |

The interfaces share settings, queue/history, the library and the encrypted S3 vault. One process owns a local workspace at a time; a second GUI/TUI session reports the existing lock. Installing multiple interfaces does not start them automatically.

## What CamelClip does

**Clip creation:** [Clip by Link](docs/clips.md) creates precise fragments from one URL or library file, with shared source preparation, independent verified outputs and optional S3 delivery. Included in the 0.16.0 installers.


- **Posters and subtitles:** inspect existing tracks, load a poster, search transcripts and propose reviewed clip ranges. GUI, terminal and JSON CLI share the same Core service. [Usage and current scope](docs/companions.md).
- **Create clips:** 1–100 manual/chapter intervals, precise H.264/AAC video or audio output, durable batches and provenance. [Scope and usage](docs/clips.md).
- **Download video and audio:** batch URLs, quality selection, video codecs/containers and audio extraction.
- **Follow YouTube playlists:** any number of subscriptions, new-only or full initial sync, durable video-ID history, shared downloads, pause/backfill and optional selected S3 destinations.
- **See live progress:** approximate video/audio track progress, downloaded bytes and an activity bar when totals are unknown; speed and ETA remain separate.
- **Control the queue:** separate download/processing limits, fixed-share rate budgets, priorities, pending pause, cancellation, retry and bulk actions.
- **Keep a local library:** search, filters, metadata refresh, folder scanning, playback and explicit file management.
- **Verify before publication:** staged downloads, stream/duration checks, full FFmpeg decode and publication without overwriting existing media.
- **Use optional S3 storage:** encrypted local credentials, independent upload slots, destination preview and verification status.
- **Inspect health:** offline dependency/storage diagnostics, job history, logs and CPU/RAM monitoring.

## CLI quick start

```sh
camelclip download 'VIDEO_URL' --quality 1080
camelclip audio 'VIDEO_URL' --audio-format mp3
camelclip download --list links.txt
camelclip queue list --json
camelclip library list --json
camelclip doctor
camelclip playlists add 'PLAYLIST_URL' --initial new_only --interval 15
camelclip playlists watch
```

`download` selects video and `audio` selects audio regardless of saved defaults. Each job captures its settings when submitted. Supported video codecs are H.264, HEVC, VP9 and AV1; containers are MP4, MKV and compatible WebM profiles. Audio formats include MP3, M4A, Opus, FLAC, WAV and best available. See [complete command and rate-limit details](docs/cli.md).

## YouTube playlist subscriptions

Open **Playlists** in GUI or press **5** in TUI. Add a persistent YouTube playlist URL, choose whether to download its existing entries, set a check interval (minimum five minutes), and optionally select S3 profiles. No S3 selection means local-only, even when global auto-upload is enabled elsewhere.

New-only mode establishes its baseline after the first complete scan; later newly observed video IDs are downloaded, including old videos newly added anywhere in the list. Reordering, renaming, duplicate entries and re-adding known videos do not create another download. Compatible subscriptions share local work, and each keeps its own cloud delivery requirements. The default output folder is shared; selecting different output folders deliberately creates separate local files.

Keep GUI/TUI running, or run `camelclip playlists watch`, for automatic monitoring and transfers. One-shot CLI list/add/sync commands do not start background downloads; `sync ID` performs discovery only. A locked S3 vault retains pending deliveries until explicitly unlocked (CLI watch supports `--password-fd`). Files are retained locally. Live/upcoming videos are deferred until a completed recording is available. Private playlists require an explicitly selected browser with working access.

Subscription count has no product cap, but scans and download admission use bounded shared workers. Actual polling latency depends on backlog, playlist sizes and YouTube availability. A video added and removed between polls cannot be guaranteed to be discovered. See [playlist behavior and commands](docs/playlists.md).

## Your files and settings

Application data lives in `~/Library/Application Support/CamelClip/` on macOS and `${XDG_CONFIG_HOME:-$HOME/.config}/CamelClip/` on Linux. `CAMELCLIP_CONFIG_DIR` selects an isolated workspace. Downloads default to the system Downloads folder under `CamelClip`.

Settings and queue/history are stored separately. Removing history does not delete media; physical file deletion is a separate confirmed action. Uninstalling program components retains settings, history, encrypted profiles and downloaded media. Browser cookies are disabled unless explicitly selected. Review logs before sharing them because they can contain paths and URLs.

## Build and contribute

Building requires Go 1.26+ and Python 3; macOS GUI/package builds also need Xcode Command Line Tools. Runtime users do not need a compiler or Python.

```sh
go test -race -p 1 -count=1 -timeout=5m ./...
go vet ./...
sh scripts/build_gui.sh          # Local macOS app: dist/CamelClip.app
sh scripts/build_installers.sh   # All installer targets; run on macOS
python3 scripts/test_installers.py
```

Generated files stay under `dist/`: `build/` for intermediate installer payloads, `installers/<version>/` for distributable packages, and `terminal/` for optional terminal-only release archives. See [contributor instructions](CONTRIBUTING.md) and the [release workflow](docs/releases.md).

## Documentation

- [Installation and removal](docs/installers.md) · [Graphical interface](docs/gui.md)
- [Playlist subscriptions](docs/playlists.md) · [CLI reference](docs/cli.md) · [Queue, library and S3](docs/s3.md)
- [Troubleshooting](docs/troubleshooting.md) · [Platform support](docs/support.md)
- [Architecture](docs/architecture.md) · [Testing and acceptance](docs/testing.md)
- [Changelog](CHANGELOG.md) · [Security](SECURITY.md) · [Community guidelines](CODE_OF_CONDUCT.md)
- [License](LICENSE) · [Distribution licensing](docs/licensing.md) · [Third-party notices](THIRD_PARTY_LICENSES.txt)
