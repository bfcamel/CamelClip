# Licensing and distribution audit

Reviewed 2026-10-19 against the pinned source and local Go 1.27.2 toolchain.

## Project and shipped components

CamelClip source is Apache-2.0. `LICENSE` contains the full license and `NOTICE` preserves project attribution. Renaming does not change ownership or the license of third-party code. Packages contain both files and `THIRD_PARTY_LICENSES.txt`.

The dependency graph contains 58 non-main modules. Static package inspection identifies 27 external modules in each macOS target and 25 in each Linux target; the larger notices file deliberately includes development and platform-specific dependencies as well. Go runtime and vendored standard-library notices are included. Module sources are unmodified.

Main license families found are Apache-2.0 (AWS SDK/Smithy), BSD-style (Go/x packages, gopsutil and several utilities), MIT (several helpers and modernc components), and SQLite's public-domain dedication. Upstream aggregate notices also include component-specific terms, including MPL-2.0 for HashiCorp code listed by modernc; these notices are retained rather than replaced by the project's Apache license. Presence in aggregate notices does not establish that a component is linked into every executable.

The former generator only inspected root-level license files named in go.mod. It omitted nested AWS/Smithy BSD attributions, PATENTS files and Go runtime notices. The current generator resolves the full module graph through Go, includes nested LICENSE/COPYING/NOTICE/PATENTS/AUTHORS/COPYRIGHT files (excluding testdata and repository metadata), and fails if a module has no license. Review embedded source-specific notices when upgrading dependencies: filename collection alone is not proof of complete legal compliance.

## Reproduce the inventory

```sh
go mod download all
python3 scripts/notices.py
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go list -deps -f '{{if .Module}}{{.Module.Path}} {{.Module.Version}}{{end}}' ./cmd/camelclip
# Repeat for darwin/amd64, linux/arm64 and linux/amd64.
```

Exact module versions and checksums are recorded in go.mod/go.sum. Obtain their corresponding source with `go mod download -json MODULE@VERSION`; its `Dir` and `Zip` identify the original module source. For example, the bundled SQLite translation and its aggregate notices are in [modernc.org/sqlite v1.60.1 source](https://proxy.golang.org/modernc.org/sqlite/@v/v1.60.1.zip). If redistributing modified copyleft-covered components, review and fulfill the applicable source-availability obligations separately.

## External programs

CamelClip invokes yt-dlp, FFmpeg/ffprobe and optional Deno as separate installed programs; the package builder does not bundle them or link FFmpeg libraries. Their licenses are not changed to Apache-2.0 by this project. FFmpeg licensing depends on its build options (LGPL/GPL and potentially nonredistributable configurations); yt-dlp's source and packaged executables can have different third-party obligations. Re-audit before bundling any external executable.

Primary references: [Apache-2.0 redistribution terms](https://www.apache.org/licenses/LICENSE-2.0), [FFmpeg licensing](https://ffmpeg.org/legal.html), [yt-dlp license section](https://github.com/yt-dlp/yt-dlp#license), [Go license](https://go.dev/LICENSE).

## Scope and remaining boundaries

This is a source/distribution inventory and attribution check, not a legal opinion or proof of provenance of every historical contribution. No claim is made about rights to downloaded media, codec patents, trademarks, or production cloud-provider terms. Before a public release, review the exact final archives and any added binaries/assets. Package tests verify the presence of required legal files and checksums.
