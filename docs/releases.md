# Release workflow

Releases are built locally. GitHub hosts the source and downloadable assets; publishing a tag does not trigger a build. All public release titles, descriptions, notes and installer text are English-only. Application localization remains independent.

## 1. Prepare source

1. Update `internal/ui/terminal/cli.go`, `CHANGELOG.md`, `docs/release-notes.md`, and versioned download links in README/install documentation.
2. Run `python3 scripts/check_public_content.py`, format Go, and validate the focused changes.
3. Merge the release source and create a clean checkout of its exact commit. Do not build public packages from a modified working tree.
4. Export `RELEASE_COMMIT` as the full source SHA and `SOURCE_DATE_EPOCH` as that commit's timestamp. Those values identify the release payload; they do not independently prove provenance.

## 2. Validate and build

Requirements: Go 1.26+, Python 3, yt-dlp, FFmpeg/ffprobe and, for relevant media workflows, Deno. macOS builds need Swift and the macOS SDK.

```sh
python3 scripts/check_public_content.py
test -z "$(gofmt -l cmd internal)"
go test -race -p 1 -count=1 -timeout=5m ./...
go vet ./...
sh scripts/build_installers.sh
python3 scripts/test_installers.py
```

Use read-only macOS choice validation for the host architecture:

```sh
python3 scripts/test_installer_choices.py dist/installers/<version>/camelclip_setup_<version>_macos_<arch>.pkg
```

The installer release has **six payloads**: macOS arm64/amd64 `.pkg` files and four macOS/Linux user-setup archives, plus `SHA256SUMS`. Only files for that version belong in its checksum inventory. Debug Distribution XML and intermediate payload binaries are not release assets.

The legacy terminal-only build remains available separately:

```sh
sh build.sh
python3 scripts/package.py
python3 scripts/test_packages.py
```

Its four archives and checksum file live in `dist/terminal/`; do not mix them with installer checksums.

## 3. Record acceptance honestly

Follow the [test and platform acceptance matrix](testing.md). Run media checks with their dependencies installed. Cross-builds and package inspection do not establish Linux desktop, Intel runtime or system-wide macOS installation acceptance. Tests must use isolated accounts/directories and disposable credentials.

For public macOS distribution, sign inner executables/app and outer package with the appropriate identities, then notarize. `INSTALLER_SIGN_ID` signs the outer package only. Credentials never belong in Git. Current previews are unsigned/not notarized and must say so explicitly; do not instruct users to disable OS security checks.

## 4. Upload, verify and publish

1. Create a draft release targeting the exact validated source SHA with a unique version tag. Mark 0.x and evaluation builds as prereleases.
2. Upload six payloads and `SHA256SUMS`. Verify remote names, sizes and server SHA-256 digests against the local build.
3. Review the English description, platform instructions, source SHA, remaining acceptance gaps, dependency requirements and known limitations.
4. Publish only after the required acceptance is complete. An explicitly authorized experimental preview may be published with outstanding checks clearly documented; never describe it as production-ready.
5. Verify that the public tag points to the expected source commit and every download is available.

Published tags and assets must never be replaced or moved. Text descriptions may be corrected without changing files. Installer language or code changes require a new version. Keep old releases for reproducibility and mark superseded previews in their descriptions instead of deleting their history.

Checksums detect corruption, not publisher identity. Do not downgrade an incompatible SQLite schema when rolling back executable files. Preserve user data throughout installation, maintenance and release testing.
