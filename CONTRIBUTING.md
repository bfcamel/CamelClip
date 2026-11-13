# Contributing to CamelClip

CamelClip is a local media workspace with GUI, TUI and CLI frontends for macOS and Linux, including an experimental Windows desktop integration through WSL 2. Contributions are licensed under Apache-2.0. Please report reproducible problems and keep changes focused.

## Public language

Repository documentation, issue/PR descriptions, release notes, packaging and installer messages are written in English. The application supports English and Russian; preserve both application localizations and the tests that exercise them. Do not translate legal notices or user-provided media metadata.

## Development setup

Use Go 1.26+ and Python 3. Install yt-dlp, FFmpeg/ffprobe and Deno for media smoke tests. macOS GUI/package builds require Xcode Command Line Tools. Automated fixtures must not use real browser cookies, cloud credentials or remote media sites.

```sh
python3 scripts/check_public_content.py
test -z "$(gofmt -l cmd internal)"
go test -race -p 1 -count=1 -timeout=5m ./...
go vet ./...
mkdir -p dist/build
go build -o dist/build/camelclip ./cmd/camelclip
python3 scripts/smoke.py dist/build/camelclip
```

Use `-p 1` for the full race suite on loaded machines: process/diagnostic fixtures have bounded timeouts. A skipped required media test is not release acceptance. See [testing](docs/testing.md) for platform-specific checks.

## Build outputs

- `sh scripts/build_gui.sh`: local GUI at `dist/CamelClip.app` on macOS.
- `sh scripts/build_installers.sh`: four installer targets; run on macOS for native `.pkg` files.
- `python3 scripts/test_installers.py`: archive integrity, ownership and isolated user-installation tests.
- `sh build.sh`, `python3 scripts/package.py`, `python3 scripts/test_packages.py`: optional terminal-only archives in `dist/terminal/`.

Do not commit binaries, app bundles, installer packages, caches, Finder metadata or local `BUILD.json` files. Source assets such as `internal/ui/gui/web/logo.png` belong in Git; generated icons belong in build outputs.

## Architecture and compatibility

Read [architecture](docs/architecture.md). Keep `core` and `application` independent of frontend implementations, databases and external tools. Use the existing application ports and preserve dependency-direction tests. Keep installers independent of application presentation and protect installed-file ownership.

Add behavior/regression tests for cancellation, recovery, persistence and installer changes. Preserve settings, history, encrypted vaults and media. Database changes need explicit transactional migrations and failure coverage. Never repair a corrupt queue by replacing it with an empty database. Document changes to CLI flags, settings, supported platforms and installation paths.

## Pull requests and releases

Describe the user-visible problem, resulting behavior, relevant tests and compatibility impact. Mention verification limits; cross-compilation is not native acceptance. Keep titles and descriptions in English. The `Required` CI check must pass before merging into `main`; it includes native tests and installer validation described in [testing](docs/testing.md). Maintainers review and merge; published tags and binary assets are immutable. Follow the [release checklist](docs/releases.md) for new distributions.

Do not put credentials, private URLs, browser databases or queue databases in issues or PRs. See [security reporting](SECURITY.md) for sensitive findings. Preserve third-party notices and review redistribution licenses before adding dependencies or bundled executables.
