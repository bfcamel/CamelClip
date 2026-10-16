# Release process

Pull requests and main commits run native checks on macOS/Linux for amd64/arm64. These include Go race tests, vet, media conversion/integrity tests, SQLite fault tests, native terminal/crash/installer smoke tests and checks of all four archives. Local fixtures keep the checks independent of YouTube availability and credentials.

Before releasing:

1. Update the version in `internal/ui/terminal/cli.go`, CHANGELOG.md and docs/release-notes.md in the same commit.
2. Merge the tested commit into main. Run `sh build.sh`, `python3 scripts/package.py` and `python3 scripts/test_packages.py` locally when possible.
3. Push a unique matching tag, for example `v0.9.0`. The Release workflow requires the tag commit to be an ancestor of main and reruns all checks before publishing.
4. Inspect the release assets and test a clean install/upgrade from an extracted archive. All 0.x and hyphenated versions are marked as GitHub prereleases.

The workflow uses the repository GITHUB_TOKEN with contents:write only for the publishing job. Pull-request checks have read-only repository access and cannot publish. Assets are created with `gh release create`; existing releases are not overwritten. Never move a published tag or replace a released archive. Fix a release with a new patch version. A failed unpublished tag run can be rerun after transient infrastructure problems; source changes require a new version/tag.

Each archive contains the executable, installer, README, changelog, Apache license/notices and BUILD.json with the full source commit. SHA256SUMS covers exactly the four versioned archives. Packaging normalizes timestamps, ownership and file order; identical input binaries and source metadata produce identical archives. This does not claim independently reproducible Go builds across toolchains.

```sh
# Linux
sha256sum -c SHA256SUMS
# macOS
shasum -a 256 -c SHA256SUMS
```

These checks detect download corruption; checksums hosted beside artifacts do not authenticate a publisher. Signing, macOS notarization and Homebrew packaging are future work. External yt-dlp, FFmpeg and Deno are not bundled.

Current automated failure coverage is recorded in [testing.md](testing.md). Real external-site smoke checks remain manual and must use content you can access; their instability must not hide deterministic test failures.
