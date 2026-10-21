# CamelClip 0.12.1

Patch update for queue recovery and metadata reliability:

- Resume downloads after disk/staging capacity returns, including jobs without an existing worker.
- Start explicitly selected S3 uploads even when older unrelated jobs fill the queue.
- Use current verified S3 profile capabilities for unchanged destinations.
- Return a nonzero CLI exit status for partial queue failures while preserving complete JSON results.
- Retry temporary metadata failures and preserve missing-file status.

No settings or database migration is required when upgrading from 0.12.0. macOS/Linux arm64/amd64 remain the supported build targets. Windows support is unchanged. See [the changelog](../CHANGELOG.md) and [workspace documentation](s3.md).

S3 validation uses a local API emulator; production cloud accounts and real site authentication require separate acceptance testing. Versioning this patch does not publish GitHub release assets.
