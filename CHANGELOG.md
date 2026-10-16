# Changelog

## 0.10.0

- Rename the product, command, environment variables, folders and packages to Clipnaro.
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
