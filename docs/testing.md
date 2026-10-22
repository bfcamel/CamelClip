# Failure and recovery coverage

| Scenario | Automated evidence |
| --- | --- |
| Full SQLite storage | Real `PRAGMA max_page_count` limit causes SQLITE_FULL; transaction rolls back and the database reopens with original jobs |
| Corrupt SQLite | Startup refuses invalid data and preserves original bytes |
| Failed settings save | Canceled writes preserve preferences; failed rename removes temporary files |
| Interrupted network/downloader | Simulated nonzero downloader exit; no final file; retry succeeds |
| Conversion or full-decode verification failure | Injected FFmpeg errors; no final publication; staged source can be retried |
| Abrupt process kill during all three stages | Native subprocess/PTY smoke test restarts against the same SQLite queue; exactly one verified output is published |
| Cancellation with descendants | Delayed child marker must never be written after cancellation |
| Existing invalid user output | Error is returned and existing bytes are preserved |
| Multiple owners | SQLite repository lock tests reject another owner |
| Lifecycle restore | Application tests restore every queued/active/terminal state correctly |
| Terminal navigation | Actual PTY settings/back navigation, status tabs, search, help, narrow/wide resizes and terminal-state restoration |
| Installation and update | Extracted-package layout, isolated HOME, overwrite protection, --force and idempotent PATH entry |
| Diagnostic privacy | Fake dependencies print secret paths/URLs; only safe numeric versions enter the report |

Run `go test -race -count=1 -timeout=5m ./...` and `python3 scripts/smoke.py ./camelclip` with FFmpeg installed. The tests use temporary directories and local fixtures. They do not fill the host disk, kill unrelated processes or access browser data.

A simulated FFmpeg ENOSPC response and a real SQLite quota exercise application failure paths; they do not simulate a physical disk losing power, filesystem corruption or every kernel writeback behavior. Native desktop opening is covered with a fake xdg-open command, so manual checks on GNOME/KDE and different macOS players are still valuable. External auth/session expiry and an actual network outage during yt-dlp transfer require manual checks.

## Resource and progress checks

Scheduler tests exercise twenty jobs with five network and two processing permits, simultaneous network/processing work, full processing queues, live limit changes, cancellation and bounded metadata. Progress tests feed 2,000 events without persistent writes, check copied metric pointers and expire stale aggregate speeds. SQLite tests migrate a real schema-1 database to schema 2 while retaining history.

With yt-dlp installed, the local HTTP integration test downloads and verifies video and audio with the real tool. Restricted environments that cannot bind a loopback socket skip that test explicitly. FFmpeg fixture tests check every audio profile and the merge of separate video/audio streams.

`python3 scripts/test_rate_limit.py` measures two and five real yt-dlp downloads on loopback after a five-second warmup. Each ten-second window must remain below 110% of the configured budget. The fixed-share policy intentionally underuses bandwidth with fewer than the configured maximum transfers. This does not assert an instantaneous hard cap or live redistribution.

## Workspace, credentials and S3

`python3 scripts/test_workspace.py ./camelclip` starts a temporary loopback-only S3 API server and a real FFmpeg fixture. It exercises CLI library import/exclusion/reimport, private descriptor password/credential input, read-only and conditional-write tests, upload verification and deduplication. It checks database/WAL bytes for test secrets and confirms index removal keeps the file. No real keys/accounts are used.

Go tests cover priority and disk-pressure permit ordering, 10,000 paginated assets with Unicode search, published-asset/source retention, publication/automatic-upload idempotency, AEAD record binding/tampering, wrong-password unlock, atomic password rotation, per-session locking, independent two-object S3 concurrency, failed-profile isolation and recovery of acknowledged remote objects. S3 adapter tests stream both single and multipart files and reject endpoints that ignore conditional writes. Metrics tests cover initial unknown CPU, delta/reset counters and every core in a scrollable resource panel. Secret entry fields are checked against rendered output.

## Component setup and public content

`python3 scripts/check_public_content.py` checks English-only public documentation/installer sources and relative documentation links. It intentionally does not scan application localization fixtures or third-party legal text. `go test -race ./internal/installer` covers all seven choices, ownership, path safety, checksum rejection, workspace locks, preservation and failed-upgrade rollback.

After building all installer targets, `python3 scripts/test_installers.py` validates all archive hashes and executable architectures, runs seven native user-scoped install/reinstall/add/remove/uninstall combinations, exercises the English terminal menu through a PTY, launches native CLI, and inspects both macOS package payloads. It uses temporary prefixes/configuration and disposable files. macOS Installer choice evaluation is read-only with `scripts/test_installer_choices.py`.

### Current acceptance limits

| Area | Current evidence | Remaining acceptance |
| --- | --- | --- |
| macOS arm64 application/installer | Race suite, native CLI/user installs, archive tests and read-only package choice evaluation | System-wide installation on a clean system/account |
| macOS Intel | Cross-build and artifact inspection | Native runtime and installer acceptance |
| Linux arm64/amd64 | Cross-build, ELF/manifest checks and common installer unit tests | Native setup, browser/desktop integration and runtime acceptance |
| Public distribution | Versioned artifacts and checked hashes | Apple signing and notarization |
| S3 | Local API emulator | Production provider/account-specific acceptance |

Past reports are retained under `docs/history/` for context only. They are not a statement that current release binaries passed native acceptance on those platforms. Run the full suite with `-p 1` when the machine is loaded to reduce process-fixture timing noise.
