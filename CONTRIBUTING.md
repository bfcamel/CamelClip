# Contributing

CamelClip is a terminal video downloader for macOS and Linux, licensed under Apache-2.0. Contributions through issues and pull requests are welcome. Windows and a graphical interface are outside the current supported scope.

## Development

Install Go 1.26+, FFmpeg and ffprobe, then clone the repository. yt-dlp and Deno are needed for real site downloads; automated checks use local fixtures and never need an account or cookies.

```sh
go test -race -count=1 -timeout=5m ./...
go vet ./...
go build -o camelclip ./cmd/camelclip
python3 scripts/smoke.py ./camelclip
sh build.sh
python3 scripts/package.py
python3 scripts/test_packages.py
```

Format Go changes with `gofmt`. Tests that use media tools skip when those tools are unavailable locally; install those tools before release validation so the media tests run. `build.sh` cross-builds all four supported targets. No test should access YouTube, use your real browser profile or fill the host disk.

## Architecture

Read [docs/architecture.md](docs/architecture.md). Keep `core` and `application` independent of UI, concrete databases and external tools. Add frontend behavior through the existing ports. OS integration belongs in platform files, not the downloader contract. Architecture tests enforce package directions.

Add tests for observable behavior and regressions, especially cancellation, atomic publication and persistence failure. Preserve existing user files; never recover a damaged queue by silently replacing it with an empty one. New database schemas need explicit migrations and failure tests. Changes to settings and CLI flags need compatibility notes.

## Pull requests

Describe the user-visible problem, resulting behavior, testing and any migration. Keep unrelated changes separate. By submitting a contribution, you agree to license it under Apache-2.0. Preserve third-party notices and add dependency licenses when introducing a library. Maintainers review and merge; no release schedule or response-time guarantee is implied.

For access problems, reproduce with an up-to-date yt-dlp first. Do not attach cookies, browser databases, private URLs or your queue database to an issue. See [SECURITY.md](SECURITY.md) for private reporting.

Dependency updates are reviewed manually: inspect upstream release notes, update go.mod/go.sum and third-party notices where needed, then run the local checks.
