# Code and documentation audit — 2026-10-19

Scope: current source, CLI/TUI entry points, settings paths, queue and library lifecycle, encrypted credentials, S3 transport, packaging, dependency notices and documentation. Review combines targeted source inspection, automated tests and local integration fixtures; it is not an exhaustive security certification.

## Corrected findings

| Priority | Finding | Resolution |
| --- | --- | --- |
| P2 | S3's two-minute total HTTP timeout also covered streamed uploads, causing slow or large healthy requests to fail | Timeout now covers response headers; transfer lifetime follows cancellation |
| P2 | `queue add` without URLs returned successful empty JSON | Reject with a usage error; regression test added |
| P2 | Dependency notices omitted nested licenses, PATENTS and the Go runtime | Collect resolved module graph and nested legal files; regenerate notices |
| P3 | Password error described a byte minimum while code requires ten Unicode characters | Correct the message; remove an unnecessary immutable copy when reading descriptor input |
| P3 | Documentation mixed jobs-panel controls with the library and described all permits as FIFO | Clarify panel-specific controls, priority ordering and separate workspace CLI parsing |

## Rename verification

All product names, executable names, module imports, repository URLs, environment variables, temporary/staging prefixes, cryptographic context labels, installer paths and package names use CamelClip/camelclip. `camelclip tui` and bare `camelclip` open the interface. Old command aliases and data migration are intentionally absent. Historical local data is left untouched outside the new CamelClip directory.

## Remaining findings / limits

- **P2 — bulk CLI exit status:** queue add/action results include per-item errors in JSON, but item failures can still leave process exit status zero. Scripts must inspect each result's `Error` field; a future change should return nonzero after emitting the complete result list when any item fails. See `queueCLI` in `internal/ui/terminal/workspace_cli.go`.
- **P2 — background metadata retry:** `LibraryService.Enrich` clears `NeedsProbe` after any probe failure, including a transient missing tool, and may write its older asset snapshot over the missing status set by `Refresh`. Manual refresh/scan can recover. Follow-up should preserve the refreshed record and distinguish transient failures from permanent ones.
- Workspace CLI has a separate parser: general `--lang` and download flags are not accepted, and the command group must come first. This is documented; parser unification is outside this rename.
- Secret bytes are cleared where practical, but Go string copies and garbage collection prevent a guarantee that every secret copy is immediately erased. Existing documentation states this limitation.
- Native validation is macOS/arm64. The other three targets are cross-built; this does not establish native Linux/Intel desktop behavior.
- S3 verification uses a local emulator. Production providers, real credentials, interrupted internet transfers and browser authentication require separate acceptance testing.

See `docs/licensing.md` for the separate distribution/license audit. Exact local validation results are recorded with the delivered audit report.

## Validation completed

- `go test -race -count=1 -timeout=5m ./...`: passed.
- `go vet ./...`: passed.
- Four cross-builds (macOS/Linux, arm64/amd64): passed.
- `scripts/smoke.py`: first-run consent, crash/restart at each media stage, PTY navigation/restoration, installer overwrite protection and PATH idempotence passed.
- `scripts/test_workspace.py`: real media indexing, loopback S3, idempotence, index exclusion/reimport and encrypted storage passed.
- Four archive checksums, required legal files, executable permissions and repeatable packaging: passed.
- `govulncheck`: no known vulnerabilities in called symbols or imported packages. Module-only finding [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932) concerns unused `golang.org/x/crypto/openpgp`; CamelClip uses Argon2id and does not import OpenPGP.

The S3 timeout correction was reviewed and exercised by the normal transfer/cancellation tests; a wall-clock transfer longer than two minutes was not run. External production downloads/cloud accounts were not exercised in this audit.
