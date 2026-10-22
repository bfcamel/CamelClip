# CamelClip 0.14.0 — YouTube playlist monitoring and visible progress

This preliminary release adds persistent YouTube playlist subscriptions to GUI, TUI and CLI, alongside the Core hardening completed since 0.13.1.

## Playlist subscriptions

- Any number of saved subscriptions, with shared bounded scan workers and fair download admission.
- New-only baseline or download-existing initial mode; new IDs are discovered anywhere in the playlist.
- Durable membership and idempotent admission; compatible playlists share a local download.
- Per-subscription processing preferences, check interval, pause/resume, manual sync and explicit backfill.
- Strict local-only mode or selected S3 destinations, independent of global auto-upload; existing local results can acquire cloud delivery without redownloading.
- Separate local/delivery status, scan errors and locked-vault waiting. No automatic local file deletion.

GUI: open Playlists. TUI: press 5. CLI: `camelclip playlists add`, then `camelclip playlists watch`. One-shot CLI sync performs discovery only. Monitoring runs while the application or watch command is running; no daemon/server is installed. See [playlist documentation](playlists.md).

## Progress and Core reliability

Download progress appears from the first video/audio track with an approximation marker, track count and downloaded bytes. Unknown totals show an indeterminate activity bar instead of a static dash. Speed and ETA remain separate. Core now validates adapter outputs, separates retry attempts, preserves concurrent cancellation and persists incremental queue changes atomically.

## Installation and migration

Use the matching macOS package or Linux/macOS user setup archive; select GUI, TUI, CLI or any combination. Close running sessions before updating. Existing settings, media, queue/library history and encrypted S3 profiles are retained. SQLite schema 5 is migrated transactionally; back up application data before attempting an executable downgrade. Old published assets/tags remain unchanged.

## Preview limitations

- Packages/apps are unsigned and not notarized by Apple. Native Linux/Intel runtime and full system-wide macOS acceptance remain pending.
- yt-dlp, FFmpeg/ffprobe and relevant YouTube JavaScript support are external dependencies.
- Discovery latency depends on playlist size, backlog, authentication and YouTube availability; there is no hard polling deadline or guarantee to observe items removed between checks.
- Private playlist access requires explicitly configured working browser authentication. Live/upcoming entries wait for a completed recording.
- S3 needs vault unlock after startup; failed deliveries remain pending/visible. No server or unattended secret-unlock mechanism is included.

This is an evaluation preview, not a production-readiness claim. See [testing](testing.md) and [platform support](support.md) for acceptance scope.
