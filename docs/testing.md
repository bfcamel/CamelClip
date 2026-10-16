# Failure and recovery coverage

| Scenario | Automated evidence |
| --- | --- |
| Full SQLite storage | Real `PRAGMA max_page_count` limit causes SQLITE_FULL; transaction rolls back and the database reopens with original jobs |
| Corrupt SQLite/legacy JSON | Startup refuses invalid data and preserves original bytes |
| Failed settings save | Canceled writes preserve preferences; failed rename removes temporary files |
| Interrupted network/downloader | Simulated nonzero downloader exit; no final file; retry succeeds |
| Conversion or full-decode verification failure | Injected FFmpeg errors; no final publication; staged source can be retried |
| Abrupt process kill during all three stages | Native subprocess/PTY smoke test restarts against the same SQLite queue; exactly one verified output is published |
| Cancellation with descendants | Delayed child marker must never be written after cancellation |
| Existing invalid user output | Error is returned and existing bytes are preserved |
| Multiple owners | SQLite repository lock tests reject another owner |
| Lifecycle restore | Application tests restore every queued/active/terminal state correctly |
| Terminal navigation | Actual PTY settings/back navigation, idle refresh and terminal-state restoration |
| Installation and update | Extracted-package layout, isolated HOME, overwrite protection, --force and idempotent PATH entry |
| Diagnostic privacy | Fake dependencies print secret paths/URLs; only safe numeric versions enter the report |

Run `go test -race -count=1 -timeout=5m ./...` and `python3 scripts/smoke.py ./macdown` with FFmpeg installed. The tests use temporary directories and local fixtures. They do not fill the host disk, kill unrelated processes or access browser data.

A simulated FFmpeg ENOSPC response and a real SQLite quota exercise application failure paths; they do not simulate a physical disk losing power, filesystem corruption or every kernel writeback behavior. Native desktop opening is covered with a fake xdg-open command, so manual checks on GNOME/KDE and different macOS players are still valuable. External auth/session expiry and an actual network outage during yt-dlp transfer require manual checks.
