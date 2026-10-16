MacDown 0.9.0 adds Linux support alongside macOS, local regression checks and release packaging.

- Linux/macOS, each with amd64 and arm64 builds.
- Persistent SQLite queue, separate settings and verified media publication.
- Offline `macdown doctor` and a private-data-free `doctor --json` report.
- Fault tests for database limits, stage failures, cancellation and restart recovery.
- Contributor, security, support and troubleshooting documentation.

Extract the archive for your OS/architecture and run `sh install.sh --add-to-path`. Install yt-dlp, FFmpeg/ffprobe and Deno separately; see README. Verify archives against SHA256SUMS.

This is a pre-1.0 release. Windows and a GUI are not supported. macOS builds are unsigned and not notarized. Checksums detect corruption and are not publisher signatures. Configuration and the SQLite schema remain compatible with 0.8.0. Back up your configuration directory before upgrading.
