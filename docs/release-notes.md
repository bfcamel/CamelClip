# CamelClip 0.12.0 candidate

CamelClip adds a persistent media library, priority/pause/bulk queue controls, local CPU/RAM monitoring and optional S3 uploads independent of downloading and processing.

Use 1–4 to switch jobs, library, uploads and profiles; c opens all logical CPUs and RAM. S3 credentials are encrypted in local SQLite and require a master-password unlock each session. Auto-upload defaults to off. See [workspace and S3 documentation](s3.md) for commands, safety guarantees and current limits.

Schema 1/2 upgrades to schema 3 preserve queue history and settings. Back up before upgrading; older binaries cannot read schema 3. Removing history does not remove assets or files. Imported/existing files can be scanned or refreshed, and missing files remain distinguishable from completed downloads.

S3 validation uses a local API emulator and does not access real cloud accounts. Each custom endpoint requires an explicit conditional-write test before upload. A read-only bucket test does not prove write permission. Remote verification reports size and full-content checksum separately. Multipart retry starts from the beginning after cleanup; durable continuation of individual parts is not implemented. Remote media deletion is not exposed.

This is an unpublished candidate, not a published 1.0 release. macOS/Linux arm64/amd64 remain the build targets. Intel execution on the development Mac and Linux amd64 under an arm64 VM use emulation; production-provider configurations need separate acceptance testing.
