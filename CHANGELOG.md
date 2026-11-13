# Changelog

## Unreleased

- Added Windows browser, Explorer and Downloads integration for source builds running under WSL 2.
- Grouped desktop navigation and made automatic capped video quality the default in Add links.
- Added clip creation and media companion workflows.

## 0.14.0 — 2026-10-22 (preview)

- Follow any number of persistent YouTube playlists with new-only or full initial synchronization, durable video identity, bounded scanning/admission and shared materializations.
- Add playlist management to GUI, TUI and CLI, including pause/resume, backfill, per-item status and selected S3 destinations.
- Keep playlist local-only mode independent of global S3 auto-upload; persist delivery bindings and reuse already published compatible files.
- Show approximate video/audio track progress from the first track, downloaded bytes and an activity bar when totals are unknown; retain speed and ETA.
- Harden Core publication identity, retry state, adapter contracts, cloud cancellation and service shutdown; use incremental queue persistence.
- Migrate to SQLite schema 5 without discarding queue/library/profile history. Older executables cannot open migrated workspaces.
- Preview remains unsigned/not notarized; no server or bundled external media tools.

## 0.13.1 — 2026-10-22 (preview)

- Make installer, package and launcher messages English-only while preserving application localization.
- Rewrite README and contributor/release guidance; consolidate current documentation and archive dated reports.
- Organize intermediate builds, versioned installers and legacy terminal archives under dedicated output directories.
- Add public English-content and documentation-link checks.
- Preserve published v0.13.0 assets; distribute setup language changes under a new version.

## 0.13.0 — 2026-10-22 (preview)

- Add the graphical workspace alongside the existing TUI and CLI, with the shared exclusive workspace lock.
- Use the transparent camel-player logo in the GUI and macOS Finder/Dock icon.
- Add selectable GUI/TUI/CLI user installers and native macOS component packages for arm64/amd64.
- Preserve installed choices during updates; add explicit component removal, payload/ownership verification and ordinary write-failure rollback.
- Include installation manifests, maintenance commands, desktop launchers, dependency reporting and isolated installer tests.
- Preview limits: unsigned/not notarized; system-wide macOS install and native Linux/Intel acceptance remain pending. External media dependencies are not bundled.

## 0.12.1 — 2026-10-21

- Resume downloads blocked by disk/staging limits even when no worker was created yet.
- Filter explicitly selected S3 jobs before pagination so older unrelated jobs cannot starve CLI uploads.
- Refresh verified S3 profile capabilities for queued jobs without accepting destination changes.
- Return a nonzero CLI status after reporting all queue action results when any item fails.
- Preserve missing-file status and retry failed background metadata probes with a delay and fair page traversal.
- Add regression coverage for disk recovery, selected upload backlogs, profile checks, partial CLI failures and library recovery.

### CamelClip rename and earlier audit

- Rename executable, module, TUI branding, paths, variables and build artifacts to CamelClip/camelclip. No legacy aliases or data migration.
- Add explicit `camelclip tui`.
- Reject empty `queue add`, clarify Unicode password length and avoid a redundant immutable password copy.
- Remove the two-minute total S3 upload timeout; retain a response-header timeout and cancellation.
- Expand dependency license notices to nested attributions, patent grants and the Go runtime.
- Reconcile CLI, workspace and packaging documentation with source; record audit scope and limits.

## 0.12.0

- Priority/order controls, pending pause/resume, confirmed bulk actions, bounded retries and staging/disk backpressure.
- Persistent paginated media library independent of download history, safe scans, metadata refresh and separate file deletion.
- Local encrypted S3 profiles, per-session master-password unlock and atomic password rotation.
- Independent S3 uploads with multipart, conditional publication, explicit capability tests, cleanup and recovery checkpoints.
- Local asynchronous per-CPU and RAM monitoring, four workspace screens and CLI management commands.
- Transactional upgrades from queue schema 1/2 to schema 3; preserved source metadata and automatic-upload publication outbox.

## 0.11.0

- Adaptive jobs dashboard with status tabs, stable selection, search, details and help.
- Independent FIFO download and processing limits; defaults: 5 downloads and max(CPU / 4, 2) processing tasks.
- Structured per-stage metrics and bounded speed sampling, with unknown and stale values shown explicitly.
- Configurable per-job rates and a conservative global budget divided into fixed shares for new downloads.
- Transactional SQLite schema migration preserving existing jobs and settings.

## 0.10.0

- Rename the product, command, environment variables, folders and packages to CamelClip.
- First-run setup for language, storage, video profile and optional browser access, with dependency checks and retry.
- Reset terminal background attributes between cells to contain selected rows inside their panel.
- Remove deprecated CLI modes and legacy JSON queue import.

## 0.9.0

- Linux support on amd64 and arm64, alongside both macOS architectures.
- Linux desktop opening through xdg-open, XDG configuration and localized download directories, headless operation and a platform-aware installer.
- Offline diagnostics for dependencies, selected encoders, storage access, free disk space, saved settings and SQLite integrity; shareable JSON reports exclude private data.
- Local checks for race detection, media processing, PTY navigation, crash recovery and installation on macOS/Linux.
- Locally prepared versioned release archives, dependency notices, build metadata, SHA-256 checksums and reproducible archive packaging.
- Failure tests for full SQLite storage, failed processing/verification, descendant cancellation, corrupt databases and preservation of existing files.
- Contribution, security, community, support, troubleshooting and release documentation.

## 0.8.0

- Apache License 2.0.
- Isolated downloader core and application service with strict interfaces for frontends and storage.
- Transactional SQLite queue/history and separate JSON settings.
- Consistent settings/back navigation and a visible settings shortcut.

## 0.7.0

- Persistent queue and separate download, processing and verification stages.
- Configurable H.264, HEVC, VP9 and AV1 profiles with MP4, MKV and WebM containers.

## 0.6.0

- Browser-cookie selectors and clearer authentication/bot-challenge errors.
- Compatible H.264/AAC MP4 output, complete decode verification and atomic publication.
