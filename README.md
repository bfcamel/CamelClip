<p align="center">
  <img src="internal/ui/gui/web/logo.png" width="104" height="104" alt="CamelClip logo">
</p>

# CamelClip

**A local video and audio workspace for macOS and Linux — GUI, TUI and CLI.**

Download with yt-dlp, process and verify with FFmpeg, organize a persistent media library, and optionally upload to S3-compatible storage. Written in Go and licensed under Apache-2.0.

[Download the preview](https://github.com/bfcamel/CamelClip/releases/tag/v0.13.1) · [Installation](docs/installers.md) · [CLI reference](docs/cli.md) · [Contributing](CONTRIBUTING.md)

## Download and install

Choose your operating system and architecture. Each installer lets you select **GUI**, **TUI**, **CLI**, or any combination.

| Platform | Recommended download |
| --- | --- |
| macOS — Apple Silicon | [Graphical installer (.pkg)](https://github.com/bfcamel/CamelClip/releases/download/v0.13.1/camelclip_setup_0.13.1_macos_arm64.pkg) |
| macOS — Intel | [Graphical installer (.pkg)](https://github.com/bfcamel/CamelClip/releases/download/v0.13.1/camelclip_setup_0.13.1_macos_amd64.pkg) |
| Linux — x86_64 | [Setup archive (.tar.gz)](https://github.com/bfcamel/CamelClip/releases/download/v0.13.1/camelclip_setup_0.13.1_linux_amd64.tar.gz) |
| Linux — ARM64 | [Setup archive (.tar.gz)](https://github.com/bfcamel/CamelClip/releases/download/v0.13.1/camelclip_setup_0.13.1_linux_arm64.tar.gz) |

macOS user-scoped archives and [SHA256SUMS](https://github.com/bfcamel/CamelClip/releases/download/v0.13.1/SHA256SUMS) are available on the release page. Installer text and project documentation are English-only; the application supports English and Russian.

**Preview status:** builds are unsigned and not notarized by Apple. Native Linux/Intel acceptance and system-wide macOS installation acceptance remain pending. See [platform support](docs/support.md) before use. Windows and a server edition are not available.

### macOS

Open the `.pkg`, select components, and follow the installer. GUI is installed to `/Applications/CamelClip.app`. TUI has a Finder launcher and the `camelclip-tui` command; CLI provides `camelclip`.

### Linux

Extract the setup archive, enter the extracted directory, and run:

```sh
./setup.sh                                # Interactive component menu
./setup.sh --components gui,tui,cli --yes  # Install all components
```

Installation is user-scoped by default and does not require `sudo`. Add `~/.local/bin` to PATH if your shell does not already include it. Linux GUI opens in your default browser.

See [installer instructions](docs/installers.md) for custom prefixes, updates, component removal and the legacy terminal-only installer.

### Runtime dependencies

Install **yt-dlp** and **FFmpeg/ffprobe**. Deno and yt-dlp's EJS support are needed for relevant YouTube workflows. Dependencies must be on PATH; installers report missing tools and never silently install third-party software.

On macOS with Homebrew:

```sh
brew install yt-dlp ffmpeg deno
```

On Linux, use your distribution for FFmpeg and desktop tools, and follow the official [yt-dlp](https://github.com/yt-dlp/yt-dlp#installation) and [Deno](https://docs.deno.com/runtime/getting_started/installation/) installation instructions. Site behavior and account access can change independently of CamelClip.

## Choose your interface

| Interface | Best for | Launch |
| --- | --- | --- |
| GUI | A graphical workspace for downloads, library and S3 | Open CamelClip on macOS; `camelclip-gui` on Linux |
| TUI | Keyboard-driven use in a terminal or SSH session | `camelclip-tui` or `camelclip tui` |
| CLI | Explicit commands and scripts | `camelclip help` |

The interfaces share settings, queue/history, the library and the encrypted S3 vault. One process owns a local workspace at a time; a second GUI/TUI session reports the existing lock. Installing multiple interfaces does not start them automatically.

## What CamelClip does

- **Download video and audio:** batch URLs, quality selection, video codecs/containers and audio extraction.
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
```

`download` selects video and `audio` selects audio regardless of saved defaults. Each job captures its settings when submitted. Supported video codecs are H.264, HEVC, VP9 and AV1; containers are MP4, MKV and compatible WebM profiles. Audio formats include MP3, M4A, Opus, FLAC, WAV and best available. See [complete command and rate-limit details](docs/cli.md).

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
- [CLI reference](docs/cli.md) · [Queue, library and S3](docs/s3.md)
- [Troubleshooting](docs/troubleshooting.md) · [Platform support](docs/support.md)
- [Architecture](docs/architecture.md) · [Testing and acceptance](docs/testing.md)
- [Changelog](CHANGELOG.md) · [Security](SECURITY.md) · [Community guidelines](CODE_OF_CONDUCT.md)
- [License](LICENSE) · [Distribution licensing](docs/licensing.md) · [Third-party notices](THIRD_PARTY_LICENSES.txt)
