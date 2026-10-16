# Local release process

Tests, builds and packaging run on the maintainer's machine. GitHub hosts source and release files; GitHub Actions and scheduled dependency bots are not part of this process.

## Validate a candidate

Install Go 1.26+, Python 3, FFmpeg and ffprobe. Use a clean checkout and run:

```sh
test -z "$(gofmt -l cmd internal)"
go test -race -count=1 -timeout=5m ./...
go vet ./...
go build -o macdown ./cmd/macdown
python3 scripts/smoke.py ./macdown
sh build.sh
python3 scripts/package.py
python3 scripts/test_packages.py
```

`build.sh` cross-builds macOS/Linux for arm64/amd64. Cross-compilation alone is not a runtime test. Run smoke tests on each supported OS, record the OS/tool versions and distinguish native hardware from emulation. Media tests must run with FFmpeg installed; skipped media tests are not a successful release validation. External-site access checks remain manual and use content the tester can access.

## Prepare and publish

1. Update the application version, CHANGELOG.md and docs/release-notes.md. Complete the release checklist in [roadmap-1.0.md](roadmap-1.0.md).
2. Commit to main, validate that exact clean commit using the commands above, and push it. If anything changes, repeat the relevant checks and rebuild packages. Record the full commit and validation results with the candidate.
3. Upload the four archives and SHA256SUMS to a draft GitHub release targeting that exact commit. Inspect the contents and test installation/upgrade from an extracted archive in an isolated account or machine.
4. Publish only after the recorded checks and manual acceptance are complete. Give the release a unique matching tag, such as `v0.9.0`. Mark 0.x and release candidates as prereleases. A tag push by itself does not build or publish anything.

The existing 0.9.0 draft can be updated before publication. Published tags and assets must never be moved or replaced; fixes require a new version. Keep the previous binary and a configuration backup for compatible rollback. Never downgrade an incompatible SQLite schema.

Each archive contains the executable, installer, README, changelog, Apache license/notices and BUILD.json with the full source commit. SHA256SUMS covers exactly the four versioned archives. Packaging normalizes timestamps, ownership and file order; repeated packaging with the same inputs/toolchain is checked locally. This does not claim independently reproducible Go builds across toolchains.

```sh
# Linux
sha256sum -c SHA256SUMS
# macOS
shasum -a 256 -c SHA256SUMS
```

Checksums detect corruption, not publisher identity. Signing, macOS notarization and distribution choices are tracked in the roadmap. External yt-dlp, FFmpeg and Deno are not bundled. Dependency updates are reviewed and tested manually.
