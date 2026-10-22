# CamelClip 0.13.1 — English-only setup and project polish

This preliminary release makes all installer messages and public project descriptions English-only. English and Russian remain available inside the application.

## Changes

- English-only terminal setup menus, confirmations, dependency reports, macOS package screens and legacy Finder launcher.
- A shorter README with direct platform downloads, interface selection, prerequisites and documentation links.
- Consolidated architecture, installation, contribution and release documentation; dated validation records moved to history.
- Clean build directories: intermediate payloads in `dist/build/`, versioned installer artifacts in `dist/installers/<version>/`, and optional terminal-only archives in `dist/terminal/`.
- A public-content check prevents non-English installer/publication text and broken local documentation links from returning.

## Installation

Use the matching macOS `.pkg`, or extract the Linux/user-scoped macOS archive and run `./setup.sh`. Select GUI, TUI, CLI, or any combination. See [installer instructions](installers.md) for updates and removal.

No application settings or database migration is introduced by this patch. Program removal retains settings, history, media and encrypted profiles. Published `v0.13.0` assets remain unchanged; installer language changes are distributed as a new version.

## Preview limitations

Packages/apps are unsigned and not notarized by Apple. Full system-wide macOS installation, native Linux/Intel runtime and desktop acceptance remain pending. Linux GUI opens in a browser. External yt-dlp, FFmpeg/ffprobe and, where required, Deno are not bundled. The server component is not available. This is an evaluation preview, not a production-readiness claim.

See [testing and acceptance](testing.md) for automated coverage and outstanding checks.
