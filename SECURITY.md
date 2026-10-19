# Security policy

Only the latest published version receives fixes. Before 1.0, compatibility changes may occur in minor releases and must be recorded in the changelog. No security support period or response SLA is promised.

Report suspected vulnerabilities privately using the repository's **Security → Advisories → Report a vulnerability** feature when available. If private reporting is disabled, open an issue asking the maintainer to provide a private channel, without exploit details or sensitive data. Do not publish cookies, credentials, browser databases or private download URLs.

Include the affected version, operating system, impact and minimal reproduction. Share `camelclip doctor --json` if useful; this report excludes paths, URLs, cookie contents and raw dependency output. Review any other attachments before sending them.

CamelClip runs external yt-dlp, FFmpeg, ffprobe and optionally Deno from PATH. Install them from trusted sources and keep them current. The application never downloads or updates these dependencies automatically. It reads browser cookies only when explicitly configured or requested for a download; authentication is not checked by doctor.

Release archives include SHA-256 checksums for download integrity. These are not publisher signatures. macOS builds currently have no Developer ID signature or notarization; Linux packages are unsigned. See [docs/releases.md](docs/releases.md).
